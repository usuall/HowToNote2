// 앱 설정(settings.json). 위치는 항상 %APPDATA%\HowtoNote\settings.json이고, data_dir로 데이터 폴더만 옮길 수 있다(스펙 5.1절).
// 설정 파일이 데이터 폴더 안에 있으면 데이터 폴더를 옮길 때 설정을 찾을 수 없어서 위치를 고정했다.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type Settings struct {
	DataDir       string `json:"data_dir,omitempty"`       // 비우면 설정 폴더(%APPDATA%\HowtoNote)
	TesseractPath string `json:"tesseract_path,omitempty"` // 비우면 PATH에서 찾는다
	TitleStyle    string `json:"title_style,omitempty"`    // "합니다"(기본) 또는 "하세요"
}

// LoadSettings는 dir\settings.json을 읽는다. 파일이 없으면 기본값이다.
func LoadSettings(dir string) (Settings, error) {
	var s Settings
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, fmt.Errorf("settings.json 해석: %w", err)
	}
	return s, nil
}
