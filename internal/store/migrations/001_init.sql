-- HowtoNote 첫 스키마(스펙 5.2절). 좌표·크기는 모두 원본 이미지의 물리 픽셀이다.
CREATE TABLE folders (
    id        INTEGER PRIMARY KEY,
    parent_id INTEGER REFERENCES folders(id) ON DELETE CASCADE,
    title     TEXT NOT NULL,
    sort      REAL NOT NULL DEFAULT 0
);

CREATE TABLE guides (
    id            INTEGER PRIMARY KEY,
    folder_id     INTEGER REFERENCES folders(id) ON DELETE SET NULL,
    title         TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',   -- Tiptap JSON
    settings_json TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL,              -- RFC 3339 UTC
    updated_at    TEXT NOT NULL
);

CREATE TABLE images (
    id         INTEGER PRIMARY KEY,
    sha256     TEXT NOT NULL UNIQUE,
    width      INTEGER NOT NULL,
    height     INTEGER NOT NULL,
    path       TEXT NOT NULL,                 -- 데이터 폴더 기준 상대 경로(images/ab/ab….png)
    created_at TEXT NOT NULL
);

CREATE TABLE steps (
    id               INTEGER PRIMARY KEY,
    guide_id         INTEGER NOT NULL REFERENCES guides(id) ON DELETE CASCADE,
    parent_id        INTEGER REFERENCES steps(id) ON DELETE CASCADE,
    sort             REAL NOT NULL,            -- 순서를 바꿀 때 앞뒤 값의 중간값을 넣는다
    kind             TEXT NOT NULL CHECK (kind IN ('click', 'type', 'manual', 'image', 'text')),
    title            TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '', -- Tiptap JSON
    image_id         INTEGER REFERENCES images(id),
    hidden           INTEGER NOT NULL DEFAULT 0,
    capture_json     TEXT NOT NULL DEFAULT '{}',
    annotations_json TEXT NOT NULL DEFAULT '[]',
    callouts_json    TEXT NOT NULL DEFAULT '[]',
    view_json        TEXT NOT NULL DEFAULT '{}',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

CREATE INDEX steps_guide_sort ON steps(guide_id, sort);

CREATE TABLE export_presets (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    format       TEXT NOT NULL,
    options_json TEXT NOT NULL DEFAULT '{}'
);
