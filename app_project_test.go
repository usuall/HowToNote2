// .htn 바인딩 테스트: 편집용은 원본 그림·숨김·그리기를 그대로, 배포용은 숨긴 스텝을 빼고 모자이크·자르기를 확정한 그림과
// 옮긴 그리기를 넣는다(H1·H3). 녹화 PC 정보는 dpi_scale만 남는다(H4). 저장 창에서 고른 경로만 받는다.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"howtonote/internal/project"
	"howtonote/internal/store"
)

const blur = `{"id":"b1","type":"blur","x":0,"y":0,"w":4,"h":4,"style":{"stroke":"","fill":"","width":8,"opacity":1,"font_size":0}}`

// readHtn은 .htn 파일을 읽고 검사한다.
func readHtn(t *testing.T, path string) *project.Package {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := project.Read(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// jsonEntries는 .htn 안 manifest.json과 guide.json을 압축을 푼 채로 이어 붙인다(경로가 새는지 볼 때 쓴다).
func jsonEntries(t *testing.T, path string) []byte {
	t.Helper()
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var out []byte
	for _, f := range z.File {
		if strings.HasSuffix(f.Name, ".json") {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			out = append(out, b...)
		}
	}
	return out
}

// edit는 가이드의 i번째 스텝을 mod로 고쳐 저장한다.
func edit(t *testing.T, a *App, gid int64, i int, mod func(*store.Step)) {
	t.Helper()
	steps, err := a.st.ListSteps(gid)
	if err != nil {
		t.Fatal(err)
	}
	mod(&steps[i])
	if err := a.st.UpdateSteps(gid, []store.Step{steps[i]}); err != nil {
		t.Fatal(err)
	}
}

func TestApp_ExportProjectEdit(t *testing.T) {
	dir := t.TempDir()
	a, gid := fixture(t, dir)
	defer a.st.Close()
	steps, _ := a.st.ListSteps(gid)
	edit(t, a, gid, 1, func(s *store.Step) { s.Hidden = true; s.ParentID = steps[0].ID })
	edit(t, a, gid, 2, func(s *store.Step) {
		s.ViewJSON = `{"crop":{"x":2,"y":1,"w":4,"h":4}}`
		s.Description = `{"type":"doc","content":[]}`
	})
	img, _, err := a.st.PutImage(encode(t, "png", 8, 6), 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.st.AppendStep(gid, store.NewStep{Kind: "click", Title: "넷", ImageID: img, AnnotationsJSON: "[" + blur + "]",
		CaptureJSON: `{"dpi_scale":1.25,"window_title":"C:\\Users\\비밀\\급여.xlsx","process":"EXCEL.EXE"}`}); err != nil {
		t.Fatal(err)
	}
	path := choose(a, filepath.Join(t.TempDir(), "m.htn"))
	if err := a.ExportProject(gid, project.KindEdit, path); err != nil {
		t.Fatal(err)
	}
	p := readHtn(t, path)
	all, _ := a.st.ListSteps(gid)
	g := p.Guide
	if p.Manifest.Kind != project.KindEdit || p.Manifest.AppVersion == "" || g.Title != "고정 가이드" || len(g.Steps) != 4 {
		t.Fatalf("manifest %+v, 제목 %q, 스텝 %d개", p.Manifest, g.Title, len(g.Steps))
	}
	for i, s := range g.Steps {
		if s.Image != all[i].ImageSHA || string(s.Annotations) != all[i].AnnotationsJSON || string(s.View) != all[i].ViewJSON {
			t.Errorf("스텝 %d = %+v", i, s)
		}
	}
	if g.Steps[1].Parent != 0 || !g.Steps[1].Hidden || string(g.Steps[2].Description) != `{"type":"doc","content":[]}` {
		t.Errorf("하위·숨김·설명 = %+v / %+v", g.Steps[1], g.Steps[2])
	}
	if string(g.Steps[3].Capture) != `{"dpi_scale":1.25}` || string(g.Steps[0].Capture) != `{"dpi_scale":1}` {
		t.Errorf("capture = %s / %s", g.Steps[3].Capture, g.Steps[0].Capture)
	}
	if b, err := p.Image(all[0].ImageSHA); err != nil || !bytes.Equal(b, encode(t, "png", 8, 6)) {
		t.Errorf("원본 그림 = %d바이트, %v", len(b), err)
	}
	js := jsonEntries(t, path)
	for _, leak := range []string{"비밀", "급여", "EXCEL", "window_title", filepath.ToSlash(dir), strings.ReplaceAll(dir, `\`, `\\`)} {
		if bytes.Contains(js, []byte(leak)) {
			t.Errorf(".htn에 %q가 들어 있다", leak)
		}
	}
}

func TestApp_ExportProjectPublish(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	// 스텝 0: 클릭 표시(1,1)와 모자이크, 자르기 (2,1)-(6,5). 스텝 1: 숨김. 스텝 2: 그대로
	edit(t, a, gid, 0, func(s *store.Step) {
		s.AnnotationsJSON = strings.TrimSuffix(marker, "]") + "," + blur + "]"
		s.ViewJSON = `{"crop":{"x":2,"y":1,"w":4,"h":4}}`
	})
	edit(t, a, gid, 1, func(s *store.Step) { s.Hidden = true })
	path := choose(a, filepath.Join(t.TempDir(), "m.htn"))
	if err := a.ExportProject(gid, project.KindPublish, path); !errors.Is(err, errNotRendered) {
		t.Fatalf("그림 준비 전 err = %v, want errNotRendered", err)
	}
	jobs, err := a.ProjectPlan(gid)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("ProjectPlan = %+v, %v", jobs, err)
	}
	if jobs[0].AnnotationsJSON != "["+blur+"]" || jobs[0].ViewJSON != `{"crop":{"x":2,"y":1,"w":4,"h":4}}` || jobs[1].AnnotationsJSON != "[]" {
		t.Fatalf("ProjectPlan 재료 = %+v", jobs)
	}
	baked := map[string][]byte{}
	for i, j := range jobs {
		b := encode(t, "png", 4+i, 4+i) // 원본(8x6)과 다른 그림
		baked[j.Key] = b
		if err := a.PutRendered(j.Key, base64.StdEncoding.EncodeToString(b)); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.ExportProject(gid, project.KindPublish, choose(a, path)); err != nil {
		t.Fatal(err)
	}
	p := readHtn(t, path)
	g := p.Guide
	if p.Manifest.Kind != project.KindPublish || len(g.Steps) != 2 {
		t.Fatalf("manifest %+v, 스텝 %d개(숨긴 스텝은 빠져야 한다)", p.Manifest, len(g.Steps))
	}
	for i, s := range g.Steps {
		b, err := p.Image(s.Image)
		if err != nil || !bytes.Equal(b, baked[jobs[i].Key]) {
			t.Errorf("스텝 %d 그림이 확정 그림이 아니다: %v", i, err)
		}
		if string(s.View) != "{}" || bytes.Contains(s.Annotations, []byte(`"blur"`)) || s.Hidden {
			t.Errorf("스텝 %d 보기 %s, 그리기 %s, 숨김 %v", i, s.View, s.Annotations, s.Hidden)
		}
	}
	var ms []struct{ X, Y float64 }
	if err := json.Unmarshal(g.Steps[0].Annotations, &ms); err != nil || len(ms) != 1 || ms[0].X != -1 || ms[0].Y != 0 {
		t.Errorf("자른 만큼 옮긴 클릭 표시 = %s, %v", g.Steps[0].Annotations, err)
	}
	if err := json.Unmarshal(g.Steps[1].Annotations, &ms); err != nil || len(ms) != 1 || ms[0].X != 1 || ms[0].Y != 1 {
		t.Errorf("자르기 없는 스텝의 클릭 표시 = %s, %v", g.Steps[1].Annotations, err)
	}
}

func TestApp_ExportProjectGuards(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	path := filepath.Join(t.TempDir(), "m.htn")
	if err := a.ExportProject(gid, project.KindEdit, path); !errors.Is(err, errExportWrite) {
		t.Errorf("고르지 않은 경로 err = %v", err)
	}
	if err := a.ExportProject(gid, "share", choose(a, path)); !errors.Is(err, errKind) {
		t.Errorf("모르는 종류 err = %v", err)
	}
	if exists(path) {
		t.Error("거절했는데 파일이 생겼다")
	}
	bad := filepath.Join(t.TempDir(), "없는 폴더", "m.htn")
	err := a.ExportProject(gid, project.KindEdit, choose(a, bad))
	if !errors.Is(err, errExportWrite) || strings.ContainsAny(err.Error(), `\/:`) {
		t.Errorf("쓸 수 없는 경로 err = %v(경로 없는 고정 문장이어야 한다)", err)
	}
}

// 원본 그림이 디스크에 없으면 project.Write 안 쓰기 오류(errExportWrite)가 아니라 errLoad여야 한다(폴더 문제가 아니다).
func TestApp_ExportProjectMissingImage(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	steps, _ := a.st.ListSteps(gid)
	p, ok := a.st.ImageFile(steps[0].ImageSHA)
	if !ok {
		t.Fatal("ImageFile 실패")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	path := choose(a, filepath.Join(t.TempDir(), "m.htn"))
	if err := a.ExportProject(gid, project.KindEdit, path); !errors.Is(err, errLoad) {
		t.Fatalf("err = %v, want errLoad", err)
	}
	if exists(path) {
		t.Error("원본이 없는데 파일이 생겼다")
	}
}

func TestCropOf(t *testing.T) {
	for _, c := range []struct {
		view string
		want image.Rectangle
	}{
		{`{}`, image.Rect(0, 0, 8, 6)},
		{`{"crop":{"x":2,"y":1,"w":4,"h":4}}`, image.Rect(2, 1, 6, 5)},
		{`{"crop":{"x":-3,"y":5.6,"w":100,"h":100}}`, image.Rect(0, 5, 8, 6)}, // annotations.ts clampCrop과 같은 결과
		{`{"crop":{"x":1,"y":1,"w":0,"h":3}}`, image.Rect(0, 0, 8, 6)},
		{`깨짐`, image.Rect(0, 0, 8, 6)},
	} {
		if got := cropOf(c.view, 8, 6); got != c.want {
			t.Errorf("cropOf(%s) = %v, want %v", c.view, got, c.want)
		}
	}
}

func TestShiftAnnotations(t *testing.T) {
	in := `[{"id":"a","type":"arrow","x":10,"y":10,"w":5,"h":5,"points":[10,10,15,15],"style":{}},` +
		`{"id":"b","type":"blur","x":0,"y":0,"w":5,"h":5,"style":{}},` +
		`{"id":"c","type":"rect","x":100,"y":100,"w":5,"h":5,"style":{}},` +
		`{"id":"d","type":"text","x":12,"y":12,"w":5,"h":5,"text":"글","style":{},"extra":"그대로"}]`
	var got []map[string]any
	if err := json.Unmarshal([]byte(shiftAnnotations(in, 10, 10, 20, 20)), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["id"] != "a" || got[1]["id"] != "d" {
		t.Fatalf("남은 것 = %v(모자이크 b와 밖으로 나간 c는 빠져야 한다)", got)
	}
	if got[0]["x"] != 0.0 || fmt.Sprint(got[0]["points"]) != "[0 0 5 5]" || got[1]["x"] != 2.0 || got[1]["text"] != "글" || got[1]["extra"] != "그대로" {
		t.Errorf("옮긴 값 = %v", got)
	}
}

func TestCaptureOf(t *testing.T) {
	for in, want := range map[string]string{
		`{"dpi_scale":1.5,"window_title":"비밀","process":"x.exe"}`: `{"dpi_scale":1.5}`,
		`{"window_title":"비밀"}`:                                   `{}`,
		`{}`:                                                      `{}`,
		`깨짐`:                                                      `{}`,
	} {
		if got := string(captureOf(in)); got != want {
			t.Errorf("captureOf(%s) = %s, want %s", in, got, want)
		}
	}
}

// writeHtn은 project.Write로 .htn 파일을 만든다(틀린 파일을 만드는 데 쓴다).
func writeHtn(t *testing.T, m project.Manifest, g project.Guide, imgs map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.htn")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := project.Write(f, m, g, func(sha string) ([]byte, error) { return imgs[sha], nil }); err != nil {
		t.Fatal(err)
	}
	return path
}

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// 스펙 10절: .htn으로 내보낸 뒤 다시 가져오면 같다.
func TestApp_ImportRoundTrip(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	steps, _ := a.st.ListSteps(gid)
	edit(t, a, gid, 1, func(s *store.Step) {
		s.Hidden = true
		s.ParentID = steps[0].ID
		s.CalloutsJSON = `[{"type":"tip","title":"팁","body":null,"position":"after"}]`
	})
	edit(t, a, gid, 2, func(s *store.Step) {
		s.ViewJSON = `{"crop":{"x":2,"y":1,"w":4,"h":4}}`
		s.Description = `{"type":"doc","content":[]}`
	})
	if _, err := a.st.InsertStep(gid, 0, 10, store.NewStep{Kind: "text", Title: "글 스텝"}); err != nil {
		t.Fatal(err)
	}
	path := choose(a, filepath.Join(t.TempDir(), "m.htn"))
	if err := a.ExportProject(gid, project.KindEdit, path); err != nil {
		t.Fatal(err)
	}
	fid, err := a.st.CreateFolder("받은 것")
	if err != nil {
		t.Fatal(err)
	}
	nid, err := a.importFile(path, fid)
	if err != nil || nid == gid {
		t.Fatalf("importFile = %d, %v", nid, err)
	}
	if g, _ := a.st.GetGuide(nid); g.Title != "고정 가이드" || g.FolderID != fid {
		t.Errorf("가져온 매뉴얼 = %+v", g)
	}
	want, _ := a.st.ListSteps(gid)
	got, _ := a.st.ListSteps(nid)
	if len(got) != len(want) {
		t.Fatalf("스텝 %d개, want %d", len(got), len(want))
	}
	at := func(list []store.Step, id int64) int {
		for j, s := range list {
			if s.ID == id {
				return j
			}
		}
		return -1
	}
	for i := range want {
		w, g := want[i], got[i]
		if g.Kind != w.Kind || g.Title != w.Title || g.Description != w.Description || g.ImageSHA != w.ImageSHA || g.Hidden != w.Hidden ||
			g.AnnotationsJSON != w.AnnotationsJSON || g.CalloutsJSON != w.CalloutsJSON || g.ViewJSON != w.ViewJSON ||
			g.CaptureJSON != w.CaptureJSON || at(got, g.ParentID) != at(want, w.ParentID) {
			t.Errorf("스텝 %d\n got %+v\nwant %+v", i, g, w)
		}
	}
	// 같은 파일을 두 번 가져오면 사본이 하나 더 생긴다(H5)
	if n2, err := a.importFile(path, 0); err != nil || n2 == nid {
		t.Fatalf("두 번째 가져오기 = %d, %v", n2, err)
	}
}

func TestApp_ImportRejects(t *testing.T) {
	a, _ := fixture(t, t.TempDir())
	defer a.st.Close()
	m := project.Manifest{Format: project.Format, Version: project.Version, Kind: project.KindEdit}
	guide := func(title, img string) project.Guide {
		return project.Guide{Title: title, Steps: []project.Step{{Parent: -1, Kind: "click", Image: img, Capture: json.RawMessage(`{}`),
			Annotations: json.RawMessage(`[]`), Callouts: json.RawMessage(`[]`), View: json.RawMessage(`{}`)}}}
	}
	jpg := encode(t, "jpeg", 8, 6)
	big, _ := base64.StdEncoding.DecodeString(pngSized(t, 8000, 6000)) // 4800만 화소: maxPixels를 넘는다
	notZip := filepath.Join(t.TempDir(), "x.htn")
	if err := os.WriteFile(notZip, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, path string
		want       error
	}{
		{"ZIP이 아님", notZip, errImportBad},
		{"없는 파일", filepath.Join(t.TempDir(), "없음.htn"), errImportRead},
		{"더 새 버전", writeHtn(t, project.Manifest{Format: "htn", Version: 9, Kind: "share"}, guide("x", ""), nil), errImportNewer},
		{"PNG가 아닌 그림", writeHtn(t, m, guide("x", shaHex(jpg)), map[string][]byte{shaHex(jpg): jpg}), errImportBad},
		{"너무 큰 그림", writeHtn(t, m, guide("x", shaHex(big)), map[string][]byte{shaHex(big): big}), errImportBad},
	} {
		_, err := a.importFile(c.path, 0)
		if !errors.Is(err, c.want) || strings.ContainsAny(err.Error(), `\/:`) {
			t.Errorf("%s: err = %v, want %v(경로 없는 고정 문장)", c.name, err, c.want)
		}
	}
	if gs, _ := a.st.ListGuides(); len(gs) != 1 {
		t.Fatalf("실패한 가져오기가 매뉴얼을 남겼다: %d개", len(gs))
	}
	// 제목이 비면 '가져온 매뉴얼'이다(H5)
	ok := encode(t, "png", 3, 2)
	nid, err := a.importFile(writeHtn(t, m, guide("  ", shaHex(ok)), map[string][]byte{shaHex(ok): ok}), 0)
	if g, _ := a.st.GetGuide(nid); err != nil || g.Title != "가져온 매뉴얼" {
		t.Fatalf("빈 제목 = %+v, %v", g, err)
	}
}

func TestApp_ImportWaitsForPrune(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	path := choose(a, filepath.Join(t.TempDir(), "m.htn"))
	if err := a.ExportProject(gid, project.KindEdit, path); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	a.mu.Lock() // 그림 정리가 도는 중: 그림을 넣고 스텝을 넣기 전에 정리가 끼면 새 그림이 고아로 지워진다
	go func() {
		_, err := a.importFile(path, 0)
		done <- err
	}()
	select {
	case err := <-done:
		a.mu.Unlock()
		t.Fatalf("정리가 a.mu를 쥔 동안 끝났다(%v). 기다려야 한다", err)
	case <-time.After(150 * time.Millisecond):
	}
	a.mu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
