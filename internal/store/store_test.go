// 저장소 테스트: 마이그레이션, 가이드·스텝 순서, 내용 주소 이미지, 설정 파일. 임시 폴더만 쓴다.
package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpen_MigratesAndReopens(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ver, fk int
	s.db.QueryRow("PRAGMA user_version").Scan(&ver)
	s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if ver != 3 || fk != 1 {
		t.Errorf("user_version = %d, foreign_keys = %d, 3과 1이어야 한다", ver, fk)
	}
	for _, table := range []string{"folders", "guides", "steps", "images", "export_presets"} {
		var n int
		s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
		if n != 1 {
			t.Errorf("%s 테이블이 없다", table)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "images")); err != nil {
		t.Errorf("images 폴더가 없다: %v", err)
	}
	s.Close()
	s2, err := Open(dir) // 두 번째 열기는 마이그레이션을 다시 하지 않는다
	if err != nil {
		t.Fatalf("다시 열기: %v", err)
	}
	s2.Close()
}

func TestOpen_RejectsNewerDatabase(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.db.Exec("PRAGMA user_version = 99")
	s.Close()
	if s2, err := Open(dir); err == nil {
		s2.Close()
		t.Fatal("더 새 버전 데이터베이스는 거부해야 한다")
	}
}

func TestAppendStep_KeepsOrderAndCounts(t *testing.T) {
	s := open(t)
	gid, err := s.CreateGuide("테스트 가이드")
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"하나", "둘", "셋"} {
		if _, err := s.AppendStep(gid, NewStep{Kind: "click", Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	steps, err := s.ListSteps(gid)
	if err != nil || len(steps) != 3 {
		t.Fatalf("스텝 %d개, 오류 %v", len(steps), err)
	}
	for i, want := range []string{"하나", "둘", "셋"} {
		st := steps[i]
		if st.Title != want || st.Sort != float64(i+1) || st.CaptureJSON != "{}" || st.AnnotationsJSON != "[]" || st.ImageSHA != "" {
			t.Errorf("스텝 %d = %+v", i, st)
		}
	}
	gs, err := s.ListGuides()
	if err != nil || len(gs) != 1 || gs[0].Title != "테스트 가이드" || gs[0].StepCount != 3 {
		t.Errorf("가이드 목록 = %+v, 오류 %v", gs, err)
	}
	g, err := s.GetGuide(gid)
	if err != nil || g.StepCount != 3 {
		t.Errorf("가이드 = %+v, 오류 %v", g, err)
	}
}

func TestAppendStep_RejectsUnknownKind(t *testing.T) {
	s := open(t)
	gid, _ := s.CreateGuide("g")
	if _, err := s.AppendStep(gid, NewStep{Kind: "drag"}); err == nil {
		t.Fatal("스펙에 없는 kind는 거부해야 한다")
	}
}

func TestPutImage_ContentAddressed(t *testing.T) {
	s := open(t)
	data := []byte("\x89PNG 가짜 이미지")
	id1, sha1, err := s.PutImage(data, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	id2, sha2, err := s.PutImage(data, 10, 20)
	if err != nil || id1 != id2 || sha1 != sha2 {
		t.Fatalf("같은 내용인데 (%d,%s) != (%d,%s), 오류 %v", id1, sha1, id2, sha2, err)
	}
	path, ok := s.ImageFile(sha1)
	if !ok || filepath.Base(filepath.Dir(path)) != sha1[:2] || filepath.Base(path) != sha1+".png" {
		t.Errorf("이미지 경로 = %q", path)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != string(data) {
		t.Errorf("파일 내용이 다르다: %v", err)
	}
	gid, _ := s.CreateGuide("g")
	if _, err := s.AppendStep(gid, NewStep{Kind: "click", ImageID: id1}); err != nil {
		t.Fatal(err)
	}
	if steps, _ := s.ListSteps(gid); len(steps) != 1 || steps[0].ImageSHA != sha1 {
		t.Errorf("스텝의 이미지 해시 = %+v", steps)
	}
	for _, bad := range []string{"", "../data", strings.Repeat("g", 64), strings.Repeat("A", 64)} {
		if _, ok := s.ImageFile(bad); ok {
			t.Errorf("ImageFile(%q)은 거부해야 한다", bad)
		}
	}
}

func TestSortBetween(t *testing.T) {
	cases := []struct {
		prev, next float64
		hasNext    bool
		want       float64
	}{
		{0, 0, false, 1},
		{3, 0, false, 4},
		{3.5, 0, false, 4},
		{1, 2, true, 1.5},
		{1.5, 2, true, 1.75},
		{0, 1, true, 0.5},
	}
	for _, c := range cases {
		if got := SortBetween(c.prev, c.next, c.hasNext); got != c.want {
			t.Errorf("SortBetween(%v, %v, %v) = %v, want %v", c.prev, c.next, c.hasNext, got, c.want)
		}
	}
}

func TestLoadSettings(t *testing.T) {
	dir := t.TempDir()
	if s, err := LoadSettings(dir); err != nil || s != (Settings{}) {
		t.Fatalf("파일이 없으면 기본값이어야 한다: %+v, %v", s, err)
	}
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{"data_dir":"D:\\HowtoNote","tesseract_path":"C:\\t.exe","title_style":"하세요"}`), 0o644)
	s, err := LoadSettings(dir)
	if err != nil || s.DataDir != `D:\HowtoNote` || s.TesseractPath != `C:\t.exe` || s.TitleStyle != "하세요" {
		t.Errorf("설정 = %+v, 오류 %v", s, err)
	}
	os.WriteFile(p, []byte("{"), 0o644)
	if _, err := LoadSettings(dir); err == nil {
		t.Error("깨진 JSON은 오류여야 한다")
	}
}

// 첫 화면 카드 썸네일은 목록 순서에서 처음 보이는 그림 스텝이다(UI 설계 7절). 글 스텝, 지운 스텝, 숨긴 스텝,
// 숨긴 상위의 하위 스텝은 건너뛰고, 그림 스텝이 없으면 빈 값이다.
func TestListGuides_ThumbSHA(t *testing.T) {
	s := open(t)
	gid, err := s.CreateGuide("썸네일")
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.CreateGuide("빈 매뉴얼")
	if err != nil {
		t.Fatal(err)
	}
	shas := map[string]string{}
	step := func(name string) int64 {
		t.Helper()
		n := NewStep{Kind: "text", Title: "스텝", CaptureJSON: "{}"}
		if name != "" {
			id, sha, err := s.PutImage([]byte("png-"+name), 1, 1)
			if err != nil {
				t.Fatal(err)
			}
			n.Kind, n.ImageID, shas[name] = "click", id, sha
		}
		id, err := s.AppendStep(gid, n)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	step("") // 글 스텝: 그림이 없다
	deleted := step("지움")
	hidden := step("숨김")
	child := step("숨긴 상위의 하위")
	step("첫 보이는 그림")
	step("그다음")
	exec(`UPDATE steps SET deleted = 1 WHERE id = ?`, deleted)
	exec(`UPDATE steps SET hidden = 1 WHERE id = ?`, hidden)
	exec(`UPDATE steps SET parent_id = ? WHERE id = ?`, hidden, child)

	gs, err := s.ListGuides()
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]string{}
	for _, g := range gs {
		got[g.ID] = g.ThumbSHA
	}
	if got[gid] != shas["첫 보이는 그림"] || got[empty] != "" {
		t.Errorf("썸네일 %q·%q, %q·빈 값이어야 한다", got[gid], got[empty], shas["첫 보이는 그림"])
	}
	if g, err := s.GetGuide(gid); err != nil || g.ThumbSHA != shas["첫 보이는 그림"] {
		t.Errorf("GetGuide 썸네일 %q, 오류 %v", g.ThumbSHA, err)
	}
}
