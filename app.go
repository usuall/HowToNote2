// Wails 바인딩. 첫 화면 목록, 녹화(시작·영역·일시정지·끝내기), 녹화 뒤 가이드 보기를 프론트에 내준다.
// 사용자에게 돌려주는 오류는 무엇을 하면 되는지까지 말하는 한국어 문장이다(스펙 1.2절 7).
package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"howtonote/internal/capture"
	"howtonote/internal/export"
	"howtonote/internal/ocr"
	"howtonote/internal/recorder"
	"howtonote/internal/store"
	"howtonote/internal/title"
	"howtonote/internal/uia"
	"howtonote/internal/win"
)

var (
	errSettings  = errors.New("설정 파일(settings.json)을 읽지 못했습니다. 파일 내용을 고치거나 지운 뒤 다시 실행해 보세요.")
	errDataDir   = errors.New("데이터 폴더를 열지 못했습니다. 설정 파일의 data_dir 경로를 확인해 보세요.")
	errUIA       = errors.New("화면 요소 읽기 기능을 시작하지 못했습니다. 프로그램을 다시 실행해 보세요.")
	errNoWindow  = errors.New("HowtoNote 창을 찾지 못했습니다. 프로그램을 다시 실행해 보세요.")
	errRecording = errors.New("이미 녹화 중입니다. 툴바의 '녹화 끝내기'를 먼저 누르세요.")
	errHook      = errors.New("녹화를 시작하지 못했습니다. 백신 프로그램이 HowtoNote를 막고 있는지 확인해 보세요.")
	errGrab      = errors.New("화면을 찍지 못했습니다. 잠시 뒤 다시 시도해 보세요.")
	errRegion    = errors.New("고른 영역이 너무 작습니다. 조금 더 크게 끌어서 고르세요.")
	errLoad      = errors.New("매뉴얼을 불러오지 못했습니다. 프로그램을 다시 실행해 보세요.")
)

const minRegion = 20 // 영역 최소 크기(물리 픽셀). 프론트 RegionSelect.svelte와 같다

type App struct {
	// ready는 startup이 끝나면 닫힌다. Wails v2.16은 페이지를 먼저 띄우고 OnStartup을 고루틴으로 돌리며 바인딩도
	// 고루틴으로 부르므로, 첫 화면의 ListGuides가 startup보다 먼저 올 수 있다(R21). 아래 다섯 필드는 startup만 쓰고
	// 그 뒤로는 바뀌지 않으니, 읽는 쪽은 모두 waitReady를 거친다.
	ready   chan struct{}
	st      *store.Store
	ua      *uia.Service
	ocr     *ocr.Engine
	style   title.Style
	initErr error

	mu         sync.Mutex // 아래 상태를 지킨다. 프론트가 버튼을 연달아 눌러도 경합이 없게 한다
	ctx        context.Context
	hwnd       uintptr
	rec        *recorder.Recorder // "stopped"를 처리할 때까지 남아 있어, 끝나는 중에 새 녹화가 시작되지 않는다
	saved      *win.Placement     // 녹화 전 창 상태. 읽지 못했으면 nil이고 되돌리지 않는다
	frozen     []byte             // 영역 고르기용 멈춘 화면(PNG)
	frozenAt   image.Rectangle    // 멈춘 화면의 화면 좌표
	lastExport string             // 마지막으로 내보낸 파일 경로. RevealFile은 이 경로만 연다
	// chosenExport는 ChooseExportFile이 막 돌려준 경로다. ExportGuide는 이 경로만 받는다(검수 Minor 7):
	// 프론트가 고르지 않은 경로로 밀어 쓰는 것을 막는다. 한 번 쓰면 비운다(1회용 표) — 내보내기 창은 매번
	// ChooseExportFile을 다시 불러 새 값을 받으므로, 쌓아 두고 재사용할 일이 없다.
	chosenExport string

	closeAsked atomic.Bool // 창 닫기를 한 번 막고 프론트에 저장을 맡겼는지(beforeClose)
}

// NewApp은 빈 앱을 만든다. 설정·저장소·UIA 준비는 startup에서 한다: `wails generate module`과
// `wails build`가 바인딩을 만들려고 main()/NewApp()을 실행하는데, 그때 개발 PC에
// %APPDATA%\HowtoNote\data.db를 만들거나 UIA를 시작하면 안 되기 때문이다.
func NewApp() *App { return &App{ready: make(chan struct{})} }

