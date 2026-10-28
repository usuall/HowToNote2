// 녹화기. 전역 훅의 클릭·키·단축키를 받아 스크린샷·UIA·창 정보를 모으고, 제목을 붙여 스텝으로 저장한다.
// 1단계(loop)는 누른 순간의 화면과 요소를 잡고, 2단계(saver)는 OCR·PNG 인코딩·DB 저장을 한다.
// 느린 저장이 다음 클릭의 촬영을 늦추지 않게 둘로 나눴다(스펙 4.1절).
package recorder

import (
	"errors"
	"image"
	"sync"
	"time"

	"howtonote/internal/capture"
	"howtonote/internal/ocr"
	"howtonote/internal/store"
	"howtonote/internal/title"
	"howtonote/internal/uia"
	"howtonote/internal/win"
)

type Mode string

const (
	ModeScreen Mode = "screen" // 클릭한 모니터 전체(기본)
	ModeWindow Mode = "window" // 클릭한 창
	ModeRegion Mode = "region" // 녹화 전에 고른 영역. 영역 밖 클릭은 무시한다
)

const uiaTimeout = 300 * time.Millisecond // 스펙 4.1절. M0 실측 최대 33ms

// toggleSettle은 체크박스 토글(OS 쪽 상태 변경)이 끝났다고 보는 시간이다(컨트롤러 규칙 C1, 스펙 4.3절).
// uiaTimeout과 뜻이 다르다 - uiaTimeout은 "요청 하나에 얼마나 기다릴까", toggleSettle은 "클릭 시각부터
// 몇 ms 지나야 실제 상태 변경이 끝났다고 볼까"다. 값이 같은 300ms인 것은 우연이라 상수를 분리해 둔다.
const toggleSettle = 300 * time.Millisecond

// 사용자에게 보이는 경고. 무엇을 하면 되는지까지 말한다(스펙 1.2절 7).
const (
	msgGrab    = "화면을 찍지 못했습니다. 녹화를 끝내고 다시 시작해 보세요."
	msgSave    = "스텝을 저장하지 못했습니다. 디스크에 빈 공간이 있는지 확인해 보세요."
	msgMonitor = "모니터 정보를 가져오지 못해 이 스텝은 건너뛰었습니다. 녹화를 끝내고 다시 시작해 보세요."
)

type Options struct {
	Mode          Mode
	Region        image.Rectangle // ModeRegion일 때 녹화할 화면 영역(물리 픽셀)
	GuideID       int64           // 0이면 첫 스텝을 저장할 때 새 가이드를 만든다
	IgnoreHWND    uintptr         // 이 창(앱 자신의 툴바) 위 클릭과 키 입력은 무시한다
	AllowInjected bool            // SendInput 입력도 받는다. 통합 테스트에서만 true
	Style         title.Style
	OCR           *ocr.Engine // nil이면 OCR 대체를 건너뛴다
	// OnEvent는 녹화 상태 알림이다. 여러 고루틴(loop·saver)에서 불린다. 막히면 안 된다: loop가 이 함수를 부르는 동안에는
	// 클릭·키를 처리하지 못하고, Pause·Resume은 loop가 받을 때까지 기다리므로 여기서 Pause·Resume을 부르면 서로 막힌다.
	OnEvent func(Event)
}

// Event는 프론트(툴바)로 보내는 녹화 상태다.
type Event struct {
	Type      string `json:"type"` // step, paused, resumed, warning, stopped
	GuideID   int64  `json:"guideId"`
	StepID    int64  `json:"stepId,omitempty"`
	Title     string `json:"title,omitempty"`
	ImageSHA  string `json:"imageSha,omitempty"`
	Steps     int    `json:"steps"`
	Message   string `json:"message,omitempty"`
	LatencyMs int64  `json:"latencyMs,omitempty"` // 누른 순간부터 저장까지(목표 500ms, 스펙 4.1절)
	GrabMs    int64  `json:"grabMs,omitempty"`    // 누른 순간부터 화면 찍기(GrabRect) 완료까지(M1-carry-6)
	Dropped   int64  `json:"dropped,omitempty"`   // 훅 버퍼가 꽉 차 버린 클릭 수(stopped에만 실린다, M1-carry-3)
}

