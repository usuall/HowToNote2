// 내보내기 바인딩(M3): 합성 그림 준비 목록과 올리기(EnsureRendered, 스펙 7.1절), 문서 모양(스펙 7.3절), 저장할 파일 고르기,
// 문서 만들기, '폴더 열기', 안 쓰는 그림 정리(재정 M3-C3).
// 문서 모양 옵션은 JSON 문자열로 주고받는다. Go 패키지 이름 export는 TypeScript 예약어라 Wails 모델 네임스페이스로 쓸 수 없다.
// 렌더러·저장소 오류에는 로컬 경로(fs.PathError)가 들어 있어, 사용자에게는 여기서 고정 문장으로만 바꿔 준다(재정 R31).
// err.Error()를 돌려주거나 로그에 남기지 않는다.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/png" // 올라온 합성 그림이 PNG인지 본다
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"howtonote/internal/export"
	"howtonote/internal/printer"
	"howtonote/internal/store"
)

var (
	errFormat        = errors.New("모르는 파일 형식입니다. 내보내기 창에서 형식을 다시 고르세요.")
	errOptions       = errors.New("문서 모양 값을 읽지 못했습니다. '고급 설정'을 다시 골라 보세요.")
	errRenderData    = errors.New("그림을 준비하지 못했습니다. 내보내기를 다시 눌러 보세요.")
	errNotRendered   = errors.New("그림이 아직 준비되지 않았습니다. 내보내기를 다시 눌러 보세요.")
	errExportWrite   = errors.New("이 폴더에는 저장할 수 없습니다. 같은 이름의 파일이 열려 있으면 닫고, 아니면 바탕화면에 저장해 보세요.")
	errPDFRuntime    = errors.New("PDF를 만들 WebView2 런타임을 찾지 못했습니다. HowtoNote를 다시 설치해 보세요.")
	errPDFTimeout    = errors.New("PDF를 만드는 데 너무 오래 걸립니다. 잠시 뒤 다시 해 보거나, 매뉴얼을 나눠 내보내 보세요.")
	errExport        = errors.New("문서를 만들지 못했습니다. 프로그램을 다시 실행한 뒤 해 보세요.")
	errPresetDefault = errors.New("기본 문서 모양은 지우거나 바꿀 수 없습니다. 다른 이름으로 저장해 쓰세요.")
	errReveal        = errors.New("폴더를 열지 못했습니다. 저장한 폴더를 탐색기에서 직접 열어 보세요.")
)

// formatNames는 저장 창의 파일 형식 이름이다.
var formatNames = map[string]string{"pdf": "PDF 문서", "hwpx": "한글 문서", "docx": "Word 문서", "html": "웹 페이지", "htn": "HowtoNote 파일"}

// RenderJob은 프론트가 합성 그림을 만들 스텝 하나다. Key는 Go가 정한 캐시 이름이고, Cached면 이미 있다.
type RenderJob struct {
	StepID          int64  `json:"stepId"`
	ImageSHA        string `json:"imageSha"`
	AnnotationsJSON string `json:"annotationsJson"`
	ViewJSON        string `json:"viewJson"`
	Key             string `json:"key"`
	Cached          bool   `json:"cached"`
}

// ExportPlan은 내보낼 그림 스텝의 합성 그림 목록이다. 같은 키는 한 번만 넣는다(같은 그림·그리기·자르기).
// 내보낼 스텝은 export.Build와 같은 export.Numbers로 고른다: 번호가 없는 스텝(숨김, 숨긴 최상위의 하위)은 뺀다(재정 M3-C2).
func (a *App) ExportPlan(guideID int64) ([]RenderJob, error) {
	if err := a.waitReady(); err != nil {
		return nil, err
	}
	steps, err := a.st.ListSteps(guideID)
	if err != nil {
		return nil, errLoad
	}
	out := []RenderJob{}
	seen := map[string]bool{}
	for i, num := range export.Numbers(steps) {
		s := steps[i]
		if num == "" || s.ImageSHA == "" {
			continue
		}
		key := export.RenderKey(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		_, cached := a.st.RenderedFile(key)
		out = append(out, RenderJob{StepID: s.ID, ImageSHA: s.ImageSHA, AnnotationsJSON: s.AnnotationsJSON, ViewJSON: s.ViewJSON, Key: key, Cached: cached})
	}
	return out, nil
}

// PutRendered는 프론트가 만든 합성 PNG(머리말 없는 base64)를 캐시에 쓴다. PNG가 아니거나 원본 그림 한계(maxPixels)보다
// 크면 받지 않는다. 합성 그림은 원본을 자른 크기라 그보다 클 수 없다.
func (a *App) PutRendered(key, data string) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return errRenderData
	}
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(raw)); err != nil || format != "png" || cfg.Width*cfg.Height > maxPixels {
		return errRenderData
	}
	// 그림 정리(PruneRendered)는 남은 .tmp를 지운다. 쓰는 중인 .tmp를 지우지 않게 정리와 같은 a.mu를 잡는다
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.st.PutRendered(key, raw); err != nil {
		return errRenderData
	}
	return nil
}

