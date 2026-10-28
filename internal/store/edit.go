// 편집기 저장: 스텝 목록 읽기, 고친 행 쓰기, 새 스텝 넣기, 제목 바꾸기, 지운 스텝 영구 삭제.
// 편집기는 바뀐 행을 통째로 보내고 여기서는 형식만 검사한다. 순서·하위 스텝 규칙은 프론트 src/editor/model.ts에 있다.
package store

import (
	"cmp"
	"encoding/json"
	"fmt"
	"strings"
)

const stepSelect = `SELECT st.id, st.guide_id, COALESCE(st.parent_id, 0), st.sort, st.kind, st.title, st.description,
	COALESCE(i.sha256, ''), st.hidden, st.deleted, st.capture_json, st.annotations_json, st.callouts_json, st.view_json
	FROM steps st LEFT JOIN images i ON i.id = st.image_id `

func scanStep(r interface{ Scan(...any) error }) (Step, error) {
	var st Step
	err := r.Scan(&st.ID, &st.GuideID, &st.ParentID, &st.Sort, &st.Kind, &st.Title, &st.Description,
		&st.ImageSHA, &st.Hidden, &st.Deleted, &st.CaptureJSON, &st.AnnotationsJSON, &st.CalloutsJSON, &st.ViewJSON)
	return st, err
}

// ListSteps는 지우지 않은 스텝을 순서대로 돌려준다. 없으면 빈 슬라이스다.
func (s *Store) ListSteps(guideID int64) ([]Step, error) { return s.listSteps(guideID, false) }

// ListDeletedSteps는 편집기에서 지웠지만 가이드를 닫기 전이라 아직 되살릴 수 있는 스텝이다.
func (s *Store) ListDeletedSteps(guideID int64) ([]Step, error) { return s.listSteps(guideID, true) }