type Recorder struct {
	opt      Options
	st       *store.Store
	ua       *uia.Service
	hook     *capture.Hook
	shots    chan shot
	pause    chan bool // Pause·Resume 요청을 loop로 넘긴다. 세션 상태는 loop 고루틴만 만진다
	quit     chan struct{}
	loopDone chan struct{} // loop가 끝나면 닫힌다
	done     chan struct{} // saver까지 끝나면 닫힌다
	once     sync.Once

	guideID int64 // saver 고루틴만 쓴다. Stop은 done 뒤에 읽는다
	steps   int

	dblTime  time.Duration // 시스템 더블클릭 시간. Start에서 한 번 읽고 loop만 쓴다
	dblSize  image.Point   // 시스템 더블클릭 사각형(SM_CXDOUBLECLK, SM_CYDOUBLECLK)
	prevCtrl int32         // loop만 쓴다: 앞 클릭이 찍은 요소 종류. 체크박스면 다음 클릭과 합치지 않는다(Ruling M2-C2: 체크박스 토글은 늘 따로 한 스텝이다)
	lastID   int64         // saver만 쓴다: 마지막으로 저장한 클릭 스텝. 더블클릭이면 이 스텝의 제목을 바꾼다
	lastIn   title.Input
	lastSHA  string
}

// shot은 1단계에서 모은 재료다. 2단계가 이것으로 스텝을 만든다.
type shot struct {
	kind    string // click, type, manual, double(앞 클릭과 합칠 두 번째 클릭. 다른 칸은 비어 있다)
	at      time.Time
	img     *image.RGBA
	area    image.Rectangle // img가 찍은 화면 영역
	point   *image.Point    // 클릭 표시를 둘 화면 좌표. nil이면 표시 없음
	scale   float64
	monitor image.Rectangle
	win     capture.Window
	el      uia.Element // 누르는 순간 읽은 요소. 체크박스의 "누른 뒤" 상태는 saver가 따로 다시 읽는다(C1)
	in      title.Input
	grabMs  int64 // 클릭부터 GrabRect 완료까지(ms). M1-carry-6
	right   bool  // 오른쪽 버튼 클릭(C1: 오른쪽 클릭은 체크박스를 토글하지 않으니 기다리지 않는다)
}

// Start는 전역 훅(마우스·키보드)과 단축키를 걸고 녹화를 시작한다.
func Start(st *store.Store, ua *uia.Service, opt Options) (*Recorder, error) {
	if opt.Mode == ModeRegion && opt.Region.Empty() {
		return nil, errors.New("recorder: 영역 모드인데 영역이 비어 있다")
	}
	h, err := capture.StartHook(capture.HookOptions{AllowInjected: opt.AllowInjected, Keyboard: true, Hotkeys: hotkeys})
	if err != nil {
		return nil, err
	}
	r := &Recorder{opt: opt, st: st, ua: ua, hook: h, guideID: opt.GuideID,
		shots: make(chan shot, 32), pause: make(chan bool), quit: make(chan struct{}),
		loopDone: make(chan struct{}), done: make(chan struct{})}
	// 실패해도 반환값이 0이라 안전하다: dblTime=0은 "늘 시간 초과"(절대 합치지 않음), dblSize=(0,0)은
	// 정확히 같은 픽셀만 사각형 안으로 본다 - 더블클릭을 못 알아채는 쪽으로만 틀릴 뿐, 잘못 합치지는 않는다.
	// ponytail: GetSystemMetrics는 시스템 DPI 값이다. 배율이 다른 모니터에서는 사각형이 1~2px 다를 수 있다. 문제 되면 GetSystemMetricsForDpi로 바꾼다
	dt, _, _ := win.ProcGetDoubleClickTime.Call()
	cx, _, _ := win.ProcGetSystemMetrics.Call(win.SM_CXDOUBLECLK)
	cy, _, _ := win.ProcGetSystemMetrics.Call(win.SM_CYDOUBLECLK)
	r.dblTime, r.dblSize = time.Duration(dt)*time.Millisecond, image.Pt(int(cx), int(cy))
	go r.saver()
	go r.loop()
	return r, nil
}

// Stop은 녹화를 끝내고 남은 스텝을 모두 저장한 뒤 가이드 ID를 돌려준다(스텝이 없으면 0). 여러 번 불러도 된다.
func (r *Recorder) Stop() int64 {
	r.once.Do(func() { close(r.quit) })
	<-r.done
	return r.guideID
}

// Pause는 녹화를 잠시 멈춘다. 훅은 그대로 두고 이벤트만 버린다(스펙 11절 M1). 녹화가 끝났으면 아무 일도 하지 않는다.
func (r *Recorder) Pause() { r.setPause(true) }

func (r *Recorder) Resume() { r.setPause(false) }

func (r *Recorder) setPause(p bool) {
	select {
	case r.pause <- p:
	case <-r.loopDone: // 이미 끝난 녹화
	}
}

func (r *Recorder) emit(e Event) {
	if r.opt.OnEvent != nil {
		r.opt.OnEvent(e)
	}
}

func (r *Recorder) warn(msg string) { r.emit(Event{Type: "warning", Message: msg}) }

