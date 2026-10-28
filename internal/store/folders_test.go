// 폴더·가이드 관리 테스트: 폴더 순서, 이름 바꾸기, 가이드 옮기기, 폴더를 지워도 가이드는 남음, 가이드 지우면 스텝도 지움.
package store

import (
	"errors"
	"testing"
)

func TestFolders_CreateRenameDelete(t *testing.T) {
	s := open(t)
	if fs, err := s.ListFolders(); err != nil || len(fs) != 0 {
		t.Fatalf("처음 폴더 목록 = %+v, %v", fs, err)
	}
	a, err := s.CreateFolder("인사")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.CreateFolder("회계")
	if err := s.RenameFolder(a, "인사팀"); err != nil {
		t.Fatal(err)
	}
	fs, _ := s.ListFolders()
	if len(fs) != 2 || fs[0] != (Folder{a, "인사팀"}) || fs[1] != (Folder{b, "회계"}) {
		t.Errorf("폴더 목록 = %+v", fs)
	}
	if err := s.DeleteFolder(b); err != nil {
		t.Fatal(err)
	}
	if fs, _ := s.ListFolders(); len(fs) != 1 {
		t.Errorf("지운 뒤 폴더 = %+v", fs)
	}
	if err := s.RenameFolder(b, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("없는 폴더 이름 바꾸기 = %v, ErrNotFound여야 한다", err)
	}
	if err := s.DeleteFolder(b); !errors.Is(err, ErrNotFound) {
		t.Errorf("없는 폴더 지우기 = %v, ErrNotFound여야 한다", err)
	}
}

func TestGuides_MoveRenameDelete(t *testing.T) {
	s := open(t)
	f, _ := s.CreateFolder("인사")
	g, _ := s.CreateGuide("휴가 신청")
	s.AppendStep(g, NewStep{Kind: "click", Title: "하나"})
	if err := s.MoveGuide(g, f); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameGuide(g, "연차 신청"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGuide(g)
	if err != nil || got.FolderID != f || got.Title != "연차 신청" || got.StepCount != 1 {
		t.Fatalf("가이드 = %+v, %v", got, err)
	}
	if err := s.MoveGuide(g, 999); err == nil {
		t.Error("없는 폴더로 옮기면 오류여야 한다")
	}
	// 폴더를 지워도 가이드는 남고 폴더 없음이 된다
	if err := s.DeleteFolder(f); err != nil {
		t.Fatal(err)
	}
	if gs, _ := s.ListGuides(); len(gs) != 1 || gs[0].FolderID != 0 {
		t.Errorf("폴더를 지운 뒤 가이드 = %+v", gs)
	}
	if err := s.DeleteGuide(g); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM steps`).Scan(&n)
	if gs, _ := s.ListGuides(); len(gs) != 0 || n != 0 {
		t.Errorf("가이드를 지운 뒤 가이드 %d개, 스텝 %d개", len(gs), n)
	}
	if err := s.RenameGuide(g, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("없는 가이드 이름 바꾸기 = %v", err)
	}
}
