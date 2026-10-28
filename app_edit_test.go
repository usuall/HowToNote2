// 편집 바인딩 왕복 테스트: 고정 가이드를 바인딩으로 열고 순서·하위 스텝·그리기·자르기·설명·안내 상자를 고쳐 저장한 뒤,
// 저장소를 닫았다 다시 열어 디스크에 남은 JSON을 확인한다. 프론트가 보내는 것과 같은 store.Step 행을 쓴다.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"reflect"
	"testing"

	"howtonote/internal/store"
)

const marker = `[{"id":"m1","type":"marker","x":1,"y":1,"w":4,"h":4,"style":{"stroke":"#E5383B","fill":"#E5383B","width":1,"opacity":0.35,"font_size":0}}]`

// encode는 w x h 회색 그림을 png 또는 jpeg로 만든다.
func encode(t *testing.T, format string, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(0, 0, color.Black)
	var buf bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fixture는 클릭 스텝 3개(같은 8x6 그림, 클릭 표시 하나)가 있는 가이드를 dir에 만든다.
func fixture(t *testing.T, dir string) (*App, int64) {
	t.Helper()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	gid, _ := st.CreateGuide("고정 가이드")
	img, _, err := st.PutImage(encode(t, "png", 8, 6), 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"하나", "둘", "셋"} {
		if _, err := st.AppendStep(gid, store.NewStep{Kind: "click", Title: title, ImageID: img, CaptureJSON: `{"dpi_scale":1}`, AnnotationsJSON: marker}); err != nil {
			t.Fatal(err)
		}
	}
	return &App{st: st}, gid
}

func TestApp_EditRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a, gid := fixture(t, dir)
	d, err := a.OpenGuide(gid)
	if err != nil || len(d.Steps) != 3 || len(d.Deleted) != 0 {
		t.Fatalf("열기 = %+v, %v", d, err)
	}
	s1, s2, s3 := d.Steps[0], d.Steps[1], d.Steps[2]
	s3.Sort = 0.5       // 셋째를 맨 앞으로(model.ts moveBlock의 중간값)
	s2.ParentID = s1.ID // 둘째를 첫째의 하위 스텝으로
	s1.AnnotationsJSON = marker[:len(marker)-1] +
		`,{"id":"a1","type":"arrow","x":1,"y":1,"w":4,"h":3,"points":[1,1,5,4],"style":{"stroke":"#E5383B","fill":"#E5383B","width":1,"opacity":1,"font_size":0}}]`
	s1.ViewJSON = `{"crop":{"x":1,"y":1,"w":6,"h":4}}`
	s2.Description = `{"type":"doc","content":[{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"준비"}]},` +
		`{"type":"paragraph","content":[{"type":"text","marks":[{"type":"bold"}],"text":"굵게"}]}]}`
	s2.CalloutsJSON = `[{"type":"warning","title":"주의","body":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"저장 먼저"}]}]},"position":"before"}]`
	s3.Hidden = true
	if err := a.SaveSteps(gid, []store.Step{s1, s2, s3}); err != nil {
		t.Fatal(err)
	}

	// 저장소를 닫았다 다시 열어 디스크에 남은 값을 본다
	a.st.Close()
	st2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	d2, err := (&App{st: st2}).OpenGuide(gid)
	if err != nil {
		t.Fatal(err)
	}
	if want := []store.Step{s3, s1, s2}; !reflect.DeepEqual(d2.Steps, want) {
		t.Fatalf("다시 연 스텝 =\n%+v\nwant\n%+v", d2.Steps, want)
	}
	var anns []struct {
		Type   string
		Points []int
	}
	if err := json.Unmarshal([]byte(d2.Steps[1].AnnotationsJSON), &anns); err != nil || len(anns) != 2 ||
		anns[0].Type != "marker" || anns[1].Type != "arrow" || !reflect.DeepEqual(anns[1].Points, []int{1, 1, 5, 4}) {
		t.Errorf("그리기 = %s (%v)", d2.Steps[1].AnnotationsJSON, err)
	}
	var view struct{ Crop struct{ X, Y, W, H int } }
	if err := json.Unmarshal([]byte(d2.Steps[1].ViewJSON), &view); err != nil || view.Crop.W != 6 || view.Crop.H != 4 {
		t.Errorf("자르기 = %s (%v)", d2.Steps[1].ViewJSON, err)
	}
	if g, _ := st2.GetGuide(gid); g.StepCount != 3 {
		t.Errorf("스텝 수 = %d", g.StepCount)
	}
}

