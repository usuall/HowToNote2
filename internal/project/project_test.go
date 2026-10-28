// .htn 형식 테스트: 쓰고 읽으면 같다, 같은 그림은 한 번만 넣는다, 틀린 파일은 ErrBad, 더 새 버전은 ErrNewer,
// 그림 내용과 이름(sha256) 검사, 선언된 크기를 믿지 않는 읽기 한계.
package project

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"
)

// 형식 패키지는 PNG 내용을 보지 않는다(앱이 본다). 그래서 그림은 아무 바이트나 된다.
var (
	imgA = []byte("\x89PNG-a")
	imgB = []byte("\x89PNG-b")
)

func shaOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// sample은 그림을 함께 쓰는 스텝 둘(최상위와 그 하위), 글 스텝, 다른 그림 스텝이 있는 매뉴얼이다.
func sample() (Manifest, Guide) {
	a, b := shaOf(imgA), shaOf(imgB)
	m := Manifest{Format: Format, Version: Version, AppVersion: "0.0.1", Kind: KindEdit}
	g := Guide{Title: "연차 신청", Steps: []Step{
		{Parent: -1, Kind: "click", Title: "하나", Image: a, Capture: raw(`{"dpi_scale":1.25}`),
			Annotations: raw(`[{"id":"m","type":"marker","x":1,"y":2,"w":3,"h":4,"style":{}}]`), Callouts: raw(`[]`), View: raw(`{}`),
			Description: raw(`{"type":"doc","content":[]}`)},
		{Parent: 0, Kind: "click", Title: "하위", Image: a, Hidden: true, Capture: raw(`{}`), Annotations: raw(`[]`),
			Callouts: raw(`[{"type":"tip","title":"팁","body":null,"position":"after"}]`), View: raw(`{"crop":{"x":1,"y":1,"w":2,"h":2}}`)},
		{Parent: -1, Kind: "text", Title: "글", Capture: raw(`{}`), Annotations: raw(`[]`), Callouts: raw(`[]`), View: raw(`{}`)},
		{Parent: -1, Kind: "image", Image: b, Capture: raw(`{}`), Annotations: raw(`[]`), Callouts: raw(`[]`), View: raw(`{}`)},
	}}
	return m, g
}

func images(sha string) ([]byte, error) {
	for _, b := range [][]byte{imgA, imgB} {
		if shaOf(b) == sha {
			return b, nil
		}
	}
	return nil, errors.New("없는 그림")
}

