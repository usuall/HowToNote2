// 편집기와 첫 화면 관리 바인딩(M2): 폴더·가이드 관리, 스텝 자동 저장, 글·그림 스텝 넣기, 가이드 닫기, 녹화 이어서 하기, 창 닫기 전 저장.
// 사용자에게 돌려주는 오류는 무엇을 하면 되는지까지 말하는 한국어 문장이다(스펙 1.2절 7).
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	_ "image/gif" // 붙여넣거나 고른 그림의 형식. PNG가 아니면 PNG로 바꿔 저장한다
	_ "image/jpeg"
	"image/png"
	"strings"
	"unicode/utf8"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"howtonote/internal/recorder"
	"howtonote/internal/store"
)

var (
	errName  = errors.New("이름을 1자 이상 100자 이하로 입력하세요.")
	errSave  = errors.New("저장하지 못했습니다. 디스크에 빈 공간이 있는지 확인하고 다시 해 보세요.")
	errGone  = errors.New("이미 지워진 항목입니다. 처음 화면에서 목록을 다시 확인하세요.")
	errImage = errors.New("그림을 읽지 못했습니다. PNG나 JPG 그림을 넣어 보세요.")
	errBig   = errors.New("그림이 너무 큽니다. 그림 크기를 줄이거나 필요한 부분만 잘라서 넣어 보세요.")
)

// maxSide는 넣을 수 있는 그림의 한 변 최대 길이(px)다. 편집기와 합성 렌더러의 캔버스 한계(16384)보다 조금 작게 둔다.
// maxPixels는 전체 화소 수 한계다. 한 변만 보면 16000x16000(1GB RGBA)도 들어와 디코딩·캔버스가 메모리를 다 쓴다.
const (
	maxSide   = 16000
	maxPixels = 40_000_000
)

// saved는 저장소 오류를 사용자 문장으로 바꾼다.
func saved(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return errGone
	}
	// 외래 키 위반은 가리키는 행(폴더, 상위 스텝, 가이드)이 그사이 지워졌다는 뜻이다. 디스크 공간 문제가 아니다
	var se *sqlite.Error
	if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
		return errGone
	}
	return errSave
}

// cleanName은 폴더·가이드 이름의 공백을 정리하고 길이를 본다.
func cleanName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || utf8.RuneCountInString(s) > 100 {
		return "", errName
	}
	return s, nil
}

func (a *App) ListFolders() ([]store.Folder, error) {
	if err := a.waitReady(); err != nil {
		return nil, err
	}
	fs, err := a.st.ListFolders()
	if err != nil {
		return nil, errLoad
	}
	return fs, nil
}

func (a *App) CreateFolder(title string) (int64, error) {
	if err := a.waitReady(); err != nil {
		return 0, err
	}
	t, err := cleanName(title)
	if err != nil {
		return 0, err
	}
	id, err := a.st.CreateFolder(t)
	return id, saved(err)
}

func (a *App) RenameFolder(id int64, title string) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	t, err := cleanName(title)
	if err != nil {
		return err
	}
	return saved(a.st.RenameFolder(id, t))
}

// DeleteFolder는 폴더만 지운다. 안의 가이드는 '모든 가이드'에 남는다.
func (a *App) DeleteFolder(id int64) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	return saved(a.st.DeleteFolder(id))
}

func (a *App) RenameGuide(id int64, title string) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	t, err := cleanName(title)
	if err != nil {
		return err
	}
	return saved(a.st.RenameGuide(id, t))
}

// MoveGuide는 가이드를 폴더로 옮긴다. folderID가 0이면 폴더에서 뺀다.
func (a *App) MoveGuide(id, folderID int64) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	return saved(a.st.MoveGuide(id, folderID))
}

// DeleteGuide는 가이드와 스텝을 지운다. 프론트가 먼저 한 번 더 묻는다.
func (a *App) DeleteGuide(id int64) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	if err := a.st.DeleteGuide(id); err != nil {
		return saved(err)
	}
	a.prune() // 지운 매뉴얼만 쓰던 그림과 합성 그림(재정 M3-C3)
	return nil
}

// SaveSteps는 편집기의 자동 저장이다. 바뀐 행을 통째로 받아 한 번에 쓴다.
func (a *App) SaveSteps(guideID int64, steps []store.Step) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	return saved(a.st.UpdateSteps(guideID, steps))
}