func (s *Store) listSteps(guideID int64, deleted bool) ([]Step, error) {
	rows, err := s.db.Query(stepSelect+`WHERE st.guide_id = ? AND st.deleted = ? ORDER BY st.sort, st.id`, guideID, deleted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Step{}
	for rows.Next() {
		st, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// jsonOf는 s가 open('[' 또는 '{')으로 시작하는 올바른 JSON인지 본다.
func jsonOf(s string, open byte) bool {
	s = strings.TrimSpace(s)
	return s != "" && s[0] == open && json.Valid([]byte(s))
}

// checkStep은 저장 전 형식 검사다. 깨진 JSON이 DB에 들어가 편집기나 내보내기(M3)가 멈추는 일을 막는다.
func checkStep(st Step) error {
	switch {
	case !jsonOf(st.AnnotationsJSON, '['):
		return fmt.Errorf("스텝 %d: annotations_json이 JSON 배열이 아니다", st.ID)
	case !jsonOf(st.CalloutsJSON, '['):
		return fmt.Errorf("스텝 %d: callouts_json이 JSON 배열이 아니다", st.ID)
	case !jsonOf(st.ViewJSON, '{'):
		return fmt.Errorf("스텝 %d: view_json이 JSON 객체가 아니다", st.ID)
	case st.Description != "" && !jsonOf(st.Description, '{'):
		return fmt.Errorf("스텝 %d: description이 JSON 객체가 아니다", st.ID)
	case st.ParentID == st.ID:
		return fmt.Errorf("스텝 %d: 자기 자신을 상위 스텝으로 둘 수 없다", st.ID)
	}
	return nil
}

// nullID는 0을 NULL로 바꾼다(parent_id, image_id).
func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// UpdateSteps는 편집기가 고친 행을 한 트랜잭션으로 쓴다. 하나라도 틀리면 아무것도 쓰지 않는다.
// 쓰는 칸은 parent_id, sort, title, description, hidden, deleted, annotations_json, callouts_json, view_json이다.
// kind, 이미지, capture_json은 녹화 결과라 그대로 둔다. 상위 스텝은 같은 가이드의 스텝이어야 한다.
func (s *Store) UpdateSteps(guideID int64, steps []Step) error {
	for _, st := range steps {
		if err := checkStep(st); err != nil {
			return err
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // Commit 뒤에는 아무 일도 하지 않는다
	t := now()
	for _, st := range steps {
		res, err := tx.Exec(`UPDATE steps SET parent_id = ?1, sort = ?2, title = ?3, description = ?4, hidden = ?5, deleted = ?6,
			annotations_json = ?7, callouts_json = ?8, view_json = ?9, updated_at = ?10
			WHERE id = ?11 AND guide_id = ?12 AND (?1 IS NULL OR ?1 IN (SELECT id FROM steps WHERE guide_id = ?12))`,
			nullID(st.ParentID), st.Sort, st.Title, st.Description, st.Hidden, st.Deleted,
			st.AnnotationsJSON, st.CalloutsJSON, st.ViewJSON, t, st.ID, guideID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return fmt.Errorf("스텝 %d(상위 %d)이 가이드 %d에 없다: %w", st.ID, st.ParentID, guideID, ErrNotFound)
		}
	}
	if _, err := tx.Exec(`UPDATE guides SET updated_at = ? WHERE id = ?`, t, guideID); err != nil {
		return err
	}
	return tx.Commit()
}

// InsertStep은 편집기에서 만든 스텝(글 스텝, 그림 스텝)을 sort 자리에 넣고 저장된 행을 돌려준다.
// 녹화기는 끝에 붙이는 AppendStep을 쓴다.
func (s *Store) InsertStep(guideID, parentID int64, sort float64, n NewStep) (Step, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Step{}, err
	}
	defer tx.Rollback()
	t := now()
	res, err := tx.Exec(`INSERT INTO steps(guide_id, parent_id, sort, kind, title, image_id, capture_json, annotations_json, created_at, updated_at)
		SELECT ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?9
		WHERE ?2 IS NULL OR ?2 IN (SELECT id FROM steps WHERE guide_id = ?1)`,
		guideID, nullID(parentID), sort, n.Kind, n.Title, nullID(n.ImageID), cmp.Or(n.CaptureJSON, "{}"), cmp.Or(n.AnnotationsJSON, "[]"), t)
	if err != nil {
		return Step{}, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return Step{}, err
	} else if n == 0 {
		return Step{}, fmt.Errorf("상위 스텝 %d이 가이드 %d에 없다: %w", parentID, guideID, ErrNotFound)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Step{}, err
	}
	if _, err := tx.Exec(`UPDATE guides SET updated_at = ? WHERE id = ?`, t, guideID); err != nil {
		return Step{}, err
	}
	if err := tx.Commit(); err != nil {
		return Step{}, err
	}
	return scanStep(s.db.QueryRow(stepSelect+`WHERE st.id = ?`, id))
}

// SetStepTitle은 제목만 바꾼다. 녹화기가 더블클릭을 앞 스텝에 합칠 때 쓴다.
func (s *Store) SetStepTitle(id int64, title string) error {
	return s.exec1(`UPDATE steps SET title = ?, updated_at = ? WHERE id = ?`, title, now(), id)
}

// PurgeDeleted는 삭제 표시한 스텝을 영구히 지운다(가이드를 닫을 때). guideID가 0이면 모든 가이드다(앱을 열 때).
// 지운 스텝을 상위로 둔 남은 스텝은 먼저 최상위로 올린다. parent_id의 ON DELETE CASCADE가 그 스텝까지 지우지 않게 한다.
func (s *Store) PurgeDeleted(guideID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE steps SET parent_id = NULL WHERE deleted = 0 AND parent_id IN
		(SELECT id FROM steps WHERE deleted = 1 AND (?1 = 0 OR guide_id = ?1))`, guideID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM steps WHERE deleted = 1 AND (?1 = 0 OR guide_id = ?1)`, guideID); err != nil {
		return err
	}
	return tx.Commit()
}
