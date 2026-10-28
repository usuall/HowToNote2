package export

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"testing"
	"time"

	"howtonote/internal/store"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// guideFixture는 스텝 4개(최상위, 하위, 숨김, 글 스텝)가 있는 가이드를 만든다. 그림은 첫 두 스텝에만 있다.
func guideFixture(t *testing.T) (*store.Store, int64, []store.Step) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	gid, _ := st.CreateGuide("주간 보고 쓰기")
	img, _, err := st.PutImage(pngBytes(t, 40, 30), 40, 30)
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"'보고서' 메뉴를 선택합니다.", "'새로 만들기' 버튼을 클릭합니다.", "숨긴 스텝", ""} {
		n := store.NewStep{Kind: "click", Title: title, ImageID: img}
		if title == "" {
			n = store.NewStep{Kind: "text"}
		}
		if _, err := st.AppendStep(gid, n); err != nil {
			t.Fatal(err)
		}
	}
	steps, _ := st.ListSteps(gid)
	steps[1].ParentID = steps[0].ID
	steps[1].ViewJSON = `{"crop":{"x":0,"y":0,"w":20,"h":10}}`
	steps[1].CalloutsJSON = `[{"type":"tip","title":"","body":null,"position":"before"}]`
	steps[2].Hidden = true
	steps[3].Description = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"끝"}]}]}`
	if err := st.UpdateSteps(gid, steps); err != nil {
		t.Fatal(err)
	}
	steps, _ = st.ListSteps(gid)
	return st, gid, steps
}

// Numbers는 frontend/src/editor/model.ts의 numbering과 같아야 한다(컨트롤러 재정 M3-C2).
// 숨긴 최상위 스텝의 하위 스텝도 숨긴 것으로 본다 — 첫 스텝이 숨긴 최상위일 때도 마찬가지다.
func TestNumbers_MatchesEditorRule(t *testing.T) {
	cases := []struct {
		name  string
		steps []store.Step
		want  []string
	}{
		{
			// model.test.ts 'numbering은 최상위 1, 하위 1.1이고 숨긴 스텝은 번호가 없다'와 같은 입력
			name:  "기본",
			steps: []store.Step{{ID: 1}, {ID: 2, ParentID: 1}, {ID: 3, ParentID: 1, Hidden: true}, {ID: 4, ParentID: 1}, {ID: 5}},
			want:  []string{"1", "1.1", "", "1.2", "2"},
		},
		{
			// [1, 2←1, 3(hidden), 4←3]: 숨긴 최상위 스텝(3)의 하위 스텝(4)도 번호가 없다(M3-C2)
			name:  "숨긴 최상위의 하위",
			steps: []store.Step{{ID: 1}, {ID: 2, ParentID: 1}, {ID: 3, Hidden: true}, {ID: 4, ParentID: 3}},
			want:  []string{"1", "1.1", "", ""},
		},
		{
			// [1(hidden), 2←1]: 숨긴 최상위가 첫 스텝이어도 그 하위 스텝은 번호가 없다(M3-C2)
			name:  "숨긴 첫 스텝의 하위",
			steps: []store.Step{{ID: 1, Hidden: true}, {ID: 2, ParentID: 1}},
			want:  []string{"", ""},
		},
	}
	for _, c := range cases {
		got := Numbers(c.steps)
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Fatalf("%s: Numbers = %q, want %q", c.name, got, c.want)
			}
		}
	}
}

func TestBuild_NeedsRenderedImages(t *testing.T) {
	st, gid, _ := guideFixture(t)
	if _, err := Build(st, gid, time.Now()); !errors.Is(err, ErrNotRendered) {
		t.Fatalf("합성 그림 없이 Build = %v, ErrNotRendered여야 한다", err)
	}
}

func TestBuild_FromStore(t *testing.T) {
	st, gid, steps := guideFixture(t)
	for i, s := range steps[:2] { // 숨긴 스텝(2)은 내보내지 않으므로 그리지 않는다
		if err := st.PutRendered(RenderKey(s), pngBytes(t, 20+i, 10)); err != nil {
			t.Fatal(err)
		}
	}
	d, err := Build(st, gid, time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "주간 보고 쓰기" || d.Date != "2026. 10. 21." || len(d.Steps) != 3 {
		t.Fatalf("문서 = %+v", d)
	}
	a, b, c := d.Steps[0], d.Steps[1], d.Steps[2]
	if a.Heading() != "1. '보고서' 메뉴를 선택합니다." || b.Heading() != "1.1 '새로 만들기' 버튼을 클릭합니다." || !b.Sub || c.Heading() != "2." {
		t.Errorf("제목 = %q, %q, %q", a.Heading(), b.Heading(), c.Heading())
	}
	if a.Image == nil || a.Image.Width != 20 || b.Image == nil || b.Image.Width != 21 || c.Image != nil {
		t.Errorf("그림 = %+v, %+v, %+v", a.Image, b.Image, c.Image)
	}
	if len(b.Before) != 1 || b.Before[0].Heading() != "도움말" || len(c.Blocks) != 1 || Text(c.Blocks[0].Runs) != "끝" {
		t.Errorf("안내 상자·설명 = %+v, %+v", b.Before, c.Blocks)
	}
}

// 캐시 PNG가 깨지면(디스크 오류 등) rendered가 지우고 ErrNotRendered를 돌려준다(검수 Minor 3).
// 지운 뒤에는 RenderedFile(Cached와 같은 길)도 없는 것으로 봐야, 프론트가 다음 ExportPlan에서 다시 그린다.
func TestBuild_HealsCorruptCache(t *testing.T) {
	st, gid, steps := guideFixture(t)
	for i, s := range steps[:2] {
		if err := st.PutRendered(RenderKey(s), pngBytes(t, 20+i, 10)); err != nil {
			t.Fatal(err)
		}
	}
	path, ok := st.RenderedFile(RenderKey(steps[0]))
	if !ok {
		t.Fatal("캐시 파일이 없다")
	}
	if err := os.WriteFile(path, []byte("쓰레기"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(st, gid, time.Now()); !errors.Is(err, ErrNotRendered) {
		t.Fatalf("깨진 캐시 PNG로 Build = %v, ErrNotRendered여야 한다", err)
	}
	if _, ok := st.RenderedFile(RenderKey(steps[0])); ok {
		t.Error("깨진 캐시 PNG가 지워지지 않았다")
	}
}

func TestRenderKey_ChangesWithDrawingAndCrop(t *testing.T) {
	s := store.Step{ImageSHA: "ab", AnnotationsJSON: "[]", ViewJSON: "{}"}
	k := RenderKey(s)
	if len(k) != 64 || k != RenderKey(s) {
		t.Fatalf("키 = %q", k)
	}
	for _, change := range []func(*store.Step){
		func(s *store.Step) { s.ImageSHA = "cd" },
		func(s *store.Step) { s.AnnotationsJSON = `[{"id":"a"}]` },
		func(s *store.Step) { s.ViewJSON = `{"crop":{"x":1,"y":1,"w":2,"h":2}}` },
	} {
		c := s
		change(&c)
		if RenderKey(c) == k {
			t.Errorf("바뀐 스텝 %+v의 키가 같다", c)
		}
	}
}