func TestApp_SaveStepsRejectsBrokenJSON(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	d, _ := a.OpenGuide(gid)
	good, bad := d.Steps[0], d.Steps[1]
	good.Title = "바뀌면 안 됨"
	bad.AnnotationsJSON = "{"
	if err := a.SaveSteps(gid, []store.Step{good, bad}); !errors.Is(err, errSave) {
		t.Fatalf("깨진 JSON 저장 = %v, errSave여야 한다", err)
	}
	other := d.Steps[2]
	other.ID = 999
	if err := a.SaveSteps(gid, []store.Step{other}); !errors.Is(err, errGone) {
		t.Errorf("없는 스텝 저장 = %v, errGone이어야 한다", err)
	}
	if d2, _ := a.OpenGuide(gid); d2.Steps[0].Title != "하나" {
		t.Errorf("실패한 저장이 쓰였다: %+v", d2.Steps[0])
	}
}

func TestApp_AddStepsDeleteAndClose(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	d, _ := a.OpenGuide(gid)
	s1, s2 := d.Steps[0], d.Steps[1]

	text, err := a.AddTextStep(gid, 1.5, s1.ID)
	if err != nil || text.Kind != "text" || text.Sort != 1.5 || text.ParentID != s1.ID || text.ImageSHA != "" {
		t.Fatalf("글 스텝 = %+v, %v", text, err)
	}
	pic, err := a.AddImageStep(gid, 9, 0, base64.StdEncoding.EncodeToString(encode(t, "jpeg", 20, 10)))
	if err != nil || pic.Kind != "image" || pic.ImageSHA == "" {
		t.Fatalf("그림 스텝 = %+v, %v", pic, err)
	}
	f, _ := a.st.ImageFile(pic.ImageSHA)
	if b, err := os.ReadFile(f); err != nil {
		t.Fatal(err)
	} else if cfg, format, err := image.DecodeConfig(bytes.NewReader(b)); err != nil || format != "png" || cfg.Width != 20 {
		t.Errorf("저장된 그림 형식 %s %dx%d, PNG여야 한다(%v)", format, cfg.Width, cfg.Height, err)
	}
	for _, data := range []string{"!!", base64.StdEncoding.EncodeToString([]byte("그림 아님"))} {
		if _, err := a.AddImageStep(gid, 10, 0, data); !errors.Is(err, errImage) {
			t.Errorf("그림이 아닌 값 = %v, errImage여야 한다", err)
		}
	}

	// 첫째(최상위)를 지우고 둘째를 그 하위로 둔 채 닫는다
	s1.Deleted = true
	s2.ParentID = s1.ID
	if err := a.SaveSteps(gid, []store.Step{s1, s2}); err != nil {
		t.Fatal(err)
	}
	if d, _ := a.OpenGuide(gid); len(d.Deleted) != 1 || d.Deleted[0].ID != s1.ID || len(d.Steps) != 4 {
		t.Fatalf("지운 뒤 남은 %d개, 지운 %+v", len(d.Steps), d.Deleted)
	}
	if err := a.CloseGuide(gid); err != nil {
		t.Fatal(err)
	}
	d, _ = a.OpenGuide(gid)
	if len(d.Deleted) != 0 || len(d.Steps) != 4 {
		t.Fatalf("닫은 뒤 남은 %d개, 지운 %d개", len(d.Steps), len(d.Deleted))
	}
	for _, s := range d.Steps {
		if s.ID == s2.ID && s.ParentID != 0 {
			t.Errorf("지운 스텝의 하위 스텝은 최상위로 남아야 한다: %+v", s)
		}
	}
}