// startup은 OnStartup으로 불린다. 바인딩과 동시에 돌 수 있어 끝날 때 ready를 닫는다(NewApp으로 만든 App에서만 부른다).
// 설정, 저장소, UIA를 여기서 준비한다. 실패해도 창은 띄우고, 첫 화면이 오류 문장을 보여 준다.
func (a *App) startup(ctx context.Context) {
	defer close(a.ready)
	// os.UserConfigDir는 APPDATA가 비어 있으면 오류다. Getenv로 읽으면 빈 값이 상대 경로(작업 폴더)가 된다
	cfg, err := os.UserConfigDir()
	var set store.Settings
	if err == nil {
		cfg = filepath.Join(cfg, "HowtoNote")
		set, err = store.LoadSettings(cfg)
	}
	if err != nil {
		a.initErr = errSettings
	} else if a.st, err = store.Open(cmp.Or(set.DataDir, cfg)); err != nil {
		a.initErr = errDataDir
	} else if a.ua, err = uia.Start(); err != nil {
		a.initErr = errUIA
	} else {
		a.ocr = ocr.Find(set.TesseractPath) // 없으면 nil: OCR 대체만 건너뛴다
		a.style = title.Style(set.TitleStyle)
	}
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
	a.prune() // 앞 실행이 남긴 고아 그림·캐시. Open이 지운 표시 스텝을 정리한 뒤라 그 그림도 여기서 지운다
}

// waitReady는 startup이 끝날 때까지 기다린 뒤 준비 오류를 돌려준다. ready가 nil인 App(테스트가 &App{...}로
// 필드를 직접 채운 것)은 이미 준비된 것으로 본다.
func (a *App) waitReady() error {
	if a.ready != nil {
		<-a.ready
	}
	return a.initErr
}

// shutdown은 녹화 중이면 남은 스텝을 저장하고 끝낸다. 창이 닫히는 중이라 이벤트는 보내지 않는다(ctx를 먼저 비운다).
// Wails는 OnStartup 고루틴을 띄운 뒤에야 창 루프를 돌리고 루프가 끝나야 OnShutdown을 부르므로 startup은 보통 이미
// 끝났다. 그래도 startup이 끝나지 않으면(UIA 시작이 멈춤 등) 창 닫기가 영영 멈추지 않게 잠깐만 기다리고 정리 없이
// 끝낸다: 바인딩이 모두 ready를 기다리니 녹화기는 없고, 열린 저장소는 프로세스가 끝나며 닫힌다(SQLite가 다음에 되살린다).
func (a *App) shutdown(context.Context) {
	if a.ready != nil {
		select {
		case <-a.ready:
		case <-time.After(5 * time.Second):
			return
		}
	}
	a.mu.Lock()
	a.ctx = nil
	r := a.rec
	a.mu.Unlock()
	if r != nil {
		r.Stop()
	}
	if a.ua != nil {
		a.ua.Close()
	}
	if a.st != nil {
		a.st.Close()
	}
}

func (a *App) emit(name string, data any) {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, name, data)
	}
}

// window는 우리 Wails 창의 HWND다. 처음 찾은 값을 둔다.
func (a *App) window() uintptr {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.hwnd == 0 {
		a.hwnd = findOwnWindow()
	}
	return a.hwnd
}

// ListGuides는 첫 화면의 최근 가이드 목록이다.
func (a *App) ListGuides() ([]store.Guide, error) {
	if err := a.waitReady(); err != nil {
		return nil, err
	}
	gs, err := a.st.ListGuides()
	if err != nil {
		return nil, errLoad
	}
	return gs, nil
}

type GuideDetail struct {
	Guide   store.Guide  `json:"guide"`
	Steps   []store.Step `json:"steps"`
	Deleted []store.Step `json:"deleted"` // 지웠지만 가이드를 닫기 전이라 되살릴 수 있는 스텝(편집기 '삭제한 스텝')
}

// OpenGuide는 편집기가 여는 가이드 내용이다.
func (a *App) OpenGuide(id int64) (GuideDetail, error) {
	if err := a.waitReady(); err != nil {
		return GuideDetail{}, err
	}
	g, err := a.st.GetGuide(id)
	if err != nil {
		return GuideDetail{}, errLoad
	}
	steps, err := a.st.ListSteps(id)
	if err != nil {
		return GuideDetail{}, errLoad
	}
	deleted, err := a.st.ListDeletedSteps(id)
	if err != nil {
		return GuideDetail{}, errLoad
	}
	return GuideDetail{Guide: g, Steps: steps, Deleted: deleted}, nil
}

