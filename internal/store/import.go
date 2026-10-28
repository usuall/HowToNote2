// .htn 가져오기(스펙 5.3절): 새 가이드와 스텝을 한 트랜잭션으로 넣는다. 늘 새 ID를 받아 같은 파일을 두 번 가져와도
// 덮어쓰지 않고 사본이 생긴다(M4a 결정 H5). 그림은 부르는 쪽이 PutImage로 먼저 넣는다(안 쓰이게 되면 PruneImages가 지운다).
package store

import (
	"cmp"
	"fmt"
)

// ImportStep은 가져올 스텝 하나다. Parent는 steps 안 상위 스텝의 번호이고 -1이면 최상위다(앞 번호만).
// 하위 스텝이 한 단계인지는 .htn 읽기(project.Read)가 이미 검사했다.
type ImportStep struct {
	Parent          int
	Kind            string
	Title           string
	Description     string // Tiptap JSON 객체. 비었으면 빈 문자열
	ImageID         int64  // 0이면 그림 없음
	Hidden          bool
	CaptureJSON     string
	AnnotationsJSON string
	CalloutsJSON    string
	ViewJSON        string
}

// ImportGuide는 folderID 폴더(0이거나 없는 폴더면 폴더 없음)에 가이드를 새로 만들고 스텝을 순서대로 넣는다.
// sort는 1, 2, 3…이다. 하나라도 틀리면 아무것도 넣지 않는다.
func (s *Store) ImportGuide(folderID int64, title string, steps []ImportStep) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // Commit 뒤에는 아무 일도 하지 않는다
	t := now()
	res, err := tx.Exec(`INSERT INTO guides(folder_id, title, created_at, updated_at)
		VALUES((SELECT id FROM folders WHERE id = ?), ?, ?, ?)`, folderID, title, t, t)
	if err != nil {
		return 0, err
	}
	gid, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	ids := make([]int64, len(steps))
	for i, st := range steps {
		if st.Parent < -1 || st.Parent >= i {
			return 0, fmt.Errorf("가져올 스텝 %d: 상위 스텝 번호 %d", i, st.Parent)
		}
		if err := checkStep(Step{ID: -1, AnnotationsJSON: st.AnnotationsJSON, CalloutsJSON: st.CalloutsJSON,
			ViewJSON: st.ViewJSON, Description: st.Description}); err != nil {
			return 0, fmt.Errorf("가져올 스텝 %d: %w", i, err)
		}
		var parent any // -1이면 NULL
		if st.Parent >= 0 {
			parent = ids[st.Parent]
		}
		res, err := tx.Exec(`INSERT INTO steps(guide_id, parent_id, sort, kind, title, description, image_id, hidden,
			capture_json, annotations_json, callouts_json, view_json, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			gid, parent, float64(i+1), st.Kind, st.Title, st.Description, nullID(st.ImageID), st.Hidden,
			cmp.Or(st.CaptureJSON, "{}"), st.AnnotationsJSON, st.CalloutsJSON, st.ViewJSON, t, t)
		if err != nil {
			return 0, err
		}
		if ids[i], err = res.LastInsertId(); err != nil {
			return 0, err
		}
	}
	return gid, tx.Commit()
}