// loop는 1단계다. 클릭·키·단축키·일시정지 요청을 한 고루틴에서 차례로 처리해 세션 상태에 잠금이 필요 없다.
// 끝날 때 열린 세션을 저장하고, 훅을 떼고, shots를 닫아 saver도 끝나게 한다.
func (r *Recorder) loop() {
	var (
		sess   *session
		paused bool
		timer  = time.NewTimer(time.Hour)
		prev   capture.Click // 앞 클릭(더블클릭 판정용). 클릭이 아닌 일이 끼면 비운다
	)
	timer.Stop()
	defer func() {
		r.endSession(&sess, nil)
		r.hook.Stop()
		close(r.loopDone)
		close(r.shots)
	}()
	for _, id := range r.hook.FailedHotkeys() {
		// 툴바에는 일시정지·끝내기 버튼만 있고 화면 저장(F10) 버튼은 없어, 셋 모두에 맞는 방법(그 프로그램 끄기)을 말한다
		r.warn("단축키 " + hotkeyNames[id] + title.Josa(hotkeyNames[id], "을/를") + " 다른 프로그램이 쓰고 있습니다. 그 프로그램을 끄고 녹화를 다시 시작해 보세요.")
	}
	setPaused := func(p bool) {
		if p == paused {
			return
		}
		paused = p
		if p {
			r.endSession(&sess, nil)
			r.emit(Event{Type: "paused"})
		} else {
			r.emit(Event{Type: "resumed"})
		}
	}
	for {
		select {
		case <-r.quit:
			return
		case p := <-r.pause:
			prev = capture.Click{}
			setPaused(p)
		case c, ok := <-r.hook.Events():
			if !ok {
				return
			}
			if !paused {
				prev = r.click(prev, c, &sess)
			}
		case k, ok := <-r.hook.Keys():
			if !ok {
				return
			}
			prev = capture.Click{} // 사이에 키를 쳤으면 더블클릭이 아니다
			if paused {
				continue
			}
			if k.End {
				r.endSession(&sess, nil)
				continue
			}
			if sess == nil {
				sess = r.openSession()
			}
			if sess != nil {
				timer.Reset(refreshDelay)
			}
		case <-timer.C:
			r.refresh(&sess)
		case id, ok := <-r.hook.Hotkeys():
			if !ok {
				return
			}
			prev = capture.Click{} // 사이에 키를 쳤으면 더블클릭이 아니다
			switch id {
			case hotkeyPause:
				setPaused(!paused)
			case hotkeyManual: // 일시정지 중에도 찍는다: 사용자가 일부러 누른 단축키다(일시정지는 자동 스텝만 멈춘다)
				r.endSession(&sess, nil)
				r.manual()
			case hotkeyStop:
				return
			}
		}
	}
}

// click은 더블클릭의 두 번째 클릭이면(위치·시간이 시스템 더블클릭 안이고, 앞 클릭이 체크박스가 아니면) 앞 스텝에
// 합치게 하고, 아니면 onClick으로 새 스텝 재료를 만든다. 다음 판정에 쓸 앞 클릭을 돌려준다(합쳤거나 무시한 클릭이면
// 빈 값 - 그 뒤 클릭은 새로 짝을 찾는다). 합치기를 녹화기에서 하는 것은 클릭 시각(ms)과 시스템 더블클릭 설정을
// 여기서만 알기 때문이다(저장된 스텝의 시각은 초 단위라 ms 비교를 못 한다, D4).
// 체크박스 다음은 합치지 않는다(Ruling M2-C2): 체크박스는 누를 때마다 선택/해제가 바뀌어야 해서, 두 번 누르면
// 늘 선택→해제(또는 해제→선택) 두 스텝이어야 한다. TestRecorder_CheckBoxTitleReflectsStateAfterClick이 이를 본다.
func (r *Recorder) click(prev, c capture.Click, sess **session) capture.Click {
	if !prev.Right && !c.Right && r.prevCtrl != title.CheckBox &&
		isDouble(prev.At, c.At, image.Pt(int(prev.X), int(prev.Y)), image.Pt(int(c.X), int(c.Y)), r.dblTime, r.dblSize) {
		r.shots <- shot{kind: "double", at: c.At}
		return capture.Click{} // 세 번째 클릭은 새 스텝이다
	}
	if r.onClick(c, sess) {
		return c
	}
	return capture.Click{} // 무시한 클릭(우리 창, IME 후보창, 영역 밖)과는 합치지 않는다
}

