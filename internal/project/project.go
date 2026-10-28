// .htn 매뉴얼 파일(스펙 5.3절) 쓰기와 읽기. ZIP 안에 manifest.json, guide.json, images/<sha256>.png를 둔다.
// 저장소를 모르는 형식 패키지다: 편집용·배포용 내용을 고르는 일은 앱(app_project.go)이 한다.
// 읽기는 남이 보낸 파일을 받는 신뢰 경계라 크기·개수·형식을 모두 검사하고, ZIP 안 이름으로 디스크에 풀지 않는다.
package project

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	Format      = "htn"
	Version     = 1
	KindEdit    = "edit"    // 원본 그림을 그대로 넣는다
	KindPublish = "publish" // 모자이크·자르기를 그림에 확정한다(M4a 결정 H1)
)

// 읽기 한계. 녹화 한 편이 수백 장이어도 넉넉하고, 압축 폭탄과 끝없는 항목은 막는다.
const (
	maxSteps   = 10000
	maxEntries = 2*maxSteps + 16
	maxJSON    = 64 << 20 // manifest.json, guide.json 하나
	maxImage   = 64 << 20 // 그림 하나
	maxTotal   = 4 << 30  // 압축을 푼 전체(선언된 크기 합)
)

var (
	ErrBad   = errors.New("project: HowtoNote 파일이 아니거나 망가졌다")
	ErrNewer = errors.New("project: 더 새 버전의 HowtoNote 파일이다")
)

type Manifest struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	AppVersion string `json:"app_version"`
	Kind       string `json:"kind"`
}

// Step은 guide.json의 스텝 하나다. 순서는 배열 순서이고, 상위 스텝은 DB ID 대신 배열 번호로 가리킨다(-1이면 최상위, H6).
// JSON 칸은 저장소의 JSON을 그대로 옮긴다. Description은 비었으면 넣지 않는다.
type Step struct {
	Parent      int             `json:"parent"`
	Kind        string          `json:"kind"`
	Title       string          `json:"title"`
	Description json.RawMessage `json:"description,omitempty"`
	Image       string          `json:"image,omitempty"` // 그림 내용의 sha256. 없으면 넣지 않는다
	Hidden      bool            `json:"hidden,omitempty"`
	Capture     json.RawMessage `json:"capture"`
	Annotations json.RawMessage `json:"annotations"`
	Callouts    json.RawMessage `json:"callouts"`
	View        json.RawMessage `json:"view"`
}

type Guide struct {
	Title string `json:"title"`
	Steps []Step `json:"steps"`
}

// internal/store/migrations/001_init.sql의 kind CHECK, 편집기의 스텝 종류와 맞춰 둔다(종류를 추가하면 셋 다 고친다)
var kinds = map[string]bool{"click": true, "type": true, "manual": true, "image": true, "text": true}

func bad(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrBad}, a...)...)
}

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func validSHA(s string) bool { return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == "" }

// Write는 .htn을 w에 쓴다. image는 그림 이름(sha256)의 PNG 내용을 준다(편집용은 원본, 배포용은 확정한 그림).
// 같은 그림은 한 번만 넣고, 내용의 sha256이 이름과 다르면 쓰지 않는다(받는 쪽도 같은 검사를 한다).
func Write(w io.Writer, m Manifest, g Guide, image func(sha string) ([]byte, error)) error {
	z := zip.NewWriter(w)
	for _, f := range []struct {
		name string
		v    any
	}{{"manifest.json", m}, {"guide.json", g}} {
		fw, err := z.Create(f.name)
		if err != nil {
			return err
		}
		if err := json.NewEncoder(fw).Encode(f.v); err != nil {
			return err
		}
	}
	done := map[string]bool{}
	for _, s := range g.Steps {
		if s.Image == "" || done[s.Image] {
			continue
		}
		done[s.Image] = true
		b, err := image(s.Image)
		if err != nil {
			return err
		}
		if shaHex(b) != s.Image {
			return fmt.Errorf("project: 그림 %s의 내용이 이름과 다르다", s.Image)
		}
		// PNG는 이미 압축돼 있어 다시 압축하지 않는다
		fw, err := z.CreateHeader(&zip.FileHeader{Name: "images/" + s.Image + ".png", Method: zip.Store})
		if err != nil {
			return err
		}
		if _, err := fw.Write(b); err != nil {
			return err
		}
	}
	return z.Close()
}

// Package는 읽고 검사한 .htn이다. 그림은 Image로 필요할 때 하나씩 읽는다(한 번에 모두 메모리에 올리지 않는다).
type Package struct {
	Manifest Manifest
	Guide    Guide
	images   map[string]*zip.File
}