// exportOptions는 문서 모양 JSON을 기본값 위에 읽는다. 빠진 칸은 기본값이다.
func exportOptions(js string) (export.Options, error) {
	o := export.DefaultOptions()
	if strings.TrimSpace(js) == "" {
		return o, nil
	}
	if err := json.Unmarshal([]byte(js), &o); err != nil {
		return o, errOptions
	}
	return o.Normalize(), nil
}

// ListPresets는 문서 모양 목록이다. 옵션은 기본값을 채운 JSON이라 프론트는 기본값을 따로 알 필요가 없다.
func (a *App) ListPresets() ([]store.Preset, error) {
	if err := a.waitReady(); err != nil {
		return nil, err
	}
	ps, err := a.st.ListPresets()
	if err != nil {
		return nil, errLoad
	}
	for i, p := range ps {
		o, _ := exportOptions(p.Options) // 깨진 값은 기본값으로 보인다
		b, _ := json.Marshal(o.Normalize())
		ps[i].Options = string(b)
		if _, ok := export.Formats[p.Format]; !ok {
			ps[i].Format = "pdf"
		}
	}
	return ps, nil
}

// notDefault는 id가 첫 문서 모양('기본', 마이그레이션 003)이면 errPresetDefault다. '기본'은 지우지도 고치지도 않는다
// (스펙 1.2절 2: 기본값만으로 끝까지, 재정 M3-C21).
func (a *App) notDefault(id int64) error {
	ps, err := a.st.ListPresets()
	switch {
	case err != nil:
		return errLoad
	case len(ps) > 0 && ps[0].ID == id:
		return errPresetDefault
	}
	return nil
}

// SavePreset은 문서 모양을 저장한다. ID가 0이면 새로 만든다.
func (a *App) SavePreset(p store.Preset) (int64, error) {
	if err := a.waitReady(); err != nil {
		return 0, err
	}
	name, err := cleanName(p.Name)
	if err != nil {
		return 0, err
	}
	if _, ok := export.Formats[p.Format]; !ok {
		return 0, errFormat
	}
	o, err := exportOptions(p.Options)
	if err != nil {
		return 0, err
	}
	if p.ID != 0 {
		if err := a.notDefault(p.ID); err != nil {
			return 0, err
		}
	}
	b, _ := json.Marshal(o)
	id, err := a.st.SavePreset(store.Preset{ID: p.ID, Name: name, Format: p.Format, Options: string(b)})
	return id, saved(err)
}

// DeletePreset은 문서 모양을 지운다. 첫 항목('기본')은 지우지 않는다.
func (a *App) DeletePreset(id int64) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	if err := a.notDefault(id); err != nil {
		return err
	}
	return saved(a.st.DeletePreset(id))
}

// ChooseExportFile은 저장 창을 띄워 파일 경로를 고르게 한다. 취소하면 빈 문자열이다. 파일 이름은 매뉴얼 제목이다(스펙 7.4절).
func (a *App) ChooseExportFile(guideID int64, format string) (string, error) {
	if err := a.waitReady(); err != nil {
		return "", err
	}
	ext, ok := export.Formats[format]
	if format == "htn" {
		ext, ok = ".htn", true // .htn은 문서 렌더러가 아니라 ExportProject가 쓴다. export.Formats에 넣으면 ExportGuide가 받는다
	}
	if !ok {
		return "", errFormat
	}
	g, err := a.st.GetGuide(guideID)
	if err != nil {
		return "", errLoad
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return "", errNoWindow
	}
	path, err := runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title:           "내보낼 파일 저장",
		DefaultFilename: export.FileName(g.Title, ext),
		Filters:         []runtime.FileFilter{{DisplayName: formatNames[format] + " (*" + ext + ")", Pattern: "*" + ext}},
	})
	if err != nil {
		return "", errExportWrite
	}
	if path != "" && !strings.EqualFold(filepath.Ext(path), ext) {
		path += ext
	}
	a.mu.Lock()
	a.chosenExport = path
	a.mu.Unlock()
	return path, nil
}