// onClick은 누른 순간의 화면·요소·창을 잡아 saver에 바로 넘긴다. 체크박스의 "누른 뒤" 상태 재확인처럼
// 느린 일은 전혀 하지 않는다 - 그걸 여기서 기다리면 다음 클릭의 촬영이 늦어진다(스펙 4.1절, 컨트롤러 규칙 C1
// 고침: 원래 여기 있던 300ms 대기를 saver로 옮겼다). 우리 창, IME 후보창, 영역 밖 클릭은 무시한다(false).
// 다른 곳 클릭은 열린 입력 세션을 끝낸다(스펙 4.2절). 이때 클릭 순간 찍은 화면 하나를 입력 스텝에도 쓰고,
// 입력 스텝을 먼저 보낸다(C3). IME 후보 고르기와 우리 창(툴바) 클릭은 입력을 끝내는 게 아니라 세션을 그대로 둔다.
// 스텝을 보내면(true) click이 다음 더블클릭 판정에 쓸 요소 종류(prevCtrl)도 같이 남긴다.
func (r *Recorder) onClick(c capture.Click, sess **session) bool {
	w := capture.WindowAt(c.X, c.Y)
	if (r.opt.IgnoreHWND != 0 && w.HWND == r.opt.IgnoreHWND) || w.IsIME() {
		return false
	}
	p := image.Pt(int(c.X), int(c.Y))
	s, ok, warned := r.snap(p, w)             // UIA보다 먼저 찍는다: 화면이 바뀌기 전 상태
	grabMs := time.Since(c.At).Milliseconds() // M1-carry-6: 클릭부터 화면 찍기 완료까지
	switch {
	case warned:
		*sess = nil // 같은 순간이라 입력 스텝도 찍을 수 없다. 경고는 snap이 이미 한 번 했다
		return false
	case !ok:
		r.endSession(sess, nil) // 영역 밖 클릭: 스텝은 아니지만 포커스는 옮겨 갔다
		return false
	}
	el, _ := r.ua.ElementAt(c.X, c.Y, uiaTimeout) // 입력 값 읽기보다 먼저: 클릭한 요소는 늦게 읽을수록 화면이 바뀌어 있다. 실패하면 OCR·창 제목으로 대체한다
	s.at = c.At
	r.endSession(sess, &s)
	s.kind, s.point, s.el, s.grabMs, s.right = "click", &p, el, grabMs, c.Right
	s.in = title.Input{Action: title.Click, ControlType: el.ControlType, Name: el.Name, Right: c.Right, WindowTitle: w.Title}
	r.prevCtrl = el.ControlType
	r.shots <- s
	return true
}

// snap은 p 지점 기준으로 모드에 맞는 영역을 찍어 shot의 화면 부분(img·area·monitor·scale·win)을 채운다.
// 클릭·입력 끝·수동 캡처가 같이 쓴다(C9). 영역 모드에서 p가 영역 밖이면 조용히 ok=false, 모니터·촬영 실패면 경고하고
// warned=true다(호출 쪽이 같은 순간 다시 찍어 경고를 두 번 내지 않게).
func (r *Recorder) snap(p image.Point, w capture.Window) (sh shot, ok, warned bool) {
	area, monitor, scale, ok, monErr := r.area(p, w)
	if monErr {
		r.warn(msgMonitor)
		return shot{}, false, true
	}
	if !ok {
		return shot{}, false, false
	}
	img, err := capture.GrabRect(area.Min.X, area.Min.Y, area.Dx(), area.Dy())
	if err != nil {
		r.warn(msgGrab)
		return shot{}, false, true
	}
	return shot{img: img, area: area, monitor: monitor, scale: scale, win: w}, true, false
}

// area는 모드에 맞는 촬영 영역과 그 지점의 모니터·배율을 정한다.
// monitorErr=true면 MonitorAt 자체가 실패한 것이다(드문 시스템 오류) - 호출 쪽이 경고를 띄운다.
// ok=false인데 monitorErr=false면 그냥 이 모드에서 찍을 자리가 아니라는 뜻이다(예: 영역 밖 클릭 - 조용히 무시한다).
func (r *Recorder) area(p image.Point, w capture.Window) (area, monitor image.Rectangle, scale float64, ok, monitorErr bool) {
	var err error
	monitor, scale, err = capture.MonitorAt(int32(p.X), int32(p.Y))
	if err != nil {
		return area, monitor, scale, false, true
	}
	switch r.opt.Mode {
	case ModeRegion:
		return r.opt.Region, monitor, scale, p.In(r.opt.Region), false
	case ModeWindow:
		// 창 경계가 클릭 지점을 포함하지 못하면(DWM 확장 프레임 경계가 드물게 어긋난다) 모니터 전체로
		// 돌아간다 - 안 그러면 클릭 표시가 이미지 밖 좌표로 저장된다.
		if a := w.Bounds.Intersect(monitor); !a.Empty() && p.In(a) {
			return a, monitor, scale, true, false
		}
	}
	return monitor, monitor, scale, true, false
}
