// SQLite 데이터 저장소. 데이터 폴더에 data.db와 images\를 두고, 스키마는 migrations/*.sql을 PRAGMA user_version으로 올린다(스펙 5.1~5.2절).
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	_ "modernc.org/sqlite" // cgo 없는 SQLite 드라이버("sqlite")
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db  *sql.DB
	dir string
}

// Open은 dir에 data.db와 images 폴더를 만들거나 열고 스키마를 최신으로 올린다.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "images"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(renderedDir)), 0o755); err != nil {
		return nil, err
	}
	// 경로 뒤 ?_pragma=는 드라이버가 떼어 내 연결마다 실행한다. 외래 키는 SQLite 기본값이 꺼짐이라 켠다.
	db, err := sql.Open("sqlite", filepath.Join(dir, "data.db")+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // ponytail: 연결 하나로 쓰기 경합을 없앤다. 읽기가 느려지면 읽기 전용 풀을 따로 둔다
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, dir: dir}
	// 앞 실행이 가이드를 닫지 못하고 끝났으면(강제 종료 등) 남은 삭제 표시 스텝을 지운다(스펙 6.3절: 닫을 때 영구 삭제).
	if err := s.PurgeDeleted(0); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// migrate는 user_version보다 번호가 큰 마이그레이션을 차례로 적용한다.
// 파일은 001_*.sql부터 빈틈없이 번호를 붙이고, 이미 배포한 파일은 고치지 않고 새 파일을 더한다.
func migrate(db *sql.DB) error {
	var ver int
	if err := db.QueryRow("PRAGMA user_version").Scan(&ver); err != nil {
		return err
	}
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	slices.Sort(names)
	if ver > len(names) {
		return fmt.Errorf("데이터베이스 버전 %d가 이 앱이 아는 버전 %d보다 새롭다", ver, len(names))
	}
	for i := ver; i < len(names); i++ {
		body, err := migrations.ReadFile(names[i])
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", names[i], err)
		}
		// PRAGMA는 매개변수를 받지 않아 정수를 직접 넣는다.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
