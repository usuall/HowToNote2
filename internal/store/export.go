// 내보내기 저장: 합성 그림 캐시(cache\rendered\<key>.png, 스펙 5.1절)와 문서 모양(export_presets, 스펙 7.3절),
// 안 쓰는 이미지·캐시 정리(컨트롤러 재정 M3-C3, M2 재정 R28).
package store

import (
	"os"
	"path/filepath"
	"strings"
)

// renderedDir은 데이터 폴더 기준 합성 그림 캐시 폴더다. Open이 만든다.
const renderedDir = "cache/rendered"

func validKey(key string) bool {
	return len(key) == 64 && strings.Trim(key, "0123456789abcdef") == ""
}

// RenderedFile은 합성 그림 캐시 파일 경로와 있는지를 돌려준다. key가 소문자 16진수 64자가 아니면 false다(경로 조작 방지).
func (s *Store) RenderedFile(key string) (string, bool) {
	if !validKey(key) {
		return "", false
	}
	p := filepath.Join(s.dir, filepath.FromSlash(renderedDir), key+".png")
	_, err := os.Stat(p)
	return p, err == nil
}

// PutRendered는 합성 그림을 캐시에 쓴다. 임시 파일에 다 쓴 뒤 이름을 바꿔, 끊겨도 반쪽 PNG가 남지 않는다.
func (s *Store) PutRendered(key string, png []byte) error {
	if !validKey(key) {
		return os.ErrInvalid
	}
	p := filepath.Join(s.dir, filepath.FromSlash(renderedDir), key+".png")
	if err := os.WriteFile(p+".tmp", png, 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// Preset은 문서 모양 하나다. Options는 export.Options의 JSON이고, 빠진 칸은 기본값으로 읽는다.
type Preset struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Format  string `json:"format"` // 마지막으로 고른 형식(pdf, hwpx, docx, html)
	Options string `json:"options"`
}

// ListPresets는 문서 모양을 만든 순서대로 돌려준다. 첫 항목은 마이그레이션이 넣은 '기본'이다.
func (s *Store) ListPresets() ([]Preset, error) {
	rows, err := s.db.Query(`SELECT id, name, format, options_json FROM export_presets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Preset{}
	for rows.Next() {
		var p Preset
		if err := rows.Scan(&p.ID, &p.Name, &p.Format, &p.Options); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SavePreset은 ID가 0이면 새로 넣고, 아니면 그 문서 모양을 고친다. 저장된 ID를 돌려준다.
func (s *Store) SavePreset(p Preset) (int64, error) {
	if p.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO export_presets(name, format, options_json) VALUES(?, ?, ?)`, p.Name, p.Format, p.Options)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	return p.ID, s.exec1(`UPDATE export_presets SET name = ?, format = ?, options_json = ? WHERE id = ?`, p.Name, p.Format, p.Options, p.ID)
}

func (s *Store) DeletePreset(id int64) error {
	return s.exec1(`DELETE FROM export_presets WHERE id = ?`, id)
}

// PruneImages는 어떤 스텝도 쓰지 않는 이미지 행과 PNG 파일을 지우고 지운 개수를 돌려준다.
// 지운 표시만 된(deleted=1) 스텝은 가이드를 닫을 때까지 되살릴 수 있어 여전히 "쓰는 것"으로 친다.
// 먼저 트랜잭션으로 행을 지운 뒤 파일을 지워서, 그사이 끊겨도 "파일 없는 참조 행"은 절대 남지 않는다
// (반대로 "주인 없는 파일"이 남는 것은 괜찮다 — 다음 PruneImages가 정리한다).
// 녹화 중(PutImage가 쓰고 AppendStep이 아직 안 쓴 사이)에는 부르면 안 된다 — 그 틈의 이미지가 고아로 보인다.
func (s *Store) PruneImages() (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // Commit 뒤에는 아무 일도 하지 않는다
	const orphan = `images WHERE id NOT IN (SELECT image_id FROM steps WHERE image_id IS NOT NULL)`
	rows, err := tx.Query(`SELECT path FROM ` + orphan)
	if err != nil {
		return 0, err
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return 0, err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()
	if _, err := tx.Exec(`DELETE FROM ` + orphan); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	for _, p := range paths {
		os.Remove(filepath.Join(s.dir, filepath.FromSlash(p))) // 이미 없어도 괜찮다 — DB가 기준이다
	}
	return len(paths), nil
}

// PruneRendered는 cache\rendered 안의 PNG 중 key가 keep에 없는 것과 남은 .tmp 파일을 지우고 지운 개수를 돌려준다.
// 녹화 중에는 부르면 안 된다(저장 전 캐시 키가 아직 keep에 없을 수 있다).
func (s *Store) PruneRendered(keep map[string]bool) (int, error) {
	dir := filepath.Join(s.dir, filepath.FromSlash(renderedDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		remove := strings.HasSuffix(name, ".tmp")
		if !remove && strings.HasSuffix(name, ".png") {
			remove = !keep[strings.TrimSuffix(name, ".png")]
		}
		if remove {
			if err := os.Remove(filepath.Join(dir, name)); err == nil {
				n++
			}
		}
	}
	return n, nil
}
