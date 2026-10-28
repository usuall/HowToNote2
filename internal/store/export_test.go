// 내보내기 저장 테스트: 합성 그림 캐시(폴더, 원자적 쓰기, 키 검사)와 문서 모양(기본 항목, 저장·고치기·지우기), 안 쓰는 이미지·캐시 정리.
package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRendered_PutAndFind(t *testing.T) {
	s := open(t)
	key := strings.Repeat("ab", 32)
	if _, ok := s.RenderedFile(key); ok {
		t.Fatal("쓰기 전에 있다고 한다")
	}
	if err := s.PutRendered(key, []byte("png")); err != nil {
		t.Fatal(err)
	}
	p, ok := s.RenderedFile(key)
	if !ok || filepath.Base(filepath.Dir(p)) != "rendered" || filepath.Base(filepath.Dir(filepath.Dir(p))) != "cache" {
		t.Fatalf("RenderedFile = %q, %v (cache/rendered/<key>.png여야 한다)", p, ok)
	}
	if b, _ := os.ReadFile(p); string(b) != "png" {
		t.Errorf("내용 = %q", b)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Error("임시 파일이 남았다")
	}
	for _, bad := range []string{"", "../x", strings.Repeat("AB", 32), strings.Repeat("a", 63)} {
		if _, ok := s.RenderedFile(bad); ok {
			t.Errorf("RenderedFile(%q)가 true다", bad)
		}
		if err := s.PutRendered(bad, []byte("x")); err == nil {
			t.Errorf("PutRendered(%q)가 받아들였다", bad)
		}
	}
}

func TestPresets_DefaultSaveDelete(t *testing.T) {
	s := open(t)
	ps, err := s.ListPresets()
	if err != nil || len(ps) != 1 || ps[0].Name != "기본" || ps[0].Format != "pdf" || ps[0].Options != "{}" {
		t.Fatalf("처음 문서 모양 = %+v, %v", ps, err)
	}
	id, err := s.SavePreset(Preset{Name: "보고용", Format: "hwpx", Options: `{"toc":false}`})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePreset(Preset{ID: id, Name: "보고용 2", Format: "docx", Options: `{}`}); err != nil {
		t.Fatal(err)
	}
	ps, _ = s.ListPresets()
	if len(ps) != 2 || ps[1] != (Preset{ID: id, Name: "보고용 2", Format: "docx", Options: `{}`}) {
		t.Errorf("고친 뒤 = %+v", ps)
	}
	if _, err := s.SavePreset(Preset{ID: 999, Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("없는 문서 모양 고치기 = %v", err)
	}
	if err := s.DeletePreset(id); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePreset(id); !errors.Is(err, ErrNotFound) {
		t.Errorf("두 번 지우기 = %v", err)
	}
}

// TestPruneImages는 컨트롤러 재정 M3-C3: 어떤 스텝도 쓰지 않는 이미지만 지우고, 지운 표시만 된(가이드를 아직 안 닫은)
// 스텝과 공유 이미지는 살아남는지 본다.
func TestPruneImages(t *testing.T) {
	s := open(t)
	gid, _ := s.CreateGuide("g")
	id1, sha1, err := s.PutImage([]byte("img1"), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	id2, sha2, err := s.PutImage([]byte("img2"), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	path1, _ := s.ImageFile(sha1)
	path2, _ := s.ImageFile(sha2)

	st1, err := s.AppendStep(gid, NewStep{Kind: "image", ImageID: id1})
	if err != nil {
		t.Fatal(err)
	}
	st2, err := s.AppendStep(gid, NewStep{Kind: "image", ImageID: id1}) // id1을 공유
	if err != nil {
		t.Fatal(err)
	}
	st3, err := s.AppendStep(gid, NewStep{Kind: "image", ImageID: id2})
	if err != nil {
		t.Fatal(err)
	}

	markDeleted := func(id int64) {
		steps, err := s.ListSteps(gid)
		if err != nil {
			t.Fatal(err)
		}
		for i := range steps {
			if steps[i].ID == id {
				steps[i].Deleted = true
			}
		}
		if err := s.UpdateSteps(gid, steps); err != nil {
			t.Fatal(err)
		}
	}

	// 지운 표시만 된 스텝(가이드를 아직 닫지 않음)은 이미지를 계속 쓰는 것으로 친다.
	markDeleted(st3)
	if n, err := s.PruneImages(); err != nil || n != 0 {
		t.Fatalf("지운 표시만 된 스텝의 이미지를 지웠다: n=%d, %v", n, err)
	}
	if _, err := os.Stat(path2); err != nil {
		t.Error("지운 표시만 된 스텝의 이미지 파일이 없어졌다")
	}

	// 가이드를 닫아 영구히 지우면(PurgeDeleted) id2는 고아다.
	if err := s.PurgeDeleted(gid); err != nil {
		t.Fatal(err)
	}
	n, err := s.PruneImages()
	if err != nil || n != 1 {
		t.Fatalf("PruneImages = %d, %v, 1개여야 한다", n, err)
	}
	if _, err := os.Stat(path2); !os.IsNotExist(err) {
		t.Error("고아 이미지 파일이 남아 있다")
	}

	// id1은 st1·st2 둘 다 쓴다. st2를 지우고 영구 삭제해도 st1이 남아 있으므로 살아야 한다.
	markDeleted(st2)
	if err := s.PurgeDeleted(gid); err != nil {
		t.Fatal(err)
	}
	if n, err := s.PruneImages(); err != nil || n != 0 {
		t.Fatalf("st1이 쓰는 공유 이미지를 지웠다: n=%d, %v", n, err)
	}
	if _, err := os.Stat(path1); err != nil {
		t.Error("st1이 쓰는 공유 이미지 파일이 없어졌다")
	}
	_ = st1
}

// TestPruneRendered는 컨트롤러 재정 M3-C3: keep에 없는 캐시 PNG와 남은 .tmp 파일을 지우는지 본다.
func TestPruneRendered(t *testing.T) {
	s := open(t)
	keyA := strings.Repeat("aa", 32)
	keyB := strings.Repeat("bb", 32)
	keyC := strings.Repeat("cc", 32)
	for _, k := range []string{keyA, keyB, keyC} {
		if err := s.PutRendered(k, []byte("png")); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(s.dir, "cache", "rendered")
	if err := os.WriteFile(filepath.Join(dir, "stray.tmp"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := s.PruneRendered(map[string]bool{keyA: true})
	if err != nil || n != 3 { // keyB.png, keyC.png, stray.tmp
		t.Fatalf("PruneRendered = %d, %v, 3개여야 한다", n, err)
	}
	if _, ok := s.RenderedFile(keyA); !ok {
		t.Error("keep에 있는 키가 지워졌다")
	}
	if _, ok := s.RenderedFile(keyB); ok {
		t.Error("keep에 없는 키가 남아 있다")
	}
	if _, err := os.Stat(filepath.Join(dir, "stray.tmp")); !os.IsNotExist(err) {
		t.Error(".tmp 파일이 남아 있다")
	}
}