// sizeOK는 선언된 압축 푼 크기의 합이 maxTotal 안인지 본다. 공격자가 정한 값이라 더하다 넘치지 않게 뺄셈으로 비교한다.
func sizeOK(files []*zip.File) bool {
	var total uint64
	for _, f := range files {
		if f.UncompressedSize64 > maxTotal-total {
			return false
		}
		total += f.UncompressedSize64
	}
	return true
}

// Read는 .htn을 읽고 검사한다. 형식이 틀리면 ErrBad, 더 새 버전이면 ErrNewer다(errors.Is로 가린다).
func Read(r io.ReaderAt, size int64) (*Package, error) {
	// 파일 자체 크기부터 본다: zip.NewReader가 중앙 디렉터리를 메모리에 읽기 전에 거른다
	if size > maxTotal {
		return nil, bad("파일 %d바이트", size)
	}
	z, err := zip.NewReader(r, size)
	if err != nil {
		return nil, bad("ZIP: %v", err)
	}
	if len(z.File) > maxEntries {
		return nil, bad("항목 %d개", len(z.File))
	}
	if !sizeOK(z.File) {
		return nil, bad("압축을 푼 전체 크기가 너무 크다")
	}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		if files[f.Name] != nil {
			return nil, bad("같은 이름 %q", f.Name)
		}
		files[f.Name] = f
	}
	p := &Package{images: map[string]*zip.File{}}
	if err := readJSON(files["manifest.json"], &p.Manifest); err != nil {
		return nil, err
	}
	// 버전을 종류보다 먼저 본다: 새 버전은 지금 모르는 종류를 쓸 수 있다
	switch m := p.Manifest; {
	case m.Format != Format || m.Version < 1:
		return nil, bad("manifest %+v", m)
	case m.Version > Version:
		return nil, ErrNewer
	case m.Kind != KindEdit && m.Kind != KindPublish:
		return nil, bad("종류 %q", m.Kind)
	}
	if err := readJSON(files["guide.json"], &p.Guide); err != nil {
		return nil, err
	}
	for name, f := range files {
		if sha, ok := strings.CutPrefix(name, "images/"); ok {
			if sha, ok = strings.CutSuffix(sha, ".png"); ok && validSHA(sha) {
				p.images[sha] = f
			}
		}
	}
	if err := p.check(); err != nil {
		return nil, err
	}
	return p, nil
}

// check는 guide.json이 저장소에 넣을 수 있는 모양인지 본다(저장소의 checkStep과 같은 JSON 규칙, 하위 스텝은 한 단계).
func (p *Package) check() error {
	steps := p.Guide.Steps
	if len(steps) > maxSteps {
		return bad("스텝 %d개", len(steps))
	}
	for i, s := range steps {
		switch {
		case !kinds[s.Kind]:
			return bad("스텝 %d: kind %q", i, s.Kind)
		case s.Parent < -1 || s.Parent >= i:
			return bad("스텝 %d: parent %d", i, s.Parent)
		case s.Parent >= 0 && steps[s.Parent].Parent != -1:
			return bad("스텝 %d: 하위 스텝의 하위", i)
		case s.Image != "" && p.images[s.Image] == nil:
			return bad("스텝 %d: 그림 %q 없음", i, s.Image)
		case !isJSON(s.Capture, '{') || !isJSON(s.Annotations, '[') || !isJSON(s.Callouts, '[') || !isJSON(s.View, '{'):
			return bad("스텝 %d: JSON 칸", i)
		case len(s.Description) > 0 && !isJSON(s.Description, '{'):
			return bad("스텝 %d: 설명", i)
		}
	}
	return nil
}

func isJSON(b json.RawMessage, open byte) bool {
	b = bytes.TrimSpace(b)
	return len(b) > 0 && b[0] == open && json.Valid(b)
}

func readJSON(f *zip.File, v any) error {
	if f == nil {
		return bad("JSON 항목 없음")
	}
	b, err := readAll(f, maxJSON)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return bad("%s: %v", f.Name, err)
	}
	return nil
}

// readAll은 선언된 크기를 믿지 않고 limit+1바이트까지만 읽는다(압축 폭탄). 체크섬이 틀리면 zip이 오류를 준다.
func readAll(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, bad("%s: %v", f.Name, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, bad("%s: %v", f.Name, err)
	}
	if int64(len(b)) > limit {
		return nil, bad("%s: %d바이트 넘음", f.Name, limit)
	}
	return b, nil
}

// Image는 이름(sha256)의 그림 내용이다. 없거나 내용의 sha256이 이름과 다르면 ErrBad다.
func (p *Package) Image(sha string) ([]byte, error) {
	f := p.images[sha]
	if f == nil {
		return nil, bad("그림 %q 없음", sha)
	}
	b, err := readAll(f, maxImage)
	if err != nil {
		return nil, err
	}
	if shaHex(b) != sha {
		return nil, bad("그림 %q 내용이 이름과 다름", sha)
	}
	return b, nil
}