func TestApp_FoldersAndGuides(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	id, err := a.CreateFolder("  인사   팀 ")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "   ", string(make([]rune, 101))} {
		if _, err := a.CreateFolder(bad); !errors.Is(err, errName) {
			t.Errorf("이름 %q = %v, errName이어야 한다", bad, err)
		}
	}
	if err := a.MoveGuide(gid, id); err != nil {
		t.Fatal(err)
	}
	if err := a.RenameGuide(gid, "연차 신청"); err != nil {
		t.Fatal(err)
	}
	fs, _ := a.ListFolders()
	gs, _ := a.ListGuides()
	if len(fs) != 1 || fs[0].Title != "인사 팀" || len(gs) != 1 || gs[0].FolderID != id || gs[0].Title != "연차 신청" {
		t.Fatalf("폴더 %+v, 가이드 %+v", fs, gs)
	}
	if err := a.DeleteFolder(id); err != nil {
		t.Fatal(err)
	}
	if err := a.RenameFolder(id, "x"); !errors.Is(err, errGone) {
		t.Errorf("지운 폴더 이름 바꾸기 = %v, errGone이어야 한다", err)
	}
	if err := a.MoveGuide(gid, id); !errors.Is(err, errGone) { // 외래 키 오류는 디스크 공간 안내가 아니다
		t.Errorf("지운 폴더로 옮기기 = %v, errGone이어야 한다", err)
	}
	if err := a.DeleteGuide(gid); err != nil {
		t.Fatal(err)
	}
	if gs, _ := a.ListGuides(); len(gs) != 0 {
		t.Errorf("지운 뒤 가이드 = %+v", gs)
	}
}

func TestApp_BeforeCloseAsksOnce(t *testing.T) {
	a := &App{}
	if !a.beforeClose(context.Background()) {
		t.Fatal("처음 닫을 때는 막고 프론트에 저장할 기회를 줘야 한다")
	}
	if a.beforeClose(context.Background()) {
		t.Fatal("두 번째 닫기는 막으면 안 된다")
	}
}

func TestApp_CancelCloseRearms(t *testing.T) {
	a := &App{}
	a.beforeClose(context.Background())
	a.CancelClose() // 프론트가 저장에 실패했다
	if !a.beforeClose(context.Background()) {
		t.Fatal("닫기 저장이 실패한 뒤 다시 닫으면 또 막고 저장할 기회를 줘야 한다")
	}
	if a.beforeClose(context.Background()) {
		t.Fatal("프론트가 답하지 않으면 두 번째 닫기는 막으면 안 된다")
	}
}

// pngSized는 머리(IHDR)의 가로·세로만 바꾼 PNG다. pngOf는 머리만 읽고 크기를 거르므로 큰 그림을 실제로 만들지 않는다.
func pngSized(t *testing.T, w, h uint32) string {
	b := encode(t, "png", 1, 1)
	binary.BigEndian.PutUint32(b[16:], w)
	binary.BigEndian.PutUint32(b[20:], h)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return base64.StdEncoding.EncodeToString(b)
}

func TestPngOf_SizeLimits(t *testing.T) {
	for _, c := range []struct {
		w, h uint32
		big  bool
	}{{16000, 2500, false}, {16001, 10, true}, {6000, 6000, false}, {8000, 6000, true}, {16000, 16000, true}} {
		_, _, _, err := pngOf(pngSized(t, c.w, c.h))
		if got := errors.Is(err, errBig); got != c.big || (!c.big && err != nil) {
			t.Errorf("%dx%d: err = %v, 너무 큼 = %v여야 한다", c.w, c.h, err, c.big)
		}
	}
}