// ExportGuide는 매뉴얼을 format 문서로 만들어 path에 쓴다. 합성 그림은 프론트가 먼저 준비한다(ensureRendered).
// 오류는 정해 둔 문장으로만 바꾼다(재정 R31): 모르는 렌더러 오류도 경로가 든 원래 문장 대신 errExport다.
func (a *App) ExportGuide(guideID int64, format, optionsJSON, path string) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	if _, ok := export.Formats[format]; !ok {
		return errFormat
	}
	// ChooseExportFile이 막 돌려준 경로만 받는다(검수 Minor 7): 다른 경로는 저장 창을 거치지 않은 것이라 거절한다.
	// 한 번 확인하면 비운다(App.chosenExport 주석 참고).
	a.mu.Lock()
	chosen := a.chosenExport
	a.chosenExport = ""
	a.mu.Unlock()
	if path == "" || path != chosen || !filepath.IsAbs(path) {
		return errExportWrite
	}
	o, err := exportOptions(optionsJSON)
	if err != nil {
		return err
	}
	now := time.Now()
	d, err := export.Build(a.st, guideID, now)
	switch {
	case errors.Is(err, export.ErrNotRendered):
		return errNotRendered
	case err != nil:
		return errLoad
	}
	var b []byte
	switch format {
	case "pdf":
		b, err = export.PDF(d, o)
	case "hwpx":
		b, err = export.HWPX(d, o, now)
	case "docx":
		b, err = export.DOCX(d, o, now)
	case "html":
		b, err = export.HTML(d, o)
	}
	switch {
	case errors.Is(err, printer.ErrNoRuntime):
		return errPDFRuntime
	case errors.Is(err, printer.ErrTimeout):
		return errPDFTimeout
	case err != nil:
		return errExport
	}
	// ponytail: 고른 경로에 바로 쓴다(D13). 쓰다가 끊기면(디스크 가득, 강제 종료) 반쪽 파일이 남고 같은 이름의 예전 문서도
	// 이미 덮어써 잃는다. 사용자가 다시 저장하면 고쳐진다. 반쪽 파일 보고가 오면 같은 폴더 임시 파일에 쓰고 os.Rename으로 바꾼다
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return errExportWrite
	}
	a.mu.Lock()
	a.lastExport = path
	a.mu.Unlock()
	return nil
}

// RevealFile은 방금 내보낸 파일을 탐색기에서 고른 채로 보여 준다(내보내기 알림의 '폴더 열기', UI 설계 5.5절).
// 프론트가 아무 경로나 열지 못하게, 마지막으로 내보낸 경로만 받는다. 저장소를 쓰지 않아 waitReady가 필요 없다.
func (a *App) RevealFile(path string) error {
	a.mu.Lock()
	last := a.lastExport
	a.mu.Unlock()
	if path == "" || path != last {
		return errReveal
	}
	return reveal(path)
}

// prune은 어떤 스텝도 쓰지 않는 원본 그림과 합성 그림 캐시를 지운다(재정 M3-C3, M2 재정 R28). startup 끝과
// CloseGuide·DeleteGuide가 성공한 뒤에 부른다. 녹화 중에는 건너뛴다: 녹화기는 PutImage 뒤에 AppendStep을 해서 그 사이의
// 새 그림이 고아로 보인다. a.mu를 쥔 채로 해서 정리하는 동안 녹화가 시작되지 않는다(begin도 a.mu를 쥔다).
// PutImage·PutRendered를 부르는 곳은 모두 a.mu를 잡는다(정리가 그 사이에 끼지 않게). 녹화기만 예외라 위 a.rec 검사로 막는다.
// 지운 표시만 된 스텝도 되살릴 수 있어 쓰는 것으로 친다. 실패는 다음 정리에서 다시 하므로 무시한다.
// ponytail: a.mu를 쥔 채로 동기로 돌고 매뉴얼마다 쿼리를 두 번 한다(매뉴얼 수에 비례). 동기라 shutdown보다 먼저 끝나는 순서가
// 지켜진다. 시작이나 편집기 닫기가 눈에 띄게 느려지면 모든 스텝의 키를 JOIN 한 번으로 읽거나, rec 검사를 a.mu 안에 둔 채 고루틴으로 돌린다.
func (a *App) prune() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rec != nil || a.st == nil {
		return
	}
	_, _ = a.st.PruneImages()
	gs, err := a.st.ListGuides()
	if err != nil {
		return
	}
	keep := map[string]bool{}
	for _, g := range gs {
		live, err1 := a.st.ListSteps(g.ID)
		gone, err2 := a.st.ListDeletedSteps(g.ID)
		if err1 != nil || err2 != nil {
			return // 목록이 모자란 채로 지우면 쓰는 캐시까지 지운다
		}
		for _, s := range append(live, gone...) {
			if s.ImageSHA != "" {
				keep[export.RenderKey(s)] = true
			}
		}
	}
	_, _ = a.st.PruneRendered(keep)
}
