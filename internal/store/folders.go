// 첫 화면의 폴더·가이드 관리: 폴더 만들기·이름 바꾸기·지우기, 가이드 이름 바꾸기·폴더 옮기기·지우기.
// 폴더는 한 단계만 쓴다(folders.parent_id는 비워 둔다). 첫 화면 목록이 한 줄 폴더 막대라 그보다 깊으면 찾기 어렵다.
package store

import "errors"

// ErrNotFound는 고치거나 지울 행이 없을 때다(그사이 다른 곳에서 지웠거나 ID가 틀림).
var ErrNotFound = errors.New("store: 찾을 수 없음")

type Folder struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// exec1은 행 하나를 고치는 문장을 실행하고, 고친 행이 없으면 ErrNotFound를 돌려준다.
func (s *Store) exec1(query string, args ...any) error {
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListFolders는 만든 순서대로 폴더를 돌려준다. 없으면 빈 슬라이스다.
func (s *Store) ListFolders() ([]Folder, error) {
	rows, err := s.db.Query(`SELECT id, title FROM folders ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Folder{}
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.Title); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// CreateFolder는 폴더를 맨 뒤에 만든다.
func (s *Store) CreateFolder(title string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO folders(title, sort) VALUES(?, COALESCE((SELECT MAX(sort) FROM folders), 0) + 1)`, title)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) RenameFolder(id int64, title string) error {
	return s.exec1(`UPDATE folders SET title = ? WHERE id = ?`, title, id)
}

// DeleteFolder는 폴더만 지운다. 안의 가이드는 지우지 않고 폴더 없음으로 둔다(guides.folder_id ON DELETE SET NULL).
func (s *Store) DeleteFolder(id int64) error {
	return s.exec1(`DELETE FROM folders WHERE id = ?`, id)
}

func (s *Store) RenameGuide(id int64, title string) error {
	return s.exec1(`UPDATE guides SET title = ?, updated_at = ? WHERE id = ?`, title, now(), id)
}

// MoveGuide는 가이드를 폴더로 옮긴다. folderID가 0이면 폴더에서 뺀다. 없는 폴더면 외래 키 오류다.
func (s *Store) MoveGuide(id, folderID int64) error {
	var f any // 0이면 NULL
	if folderID != 0 {
		f = folderID
	}
	return s.exec1(`UPDATE guides SET folder_id = ? WHERE id = ?`, f, id)
}

// DeleteGuide는 가이드와 그 스텝을 지운다(steps.guide_id ON DELETE CASCADE).
// 이미지 파일은 다른 가이드가 같은 해시를 쓸 수 있어 여기서 지우지 않는다. 어느 스텝도 쓰지 않는 이미지는 PruneImages가 지운다(M3-C3).
func (s *Store) DeleteGuide(id int64) error {
	return s.exec1(`DELETE FROM guides WHERE id = ?`, id)
}
