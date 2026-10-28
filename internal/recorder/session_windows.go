// 입력 세션(스펙 4.2절), 전역 단축키, 수동 캡처. 키보드 훅은 세션의 시작과 끝만 알려 주고, 값은 끝날 때 UIA로 읽는다.
// 키 값은 메모리에도 디스크에도 남기지 않는다. 남는 것은 입력란의 최종 값(비밀번호는 ●●●●)뿐이다.
package recorder

import (
	"image"
	"time"
	"unsafe"

	"howtonote/internal/capture"
	"howtonote/internal/title"
	"howtonote/internal/uia"
	"howtonote/internal/win"
)

const refreshDelay = 250 * time.Millisecond // 마지막 키 뒤 이만큼 쉬면 값을 미리 읽어 둔다

// 전역 단축키(스펙 4.1절). 툴바 안내 문구(frontend/src/Toolbar.svelte)와 같이 바꾼다.
const (
	hotkeyPause  = 1 // 일시정지/다시 시작
	hotkeyManual = 2 // 지금 화면 캡처
	hotkeyStop   = 3 // 녹화 끝내기
)

var hotkeys = []capture.Hotkey{
	{ID: hotkeyPause, Mods: capture.ModControl | capture.ModShift, VK: 0x78},  // F9
	{ID: hotkeyManual, Mods: capture.ModControl | capture.ModShift, VK: 0x79}, // F10
	{ID: hotkeyStop, Mods: capture.ModControl | capture.ModShift, VK: 0x7A},   // F11
}

var hotkeyNames = map[int]string{hotkeyPause: "Ctrl+Shift+F9", hotkeyManual: "Ctrl+Shift+F10", hotkeyStop: "Ctrl+Shift+F11"}

// session은 열린 입력 세션이다. 요소 핸들을 들고 있지 않고 속성 값으로 같은 입력란인지 비교한다.
type session struct {
	el    uia.Element    // 세션을 연 입력란
	win   capture.Window // 세션을 연 창. 입력 스텝의 창 정보와 (클릭으로 끝나지 않을 때) 촬영 기준이다
	last  string         // 미리 읽어 둔 값
	known bool           // last를 한 번이라도 읽었는지
}

// editable은 글자를 쳐서 값이 바뀌는 요소인지 본다. 읽기 전용 입력란·문서 보기에서 친 키(Space로 스크롤 등)는 입력이 아니다(I2).
func editable(el uia.Element) bool {
	return (el.ControlType == title.Edit || el.ControlType == title.Document) && !el.ReadOnly
}

func same(a, b uia.Element) bool {
	return a.ProcessID == b.ProcessID && a.ControlType == b.ControlType && a.AutomationID == b.AutomationID &&
		a.Name == b.Name && a.Rect == b.Rect
}

func center(r image.Rectangle) image.Point { return r.Min.Add(r.Max).Div(2) }

// openSession은 포커스가 입력란이면 세션을 연다.
// ponytail: 입력란이 아닌 곳에서 키를 칠 때마다 UIA를 한 번씩 부른다(수 ms). 느리면 같은 창이면 다시 묻지 않게 한다
func (r *Recorder) openSession() *session {
	el, err := r.ua.Focused(uiaTimeout)
	if err != nil {
		return nil
	}
	return r.sessionOn(el)
}

// sessionOn은 el이 쓸 수 있는 입력란이면 그 입력란의 세션을 만든다. 우리 창에서 친 키는 무시한다.
func (r *Recorder) sessionOn(el uia.Element) *session {
	w := capture.ForegroundWindow()
	if !editable(el) || (r.opt.IgnoreHWND != 0 && w.HWND == r.opt.IgnoreHWND) {
		return nil
	}
	return &session{el: el, win: w}
}

// refresh는 마지막 키 뒤 refreshDelay가 지나면 값을 미리 읽어 둔다.
// 포커스가 클릭·Tab·Enter 없이 다른 입력란으로 옮겨 갔으면(자동 넘김, Ctrl+L 등) 앞 세션을 닫고 새 입력란으로
// 세션을 연다(I3). 안 그러면 새 입력란에 친 글자가 앞 세션에 묻혀 사라진다. Alt+Tab은 여기 오지 않는다: 훅이 Tab을
// 보조 키보다 먼저 보고 세션을 끝낸다(capture.classify).
// ponytail: 마지막 키 뒤 250ms 안에 키 없이 포커스가 다른 입력란으로 옮겨 가면(Ctrl·Win 단축키, 앱이 스스로 옮기는
// 포커스) 그 뒤 아무것도 안 쳐도 그 입력란에 세션이 열려, 끝날 때 원래 있던 값으로 입력 스텝이 생긴다. 잦으면 키마다
// 포커스 창(GetGUIThreadInfo)을 비교해 옮겨 간 뒤 친 키가 있을 때만 연다
func (r *Recorder) refresh(sp **session) {
	s := *sp
	if s == nil {
		return
	}
	el, err := r.ua.Focused(uiaTimeout)
	switch {
	case err != nil:
	case same(el, s.el):
		s.last, s.known = el.Value, true
	case editable(el):
		r.endSession(sp, nil)
		if *sp = r.sessionOn(el); *sp != nil {
			(*sp).last, (*sp).known = el.Value, true
		}
	}
}

