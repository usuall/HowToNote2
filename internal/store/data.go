// 가이드·스텝·이미지 읽기와 쓰기. 이미지는 내용 해시(sha256) 주소로 한 번만 저장한다(스펙 5.1절).
package store

import (
	"cmp"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Guide struct {
	ID        int64  `json:"id"`
	FolderID  int64  `json:"folderId"` // 0이면 폴더 없음
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"` // RFC 3339 UTC
	StepCount int    `json:"stepCount"`
	ThumbSHA  string `json:"thumbSha"` // 첫 화면 카드 썸네일: 목록 순서에서 처음 보이는 그림 스텝의 그림. 없으면 빈 문자열
}

type Step struct {
	ID              int64   `json:"id"`
	GuideID         int64   `json:"guideId"`
	ParentID        int64   `json:"parentId"` // 0이면 최상위. 하위 스텝은 한 단계뿐이다(번호 1.1)
	Sort            float64 `json:"sort"`
	Kind            string  `json:"kind"`
	Title           string  `json:"title"`
	Description     string  `json:"description"` // Tiptap JSON. 비었으면 빈 문자열
	ImageSHA        string  `json:"imageSha"`    // 이미지가 없으면 빈 문자열
	Hidden          bool    `json:"hidden"`
	Deleted         bool    `json:"deleted"` // 편집기에서 지웠지만 가이드를 닫기 전이라 되살릴 수 있는 스텝
	CaptureJSON     string  `json:"captureJson"`
	AnnotationsJSON string  `json:"annotationsJson"`
	CalloutsJSON    string  `json:"calloutsJson"`
	ViewJSON        string  `json:"viewJson"`
}

// NewStep은 AppendStep에 넘기는 값이다.
type NewStep struct {
	Kind            string // click, type, manual, image, text
	Title           string
	ImageID         int64 // 0이면 이미지 없음
	CaptureJSON     string
	AnnotationsJSON string
}

// guideThumb는 가이드의 썸네일 그림 해시다. 편집기(fixParents)가 하위 스텝을 늘 상위 스텝 바로 뒤 sort에 두므로
// sort, id 순서가 곧 목록 순서다. 지운 스텝, 숨긴 스텝, 숨긴 상위의 하위 스텝, 그림 없는 스텝은 건너뛴다.
const guideThumb = `COALESCE((SELECT i.sha256 FROM steps t JOIN images i ON i.id = t.image_id LEFT JOIN steps p ON p.id = t.parent_id
	WHERE t.guide_id = g.id AND t.deleted = 0 AND t.hidden = 0 AND COALESCE(p.hidden, 0) = 0
	ORDER BY t.sort, t.id LIMIT 1), '')`

const guideSelect = `SELECT g.id, COALESCE(g.folder_id, 0), g.title, g.updated_at, COUNT(st.id), ` + guideThumb + `
	FROM guides g LEFT JOIN steps st ON st.guide_id = g.id AND st.deleted = 0 `

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func (s *Store) CreateGuide(title string) (int64, error) {
	t := now()
	res, err := s.db.Exec(`INSERT INTO guides(title, created_at, updated_at) VALUES(?, ?, ?)`, title, t, t)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetGuide(id int64) (Guide, error) {
	var g Guide
	err := s.db.QueryRow(guideSelect+`WHERE g.id = ? GROUP BY g.id`, id).Scan(&g.ID, &g.FolderID, &g.Title, &g.UpdatedAt, &g.StepCount, &g.ThumbSHA)
	return g, err
}

// ListGuides는 최근에 바뀐 가이드부터 돌려준다(첫 화면 목록). 없으면 빈 슬라이스다.
func (s *Store) ListGuides() ([]Guide, error) {
	rows, err := s.db.Query(guideSelect + `GROUP BY g.id ORDER BY g.updated_at DESC, g.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Guide{}
	for rows.Next() {
		var g Guide
		if err := rows.Scan(&g.ID, &g.FolderID, &g.Title, &g.UpdatedAt, &g.StepCount, &g.ThumbSHA); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// AppendStep은 가이드 끝에 스텝을 붙이고 가이드의 updated_at을 갱신한다.
func (s *Store) AppendStep(guideID int64, n NewStep) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // Commit 뒤에는 아무 일도 하지 않는다
	var last sql.NullFloat64
	if err := tx.QueryRow(`SELECT MAX(sort) FROM steps WHERE guide_id = ?`, guideID).Scan(&last); err != nil {
		return 0, err
	}
	var img any // 0이면 NULL
	if n.ImageID != 0 {
		img = n.ImageID
	}
	t := now()
	res, err := tx.Exec(`INSERT INTO steps(guide_id, sort, kind, title, image_id, capture_json, annotations_json, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		guideID, SortBetween(last.Float64, 0, false), n.Kind, n.Title, img,
		cmp.Or(n.CaptureJSON, "{}"), cmp.Or(n.AnnotationsJSON, "[]"), t, t)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE guides SET updated_at = ? WHERE id = ?`, t, guideID); err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// SortBetween은 두 스텝 사이에 넣을 sort 값이다. 실수 중간값이라 다른 행을 고치지 않고 순서를 바꿀 수 있다(스펙 5.2절).
// hasNext=false면 prev 뒤 끝에 붙이는 값(다음 정수)이다.
// ponytail: 같은 자리에 50번쯤 연달아 끼우면 float64 정밀도가 바닥난다. 그때 그 가이드의 sort를 1,2,3…으로 다시 매긴다
func SortBetween(prev, next float64, hasNext bool) float64 {
	if !hasNext {
		return math.Floor(prev) + 1
	}
	return (prev + next) / 2
}

// PutImage는 PNG를 images\<sha256 앞 2자리>\<sha256>.png에 저장하고 images 행 ID와 해시를 돌려준다.
// 같은 내용이면 파일도 행도 하나만 둔다.
func (s *Store) PutImage(png []byte, width, height int) (int64, string, error) {
	sum := sha256.Sum256(png)
	sha := hex.EncodeToString(sum[:])
	rel := filepath.Join("images", sha[:2], sha+".png")
	abs := filepath.Join(s.dir, rel)
	switch _, err := os.Stat(abs); {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return 0, "", err
		}
		// 임시 파일에 다 쓴 뒤 이름을 바꿔, 쓰다가 끊겨도 반쪽 PNG가 해시 이름으로 남지 않게 한다.
		if err := os.WriteFile(abs+".tmp", png, 0o644); err != nil {
			return 0, "", err
		}
		if err := os.Rename(abs+".tmp", abs); err != nil {
			return 0, "", err
		}
	case err != nil:
		return 0, "", err
	}
	if _, err := s.db.Exec(`INSERT INTO images(sha256, width, height, path, created_at) VALUES(?, ?, ?, ?, ?) ON CONFLICT(sha256) DO NOTHING`,
		sha, width, height, filepath.ToSlash(rel), now()); err != nil {
		return 0, "", err
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM images WHERE sha256 = ?`, sha).Scan(&id)
	return id, sha, err
}

// ImageFile은 해시로 이미지 파일 경로를 돌려준다. 소문자 16진수 64자가 아니면 false다(경로 조작 방지).
func (s *Store) ImageFile(sha string) (string, bool) {
	if len(sha) != 64 || strings.Trim(sha, "0123456789abcdef") != "" {
		return "", false
	}
	return filepath.Join(s.dir, "images", sha[:2], sha+".png"), true
}