// StartRecording은 전체 화면("screen", 기본)이나 활성 창("window") 모드로 녹화를 시작하고 창을 툴바로 바꾼다.
func (a *App) StartRecording(mode string) error {
	m := recorder.ModeScreen
	if mode == string(recorder.ModeWindow) {
		m = recorder.ModeWindow
	}
	return a.startWith(recorder.Options{Mode: m})
}

// startWith는 캡처 제외 → 툴바 모드 → 녹화기 시작 순서로 한다. 첫 클릭부터 툴바가 찍히지 않게 하려는 순서다.
// 녹화를 시작하는 바인딩(전체 화면·창, 영역)은 모두 여기를 지나므로 준비 확인도 여기서 한다.
func (a *App) startWith(opt recorder.Options) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	hwnd := a.window()
	if hwnd == 0 {
		return errNoWindow
	}
	opt.IgnoreHWND, opt.Style, opt.OCR, opt.OnEvent = hwnd, a.style, a.ocr, a.onEvent
	return a.begin(func() (*recorder.Recorder, error) {
		_ = capture.ExcludeFromCapture(hwnd) // 실패(Windows 10 2004 미만)해도 녹화는 한다. 툴바가 찍힐 수 있다
		saved := enterToolbar(hwnd)
		r, err := recorder.Start(a.st, a.ua, opt)
		if err != nil {
			restoreWindow(hwnd, saved)
			_ = capture.IncludeInCapture(hwnd)
			return nil, errHook
		}
		a.saved = saved // begin이 잠근 채로 부른다
		return r, nil
	})
}

// begin은 녹화기가 없을 때만 start를 불러 담는다. 잠근 채로 부르므로 시작 버튼을 두 번 눌러도 녹화기는 하나다.
// start 안에서 a.mu를 다시 잠그는 함수(emit, window, current)를 부르면 안 된다.
func (a *App) begin(start func() (*recorder.Recorder, error)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rec != nil {
		return errRecording
	}
	r, err := start()
	if err != nil {
		return err
	}
	a.rec = r
	return nil
}

// current는 지금 녹화기다(없으면 nil).
func (a *App) current() *recorder.Recorder {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rec
}

// endRecording은 "stopped" 때 녹화 상태를 비우고 창을 되돌린다. 비웠으면 true다.
// 되돌리기를 잠근 채로 해야 한다: begin의 enterToolbar도 잠근 채로 돌기 때문에(위 주석), 그렇지 않으면
// 멈추고 되돌리기 전에 새 녹화가 시작돼 enterToolbar가 툴바 모양을 "녹화 전 상태"로 잘못 저장할 수 있다.
func (a *App) endRecording() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rec == nil {
		return false
	}
	a.rec = nil
	if a.hwnd != 0 { // 테스트가 만드는 &App{}에는 창이 없다
		restoreWindow(a.hwnd, a.saved)
		_ = capture.IncludeInCapture(a.hwnd)
	}
	return true
}

// onEvent는 녹화기 알림을 프론트로 넘긴다. 끝났으면(툴바 버튼이든 단축키든) 창을 먼저 되돌린다.
func (a *App) onEvent(e recorder.Event) {
	if e.Type == "stopped" {
		a.endRecording()
	}
	a.emit("recorder", e)
}

func (a *App) PauseRecording() {
	if r := a.current(); r != nil {
		r.Pause()
	}
}

func (a *App) ResumeRecording() {
	if r := a.current(); r != nil {
		r.Resume()
	}
}

// StopRecording은 녹화를 끝낸다. 남은 스텝을 다 저장할 때까지 기다린다.
// 창 되돌리기와 화면 전환은 "stopped" 이벤트에서 한다(단축키로 끝낼 때와 같은 길). 잠금을 쥐지 않고 기다린다.
func (a *App) StopRecording() {
	if r := a.current(); r != nil {
		r.Stop()
	}
}

