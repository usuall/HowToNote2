// 편집기 저장 테스트: 고친 칸 쓰기, 형식 검사와 원자성, 다른 가이드 거부, 새 스텝 넣기, 삭제 표시·영구 삭제.
package store

import (
	"errors"
	"testing"
)

// guideWith는 click 스텝 n개가 있는 가이드를 만들고 스텝을 돌려준다.
func guideWith(t *testing.T, s *Store, n int) (int64, []Step) {
	t.Helper()
	gid, err := s.CreateGuide("g")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err := s.AppendStep(gid, NewStep{Kind: "click", Title: "스텝", CaptureJSON: `{"dpi_scale":1}`}); err != nil {
			t.Fatal(err)
		}
	}
	steps, err := s.ListSteps(gid)
	if err != nil || len(steps) != n {
		t.Fatalf("스텝 %d개, 오류 %v", len(steps), err)
	}
	return gid, steps
}

func TestUpdateSteps_WritesEditableFields(t *testing.T) {
	s := open(t)
	gid, steps := guideWith(t, s, 2)
	a := steps[1]
	a.ParentID = steps[0].ID
	a.Sort = 1.5
	a.Title = "고친 제목"
	a.Description = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"설명"}]}]}`
	a.Hidden = true
	a.AnnotationsJSON = `[{"id":"r","type":"rect","x":1,"y":2,"w":3,"h":4,"style":{"stroke":"#E5383B","fill":"","width":4,"opacity":1,"font_size":0}}]`
	a.CalloutsJSON = `[{"type":"tip","title":"도움말","body":null,"position":"after"}]`
	a.ViewJSON = `{"crop":{"x":0,"y":0,"w":10,"h":10}}`
	a.Kind, a.CaptureJSON = "text", "{}" // 녹화 결과 칸은 무시된다
	if err := s.UpdateSteps(gid, []Step{a}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ListSteps(gid)
	want := a
	want.Kind, want.CaptureJSON = "click", `{"dpi_scale":1}`
	if len(got) != 2 || got[1] != want {
		t.Errorf("저장된 스텝 =\n%+v\nwant\n%+v", got[1], want)
	}
}

func TestUpdateSteps_RejectsBadRowsAtomically(t *testing.T) {
	s := open(t)
	gid, steps := guideWith(t, s, 2)
	other, others := guideWith(t, s, 1)
	good := steps[0]
	good.Title = "바뀌면 안 됨"
	bad := []func(st *Step){
		func(st *Step) { st.AnnotationsJSON = "{" },
		func(st *Step) { st.AnnotationsJSON = "{}" },
		func(st *Step) { st.CalloutsJSON = "" },
		func(st *Step) { st.ViewJSON = "[]" },
		func(st *Step) { st.Description = "글자" },
		func(st *Step) { st.ParentID = st.ID },
		func(st *Step) { st.ParentID = others[0].ID }, // 다른 가이드의 스텝
		func(st *Step) { st.ID = others[0].ID },       // 다른 가이드의 행
	}
	for i, f := range bad {
		b := steps[1]
		f(&b)
		if err := s.UpdateSteps(gid, []Step{good, b}); err == nil {
			t.Errorf("%d번 잘못된 행을 받아들였다: %+v", i, b)
		}
	}
	if got, _ := s.ListSteps(gid); got[0].Title != "스텝" {
		t.Errorf("실패한 저장이 일부 쓰였다: %+v", got[0])
	}
	if got, _ := s.ListSteps(other); got[0] != others[0] {
		t.Errorf("다른 가이드 스텝이 바뀌었다: %+v", got[0])
	}
}

func TestInsertStep_PlacesBySortAndParent(t *testing.T) {
	s := open(t)
	gid, steps := guideWith(t, s, 2)
	img, sha, _ := s.PutImage([]byte("png"), 1, 1)
	st, err := s.InsertStep(gid, steps[0].ID, 1.5, NewStep{Kind: "image", ImageID: img})
	if err != nil {
		t.Fatal(err)
	}
	if st.Kind != "image" || st.ImageSHA != sha || st.ParentID != steps[0].ID || st.Sort != 1.5 ||
		st.AnnotationsJSON != "[]" || st.CalloutsJSON != "[]" || st.ViewJSON != "{}" || st.Description != "" {
		t.Errorf("넣은 스텝 = %+v", st)
	}
	got, _ := s.ListSteps(gid)
	if len(got) != 3 || got[1].ID != st.ID {
		t.Errorf("순서 = %+v", got)
	}
	_, others := guideWith(t, s, 1)
	if _, err := s.InsertStep(gid, others[0].ID, 9, NewStep{Kind: "text"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("다른 가이드 스텝을 상위로 = %v, ErrNotFound여야 한다", err)
	}
	if _, err := s.InsertStep(gid, 0, 9, NewStep{Kind: "drag"}); err == nil {
		t.Error("스펙에 없는 kind는 거부해야 한다")
	}
}

func TestSetStepTitle(t *testing.T) {
	s := open(t)
	gid, steps := guideWith(t, s, 1)
	if err := s.SetStepTitle(steps[0].ID, "'저장' 버튼을 더블클릭합니다."); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListSteps(gid); got[0].Title != "'저장' 버튼을 더블클릭합니다." {
		t.Errorf("제목 = %q", got[0].Title)
	}
	if err := s.SetStepTitle(999, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("없는 스텝 = %v", err)
	}
}

func TestDeleted_ListAndPurge(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	gid, steps := guideWith(t, s, 3)
	other, others := guideWith(t, s, 1)
	// 1번(최상위)을 지운다. 2번은 1번의 하위 스텝이다.
	a, b := steps[0], steps[1]
	a.Deleted = true
	b.ParentID = a.ID
	o := others[0]
	o.Deleted = true
	if err := s.UpdateSteps(gid, []Step{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateSteps(other, []Step{o}); err != nil {
		t.Fatal(err)
	}
	live, _ := s.ListSteps(gid)
	dead, _ := s.ListDeletedSteps(gid)
	g, _ := s.GetGuide(gid)
	if len(live) != 2 || len(dead) != 1 || dead[0].ID != a.ID || !dead[0].Deleted || g.StepCount != 2 {
		t.Fatalf("남은 %d개, 지운 %+v, 가이드 스텝 수 %d", len(live), dead, g.StepCount)
	}

	if err := s.PurgeDeleted(gid); err != nil {
		t.Fatal(err)
	}
	live, _ = s.ListSteps(gid)
	dead, _ = s.ListDeletedSteps(gid)
	if len(live) != 2 || live[0].ID != b.ID || live[0].ParentID != 0 || len(dead) != 0 {
		t.Errorf("영구 삭제 뒤 남은 %+v, 지운 %+v (하위 스텝은 최상위로 남아야 한다)", live, dead)
	}
	if d, _ := s.ListDeletedSteps(other); len(d) != 1 {
		t.Errorf("다른 가이드의 지운 스텝까지 지웠다")
	}

	// 닫지 못하고 끝난 가이드의 지운 스텝은 다음에 열 때 지운다
	s.Close()
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if d, _ := s2.ListDeletedSteps(other); len(d) != 0 {
		t.Errorf("다시 열 때 지운 스텝이 남았다: %+v", d)
	}
}