// zipOf는 [이름, 내용] 목록으로 ZIP을 만든다. 틀린 파일을 만드는 데 쓴다.
func zipOf(t *testing.T, files ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := z.Create(f[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestWriteRead_RoundTrip(t *testing.T) {
	m, g := sample()
	var buf bytes.Buffer
	if err := Write(&buf, m, g, images); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	p, err := Read(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if p.Manifest != m {
		t.Errorf("Manifest = %+v, want %+v", p.Manifest, m)
	}
	if got, want := mustJSON(p.Guide), mustJSON(g); got != want {
		t.Errorf("Guide\n got %s\nwant %s", got, want)
	}
	for _, img := range [][]byte{imgA, imgB} {
		if got, err := p.Image(shaOf(img)); err != nil || !bytes.Equal(got, img) {
			t.Errorf("Image = %q, %v", got, err)
		}
	}
	// 같은 그림(imgA)은 한 번만 들어간다
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	want := []string{"manifest.json", "guide.json", "images/" + shaOf(imgA) + ".png", "images/" + shaOf(imgB) + ".png"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("항목 = %v, want %v", names, want)
	}
}

func TestRead_Rejects(t *testing.T) {
	m, g := sample()
	mj := func(mod func(*Manifest)) string {
		x := m
		mod(&x)
		return mustJSON(x)
	}
	gj := func(mod func(*Guide)) string {
		x := g
		x.Steps = append([]Step(nil), g.Steps...)
		mod(&x)
		return mustJSON(x)
	}
	ok := mj(func(*Manifest) {})
	okGuide := gj(func(*Guide) {})
	imgs := [][2]string{{"images/" + shaOf(imgA) + ".png", string(imgA)}, {"images/" + shaOf(imgB) + ".png", string(imgB)}}
	pkg := func(manifest, guide string) []byte {
		return zipOf(t, append([][2]string{{"manifest.json", manifest}, {"guide.json", guide}}, imgs...)...)
	}
	for _, c := range []struct {
		name string
		data []byte
		want error
	}{
		{"ZIP이 아님", []byte("hello"), ErrBad},
		{"manifest 없음", zipOf(t, [2]string{"guide.json", okGuide}), ErrBad},
		{"형식 이름", pkg(mj(func(x *Manifest) { x.Format = "zip" }), okGuide), ErrBad},
		{"버전 0", pkg(mj(func(x *Manifest) { x.Version = 0 }), okGuide), ErrBad},
		{"모르는 종류", pkg(mj(func(x *Manifest) { x.Kind = "share" }), okGuide), ErrBad},
		{"더 새 버전(모르는 종류여도)", pkg(mj(func(x *Manifest) { x.Version = 2; x.Kind = "share" }), okGuide), ErrNewer},
		{"guide 없음", zipOf(t, [2]string{"manifest.json", ok}), ErrBad},
		{"guide가 JSON이 아님", pkg(ok, "{"), ErrBad},
		{"모르는 kind", pkg(ok, gj(func(x *Guide) { x.Steps[2].Kind = "video" })), ErrBad},
		{"뒤를 가리키는 parent", pkg(ok, gj(func(x *Guide) { x.Steps[0].Parent = 2 })), ErrBad},
		{"두 단계 하위", pkg(ok, gj(func(x *Guide) { x.Steps[2].Parent = 1 })), ErrBad},
		{"없는 그림", pkg(ok, gj(func(x *Guide) { x.Steps[2].Image = shaOf([]byte("x")) })), ErrBad},
		{"그리기가 배열이 아님", pkg(ok, gj(func(x *Guide) { x.Steps[0].Annotations = raw(`{}`) })), ErrBad},
		{"그리기가 없음", pkg(ok, gj(func(x *Guide) { x.Steps[0].Annotations = nil })), ErrBad},
		{"보기가 객체가 아님", pkg(ok, gj(func(x *Guide) { x.Steps[0].View = raw(`[]`) })), ErrBad},
		{"설명이 객체가 아님", pkg(ok, gj(func(x *Guide) { x.Steps[0].Description = raw(`"글"`) })), ErrBad},
		{"같은 이름 두 번", zipOf(t, [2]string{"manifest.json", ok}, [2]string{"manifest.json", ok}, [2]string{"guide.json", okGuide}), ErrBad},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Read(bytes.NewReader(c.data), int64(len(c.data)))
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestImage_NameMustMatchContent(t *testing.T) {
	m, g := sample()
	b := zipOf(t, [2]string{"manifest.json", mustJSON(m)}, [2]string{"guide.json", mustJSON(g)},
		[2]string{"images/" + shaOf(imgA) + ".png", string(imgB)}, // 이름은 A인데 내용은 B
		[2]string{"images/" + shaOf(imgB) + ".png", string(imgB)})
	p, err := Read(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Image(shaOf(imgA)); !errors.Is(err, ErrBad) {
		t.Fatalf("err = %v, want ErrBad", err)
	}
	if _, err := p.Image(shaOf([]byte("없음"))); !errors.Is(err, ErrBad) {
		t.Fatalf("없는 그림 err = %v, want ErrBad", err)
	}
}

func TestReadAll_DoesNotTrustDeclaredSize(t *testing.T) {
	b := zipOf(t, [2]string{"x", "0123456789A"})
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readAll(z.File[0], 10); !errors.Is(err, ErrBad) {
		t.Fatalf("한계보다 큰 항목 err = %v, want ErrBad", err)
	}
	if got, err := readAll(z.File[0], 11); err != nil || string(got) != "0123456789A" {
		t.Fatalf("readAll = %q, %v", got, err)
	}
}

func TestWrite_ImageMustMatchName(t *testing.T) {
	m, g := sample()
	err := Write(io.Discard, m, g, func(string) ([]byte, error) { return []byte("다른 내용"), nil })
	if err == nil {
		t.Fatal("이름과 내용이 다른 그림을 썼다")
	}
}

func TestSizeOK(t *testing.T) {
	huge := func(n uint64) *zip.File { return &zip.File{FileHeader: zip.FileHeader{UncompressedSize64: n}} }
	for _, c := range []struct {
		name  string
		files []*zip.File
		want  bool
	}{
		{"넘침(1<<63 둘)", []*zip.File{huge(1 << 63), huge(1 << 63)}, false},
		{"작은 합", []*zip.File{huge(10), huge(20)}, true},
		{"딱 maxTotal", []*zip.File{huge(maxTotal)}, true},
		{"maxTotal보다 하나 큼", []*zip.File{huge(maxTotal + 1)}, false},
	} {
		if got := sizeOK(c.files); got != c.want {
			t.Errorf("%s: sizeOK = %v, want %v", c.name, got, c.want)
		}
	}
}

// Read는 압축을 풀기 전에 파일 크기 자체도 본다(zip.NewReader가 중앙 디렉터리를 읽기 전에 거른다).
func TestRead_RejectsOversizedFile(t *testing.T) {
	_, err := Read(bytes.NewReader(nil), maxTotal+1)
	if !errors.Is(err, ErrBad) {
		t.Fatalf("err = %v, want ErrBad", err)
	}
}