type Frozen struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// BeginRegionSelect는 창이 있는 모니터를 멈춰 찍고 창을 그 모니터 전체 화면으로 바꾼다.
// 프론트는 /frozen.png 위에서 영역을 끌어 고른다. 우리 창은 캡처에서 빼고 찍어 멈춘 화면에 나오지 않는다.
func (a *App) BeginRegionSelect() (Frozen, error) {
	if err := a.waitReady(); err != nil {
		return Frozen{}, err
	}
	hwnd := a.window()
	a.mu.Lock()
	ctx, busy := a.ctx, a.rec != nil
	a.mu.Unlock()
	if hwnd == 0 || ctx == nil {
		return Frozen{}, errNoWindow
	}
	if busy {
		return Frozen{}, errRecording
	}
	_ = capture.ExcludeFromCapture(hwnd)
	time.Sleep(100 * time.Millisecond) // DWM은 다음 합성 프레임부터 창을 뺀다
	hmon, _, _ := win.ProcMonitorFromWindow.Call(hwnd, win.MONITOR_DEFAULTTONEAREST)
	full, _, ok := win.MonitorRects(hmon)
	var buf bytes.Buffer
	var img *image.RGBA
	err := errGrab
	if ok {
		if img, err = capture.GrabRect(full.Min.X, full.Min.Y, full.Dx(), full.Dy()); err == nil {
			err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img)
		}
	}
	if err != nil {
		_ = capture.IncludeInCapture(hwnd)
		return Frozen{}, errGrab
	}
	a.mu.Lock()
	a.frozen, a.frozenAt = buf.Bytes(), full
	a.mu.Unlock()
	runtime.WindowFullscreen(ctx)
	runtime.WindowSetAlwaysOnTop(ctx, true)
	return Frozen{Width: full.Dx(), Height: full.Dy()}, nil
}

// StartRegionRecording은 멈춘 화면 좌표(x, y, w, h, 물리 픽셀)의 영역으로 녹화를 시작한다.
// 성공하든 실패하든 영역 고르기 화면은 끝낸다(실패하면 프론트가 첫 화면에서 안내한다).
func (a *App) StartRegionRecording(x, y, w, h int) error {
	a.mu.Lock()
	at, ctx, hwnd := a.frozenAt, a.ctx, a.hwnd
	a.frozen = nil
	a.mu.Unlock()
	if ctx != nil {
		runtime.WindowSetAlwaysOnTop(ctx, false)
		runtime.WindowUnfullscreen(ctx)
	}
	err := errRegion
	if w >= minRegion && h >= minRegion {
		region := image.Rect(x, y, x+w, y+h).Add(at.Min).Intersect(at)
		err = a.startWith(recorder.Options{Mode: recorder.ModeRegion, Region: region})
	}
	if err != nil && !errors.Is(err, errRecording) {
		_ = capture.IncludeInCapture(hwnd)
	}
	return err
}

// CancelRegionSelect는 영역 고르기를 그만두고 창을 원래대로 되돌린다.
func (a *App) CancelRegionSelect() {
	a.mu.Lock()
	ctx, hwnd := a.ctx, a.hwnd
	a.frozen = nil
	a.mu.Unlock()
	if ctx == nil {
		return
	}
	runtime.WindowSetAlwaysOnTop(ctx, false)
	runtime.WindowUnfullscreen(ctx)
	_ = capture.IncludeInCapture(hwnd)
}

// serveAsset은 프론트가 <img>로 쓰는 파일을 내준다: /img/<sha256>.png(스텝 이미지), /frozen.png(영역 고르기 화면).
// /fonts/는 앱 화면 글꼴(Pretendard)을 내준다. 정적 자산(frontend/dist)에 없는 GET만 여기로 온다(Wails AssetServer.Handler).
func (a *App) serveAsset(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	// 글꼴은 실행 파일에 묶여 있어 App의 어떤 필드도 쓰지 않는다. startup(저장소·UIA 준비)을 기다리지 않고
	// 바로 내줘야 첫 화면 글자가 깨진 모습으로 잠깐 보이지 않는다
	if p == "/fonts/Pretendard-Regular.ttf" || p == "/fonts/Pretendard-Bold.ttf" {
		w.Header().Set("Content-Type", "font/ttf")
		w.Header().Set("Cache-Control", "max-age=31536000, immutable") // 실행 파일에 묶인 글꼴이라 바뀌지 않는다
		_, _ = w.Write(export.Font(p == "/fonts/Pretendard-Bold.ttf"))
		return
	}
	_ = a.waitReady() // 저장소를 열다 실패했으면 a.st가 nil이라 아래에서 404가 된다
	switch {
	case p == "/frozen.png":
		a.mu.Lock()
		b := a.frozen
		a.mu.Unlock()
		if b == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	case strings.HasPrefix(p, "/img/") && strings.HasSuffix(p, ".png") && a.st != nil:
		file, ok := a.st.ImageFile(strings.TrimSuffix(strings.TrimPrefix(p, "/img/"), ".png"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "max-age=31536000, immutable") // 내용 해시 주소라 바뀌지 않는다
		http.ServeFile(w, r, file)
	default:
		http.NotFound(w, r)
	}
}
