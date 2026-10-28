// 가져오기 저장 테스트: 새 가이드에 스텝을 순서·상위 스텝·칸 그대로 넣는다, 없는 폴더는 폴더 없음,
// 하나라도 틀리면 가이드째 아무것도 넣지 않는다.
package store

import (
	"testing"
)

func importSteps(imageID int64) []ImportStep {
	return []ImportStep{
		{Parent: -1, Kind: "click", Title: "하나", ImageID: imageID, CaptureJSON: `{"dpi_scale":1.5}`,
			AnnotationsJSON: `[{"id":"m"}]`, CalloutsJSON: `[]`, ViewJSON: `{}`, Description: `{"type":"doc"}`},
		{Parent: 0, Kind: "click", Title: "하위", ImageID: imageID, Hidden: true, CaptureJSON: `{}`,
			AnnotationsJSON: `[]`, CalloutsJSON: `[{"type":"tip"}]`, ViewJSON: `{"crop":{"x":1,"y":1,"w":2,"h":2}}`},
		{Parent: -1, Kind: "text", Title: "글", AnnotationsJSON: `[]`, CalloutsJSON: `[]`, ViewJSON: `{}`},
	}
}

func TestImportGuide_InsertsInOrder(t *testing.T) {
	s := open(t)
	fid, err := s.CreateFolder("받은 매뉴얼")
	if err != nil {
		t.Fatal(err)
	}
	img, sha, err := s.PutImage([]byte("png"), 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	gid, err := s.ImportGuide(fid, "가져온 것", importSteps(img))
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.GetGuide(gid)
	if err != nil || g.Title != "가져온 것" || g.FolderID != fid || g.StepCount != 3 {
		t.Fatalf("GetGuide = %+v, %v", g, err)
	}
	steps, err := s.ListSteps(gid)
	if err != nil || len(steps) != 3 {
		t.Fatalf("ListSteps = %d개, %v", len(steps), err)
	}
	a, b, c := steps[0], steps[1], steps[2]
	switch {
	case a.Title != "하나" || a.ImageSHA != sha || a.ParentID != 0 || a.CaptureJSON != `{"dpi_scale":1.5}` || a.Description != `{"type":"doc"}` || a.AnnotationsJSON != `[{"id":"m"}]`:
		t.Errorf("첫 스텝 = %+v", a)
	case b.ParentID != a.ID || !b.Hidden || b.CalloutsJSON != `[{"type":"tip"}]` || b.ViewJSON != `{"crop":{"x":1,"y":1,"w":2,"h":2}}`:
		t.Errorf("하위 스텝 = %+v", b)
	case c.Kind != "text" || c.ImageSHA != "" || c.CaptureJSON != "{}" || c.Sort <= b.Sort:
		t.Errorf("글 스텝 = %+v", c)
	}
}

func TestImportGuide_UnknownFolderIsNone(t *testing.T) {
	s := open(t)
	gid, err := s.ImportGuide(999, "폴더 없음", importSteps(0)[2:])
	if err != nil {
		t.Fatal(err)
	}
	if g, _ := s.GetGuide(gid); g.FolderID != 0 {
		t.Fatalf("FolderID = %d, want 0", g.FolderID)
	}
}

func TestImportGuide_RejectsAtomically(t *testing.T) {
	s := open(t)
	for name, mod := range map[string]func([]ImportStep){
		"그리기가 배열이 아님": func(st []ImportStep) { st[2].AnnotationsJSON = `{}` },
		"뒤를 가리키는 상위":  func(st []ImportStep) { st[0].Parent = 1 },
		"모르는 kind":    func(st []ImportStep) { st[2].Kind = "video" },
	} {
		st := importSteps(0)
		mod(st)
		if _, err := s.ImportGuide(0, name, st); err == nil {
			t.Errorf("%s: 오류가 없다", name)
		}
	}
	if gs, err := s.ListGuides(); err != nil || len(gs) != 0 {
		t.Fatalf("실패한 가져오기가 남겼다: %d개, %v", len(gs), err)
	}
}