// readValue는 세션 입력란의 지금 값을 읽는다. 포커스가 아직 거기 있으면 포커스 요소로, 옮겨 갔으면(클릭·Tab) 입력란 가운데
// 지점의 요소로 읽고, 같은 입력란일 때만 믿는다. 둘 다 안 되면 미리 읽어 둔 값이다: 마지막 쉼 뒤에 친 글자는 빠질 수 있어
// 마지막 수단으로만 쓴다(I1).
func (r *Recorder) readValue(s *session) (string, bool) {
	if el, err := r.ua.Focused(uiaTimeout); err == nil && same(el, s.el) {
		return el.Value, true
	}
	if !s.el.Rect.Empty() {
		c := center(s.el.Rect)
		if el, err := r.ua.ElementAt(int32(c.X), int32(c.Y), uiaTimeout); err == nil && same(el, s.el) {
			return el.Value, true
		}
	}
	return s.last, s.known
}

// endSession은 열린 세션을 닫고 입력 스텝 재료를 saver에 넘긴다.
// clicked가 nil이 아니면 세션을 끝낸 클릭의 화면이다(C3): 새로 찍지 않고 그 화면을 쓴다. 값은 그 뒤에 읽어도 되지만
// 화면은 클릭 순간 것이어야 한다.
// ponytail: 세션은 Enter·Tab·클릭·일시정지·수동 캡처·끝내기와 미리 읽기 때 본 포커스 이동으로만 닫힌다
func (r *Recorder) endSession(sp **session, clicked *shot) {
	s := *sp
	if s == nil {
		return
	}
	*sp = nil
	value, known := r.readValue(s)
	switch {
	case s.el.IsPassword:
		value = "●●●●"
	case !known:
		value = "" // 읽지 못함: "텍스트를 입력합니다"
	}
	var mark *image.Point
	if !s.el.Rect.Empty() {
		c := center(s.el.Rect)
		mark = &c
	}
	var sh shot
	if clicked != nil {
		sh = *clicked
	} else {
		ref := center(s.win.Bounds)
		if mark != nil {
			ref = *mark
		}
		var ok bool
		if sh, ok, _ = r.snap(ref, s.win); !ok {
			return
		}
		sh.at = time.Now()
	}
	sh.win = s.win // 지금 앞 창(클릭한 창, 일시정지를 누른 툴바)이 아니라 입력한 창을 적는다
	if mark != nil && !mark.In(sh.area) {
		mark = nil // 클릭한 화면에 입력란이 없다(다른 창·모니터를 클릭): 표시를 이미지 밖에 두지 않는다
	}
	sh.kind, sh.point, sh.el = "type", mark, s.el
	sh.in = title.Input{Action: title.Type, ControlType: s.el.ControlType, Name: s.el.Name, Value: value, WindowTitle: s.win.Title}
	r.shots <- sh
}

// manual은 수동 캡처(단축키)다. 표시 지점 없이 모드에 맞는 영역을 찍는다.
// 창 모드는 커서가 아니라 앞 창 가운데를 기준으로 한다: area는 기준점이 창 밖이면 모니터 전체로 돌아가기 때문이다.
// 앞 창이 우리 툴바면 창 정보를 버리고 커서가 있는 모니터를 찍는다.
func (r *Recorder) manual() {
	w := capture.ForegroundWindow()
	if r.opt.IgnoreHWND != 0 && w.HWND == r.opt.IgnoreHWND {
		w = capture.Window{}
	}
	p := center(w.Bounds) // 커서를 못 읽으면 앞 창 가운데(없으면 (0,0)의 모니터)
	cur := new(win.Point)
	if ok, _, _ := win.ProcGetCursorPos.Call(uintptr(unsafe.Pointer(cur))); ok != 0 {
		p = image.Pt(int(cur.X), int(cur.Y))
	}
	switch {
	case r.opt.Mode == ModeRegion:
		p = center(r.opt.Region)
	case r.opt.Mode == ModeWindow && !w.Bounds.Empty():
		p = center(w.Bounds)
	}
	sh, ok, _ := r.snap(p, w)
	if !ok {
		return
	}
	sh.kind, sh.at = "manual", time.Now()
	sh.in = title.Input{Action: title.Manual, WindowTitle: w.Title}
	r.shots <- sh
}