// AddTextStep은 그림 없는 글 스텝을 sort 자리에 넣는다. parentID가 0이 아니면 그 스텝의 하위 스텝이다.
func (a *App) AddTextStep(guideID int64, sort float64, parentID int64) (store.Step, error) {
	if err := a.waitReady(); err != nil {
		return store.Step{}, err
	}
	st, err := a.st.InsertStep(guideID, parentID, sort, store.NewStep{Kind: "text"})
	return st, saved(err)
}

// AddImageStep은 붙여넣거나 고른 그림(base64)으로 그림 스텝을 넣는다.
func (a *App) AddImageStep(guideID int64, sort float64, parentID int64, data string) (store.Step, error) {
	if err := a.waitReady(); err != nil {
		return store.Step{}, err
	}
	b, w, h, err := pngOf(data)
	if err != nil {
		return store.Step{}, err
	}
	// PutImage와 InsertStep 사이에 그림 정리(prune)가 끼면 새 그림을 고아로 보고 지운다. 정리도 a.mu를 잡는다
	a.mu.Lock()
	defer a.mu.Unlock()
	imgID, _, err := a.st.PutImage(b, w, h)
	if err != nil {
		return store.Step{}, errSave
	}
	st, err := a.st.InsertStep(guideID, parentID, sort, store.NewStep{Kind: "image", ImageID: imgID})
	return st, saved(err)
}

// pngOf는 base64 그림을 PNG 바이트와 크기로 바꾼다. 이미지 저장소는 PNG만 두므로 JPG·GIF는 PNG로 다시 만든다.
func pngOf(data string) ([]byte, int, int, error) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, 0, 0, errImage
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, errImage
	}
	if cfg.Width > maxSide || cfg.Height > maxSide || cfg.Width*cfg.Height > maxPixels {
		return nil, 0, 0, errBig
	}
	if format == "png" {
		return raw, cfg.Width, cfg.Height, nil
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, errImage
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, errImage
	}
	return buf.Bytes(), cfg.Width, cfg.Height, nil
}

// CloseGuide는 편집기를 닫을 때 부른다. 지운 스텝을 영구히 지운다(스펙 6.3절).
func (a *App) CloseGuide(id int64) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	if id == 0 { // 0은 "모든 가이드"라서 편집기에서는 받지 않는다
		return nil
	}
	if err := a.st.PurgeDeleted(id); err != nil {
		return saved(err)
	}
	a.prune() // 영구히 지운 스텝만 쓰던 그림과 합성 그림(재정 M3-C3)
	return nil
}

// ContinueRecording은 열린 가이드 끝에 스텝을 더하는 녹화를 시작한다(전체 화면 모드, 스펙 1.2절 2 기본값).
func (a *App) ContinueRecording(guideID int64) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	if _, err := a.st.GetGuide(guideID); err != nil {
		return errGone
	}
	return a.startWith(recorder.Options{Mode: recorder.ModeScreen, GuideID: guideID})
}

// beforeClose는 창을 닫기 전에 한 번만 막고 프론트에 'app:closing'을 보낸다. 프론트는 남은 편집을 저장하고
// 지운 스텝을 정리한 뒤 Quit을 부른다(Wails Quit도 이 함수를 지나는데, 그때는 이미 물었으니 막지 않는다).
// 두 번째부터는 막지 않아, 프론트가 멈춰도 한 번 더 닫으면 닫힌다. 저장이 실패하면 프론트가 CancelClose로 되돌린다.
//
// Wails는 OnBeforeClose를 UI 스레드에서 부른다. emit은 a.mu를 잠그고, begin/endRecording은 창을 다루는 동안
// (enterToolbar/restoreWindow가 UI 스레드 처리를 기다리는 동안) a.mu를 쥔 채로 있을 수 있다. 그래서 여기서
// a.mu를 잠그면(또는 잠그는 함수를 동기로 부르면) UI 스레드가 자기 자신을 기다려 멈춘다. emit은 고루틴으로 보낸다.
func (a *App) beforeClose(context.Context) bool {
	if a.closeAsked.Swap(true) {
		return false
	}
	go a.emit("app:closing", nil)
	return true
}

// CancelClose는 창을 닫으며 한 저장이 실패했을 때 프론트가 부른다. 다음 닫기를 다시 한 번 막아, 저장하지 못한
// 편집이 다음 X에 말없이 버려지지 않게 한다. 프론트가 아예 답하지 못하면 부르지 않으니 두 번째 닫기는 그대로 닫힌다.
func (a *App) CancelClose() {
	a.closeAsked.Store(false)
}
