// 전역 입력 훅(WH_MOUSE_LL, WH_KEYBOARD_LL)과 전역 단축키(RegisterHotKey). 모두 전용 OS 스레드 하나에 걸고 그 스레드가 메시지 루프를 돈다.
// 콜백은 즉시 반환하고 이벤트는 채널로 넘긴다. 콜백이 늦으면 Windows가 훅을 조용히 떼어 내므로, 콜백 안에서는 아무것도 기다리지 않는다.
package capture

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"howtonote/internal/win"
)

type Click struct {
	X        int32     `json:"x"`
	Y        int32     `json:"y"`
	Right    bool      `json:"right"`
	Injected bool      `json:"injected"`
	At       time.Time `json:"at"`
}

// Key는 키 입력의 종류만 담는다. 어떤 키였는지는 담지 않는다(스펙 4.2절, 9절).
type Key struct {
	End      bool // Enter·Tab: 입력 세션을 끝낸다. false면 입력란 내용을 바꿀 수 있는 키다
	Injected bool
	At       time.Time
}

// Hotkey는 훅 스레드에 거는 전역 단축키다.
type Hotkey struct {
	ID   int
	Mods uint32 // ModControl | ModShift 등
	VK   uint32
}

// RegisterHotKey 보조 키.
const (
	ModAlt     = 0x0001
	ModControl = 0x0002
	ModShift   = 0x0004
)

type HookOptions struct {
	AllowInjected bool     // SendInput 같은 프로그램 입력도 받는다(테스트에서만 true)
	Keyboard      bool     // WH_KEYBOARD_LL도 건다(입력 세션 감지)
	Hotkeys       []Hotkey // 다른 프로그램이 쓰고 있어 못 건 것은 FailedHotkeys로 알린다
}

type msllHookStruct struct {
	Pt        win.Point
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type kbdllHookStruct struct {
	VKCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

const (
	whKeyboardLL  = 13
	whMouseLL     = 14
	wmKeyDown     = 0x0100
	wmKeyUp       = 0x0101
	wmSysKeyDown  = 0x0104
	wmSysKeyUp    = 0x0105
	wmLButtonDown = 0x0201
	wmRButtonDown = 0x0204
	llmhfInjected = 0x01
	llkhfInjected = 0x10
	llkhfAltDown  = 0x20   // 이 키가 눌릴 때 Alt가 같이 눌려 있었다(LLKHF_ALTDOWN)
	modNoRepeat   = 0x4000 // 누르고 있어도 단축키가 한 번만 온다
	vkTab         = 0x09
	vkReturn      = 0x0D
	vkShift       = 0x10
	vkControl     = 0x11
	vkLShift      = 0xA0
	vkRShift      = 0xA1
	vkLControl    = 0xA2
	vkRControl    = 0xA3
	vkLWin        = 0x5B
	vkRWin        = 0x5C
	vkInsert      = 0x2D
	vkDelete      = 0x2E
	vkV           = 0x56
	vkX           = 0x58
	vkY           = 0x59
	vkZ           = 0x5A
)

// modifierState는 그 순간 눌려 있는 보조 키(키 값이 아니라 눌림 여부)만 담는다.
type modifierState struct {
	Ctrl, Alt, Win, Shift bool
}

// expire는 마지막 키보드 이벤트(last)로부터 3초 넘게 지났으면(now 기준) 보조 키 상태를 지운다.
// 관리자 권한(UAC)으로 뜬 창은 무결성 수준이 달라(UIPI) 우리 훅으로 키 업 메시지가 안 올 수 있다 - Ctrl을
// 누르고 그 창으로 옮겨 가서 뗀 경우가 그렇다. 이때 뗀 걸 놓치면 Ctrl이 영원히 눌린 채로 남아 그 뒤의
// 모든 키가 "단축키"로만 보여 입력 세션 감지가 멈춘다. 떨어진 지 오래됐으면 그냥 지운다.
// ponytail: 보조 키를 3초 넘게 누르고 있다가 그 상태로 다른 키를 눌러 조합하면 일반 키로 잘못 보일 수 있다 -
// 훅 스레드가 아닌 곳에서 GetKeyState로 다시 확인하는 경로가 필요해지면 그때 추가한다.
func (m *modifierState) expire(now, last time.Time) {
	if now.Sub(last) > 3*time.Second {
		*m = modifierState{}
	}
}

type Hook struct {
	events        chan Click
	keys          chan Key
	hotkeys       chan int
	failed        []int
	done          chan struct{}
	threadID      uint32
	allowInjected bool
	maxNanos      atomic.Int64
	dropped       atomic.Int64  // events 채널이 꽉 차 버린 클릭 수
	mods          modifierState // 눌려 있는 Ctrl·Win·Shift(훅 스레드 안에서만 읽고 쓴다 - 콜백은 한 번에 하나씩 돈다)
	modsAt        time.Time     // 마지막 키보드 이벤트 시각(훅 스레드만, mods 고착 판정용)
}

// setModifier는 Ctrl·Win·Shift 키 자체의 up/down으로 상태를 갱신한다. Alt는 눌린 키의 LLKHF_ALTDOWN 플래그로
// 그때그때 알 수 있어 따로 상태를 들고 있지 않는다. GetAsyncKeyState는 쓰지 않는다
// (WH_KEYBOARD_LL 훅에서 그 함수를 부르는 건 키로거가 흔히 쓰는 패턴이라 백신·EDR이 의심한다).
func (h *Hook) setModifier(vk uint32, down bool) {
	switch vk {
	case vkControl, vkLControl, vkRControl:
		h.mods.Ctrl = down
	case vkLWin, vkRWin:
		h.mods.Win = down
	case vkShift, vkLShift, vkRShift:
		h.mods.Shift = down
	}
}

var (
	active atomic.Pointer[Hook] // 동시에 하나만 건다
	// mouseInfo·kbdInfo는 패키지 전역 버퍼 하나씩이다. 콜백은 훅 스레드 하나에서 차례로만 돈다(동시에 안 겹친다).
	// 그래서 이벤트마다 새로 할당하지 않고 이 버퍼에 덮어 쓴다. kbdInfo는 다 쓰자마자 지운다(스펙 9절: 키 입력 내용을
	// 메모리에도 남기지 않는다 - VK_PACKET일 때는 ScanCode가 입력된 유니코드 문자 그 자체다).
	mouseInfo msllHookStruct
	kbdInfo   kbdllHookStruct
	mouseProc = windows.NewCallback(func(code, wparam, lparam uintptr) uintptr {
		if h := active.Load(); h != nil && int32(code) >= 0 && (wparam == wmLButtonDown || wparam == wmRButtonDown) {
			start := time.Now()
			h.mods = modifierState{} // 클릭은 그 자체로 입력 세션 경계다 - 고착된 보조 키 상태가 있었다면 여기서 지운다
			win.CopyFrom(unsafe.Pointer(&mouseInfo), lparam, unsafe.Sizeof(mouseInfo))
			injected := mouseInfo.Flags&llmhfInjected != 0
			if !injected || h.allowInjected {
				select {
				case h.events <- Click{X: mouseInfo.Pt.X, Y: mouseInfo.Pt.Y, Right: wparam == wmRButtonDown, Injected: injected, At: start}:
				default: // 받는 쪽이 밀리면 버린다. 훅을 붙잡지 않는 게 우선이다
					h.dropped.Add(1)
				}
			}
			mouseInfo = msllHookStruct{}
			h.track(start)
		}
		r, _, _ := win.ProcCallNextHookEx.Call(0, code, wparam, lparam)
		return r
	})
	keyProc = windows.NewCallback(func(code, wparam, lparam uintptr) uintptr {
		isDown := wparam == wmKeyDown || wparam == wmSysKeyDown
		isUp := wparam == wmKeyUp || wparam == wmSysKeyUp
		if h := active.Load(); h != nil && int32(code) >= 0 && (isDown || isUp) {
			start := time.Now()
			win.CopyFrom(unsafe.Pointer(&kbdInfo), lparam, unsafe.Sizeof(kbdInfo))
			vk := kbdInfo.VKCode
			altDown := kbdInfo.Flags&llkhfAltDown != 0
			injected := kbdInfo.Flags&llkhfInjected != 0
			h.mods.expire(start, h.modsAt) // 키 업을 못 받아 고착된 보조 키는 오래 지나면 지운다(UIPI)
			h.modsAt = start
			h.setModifier(vk, isDown)
			if isDown {
				mods := modifierState{Ctrl: h.mods.Ctrl, Alt: altDown, Win: h.mods.Win, Shift: h.mods.Shift}
				if end, ok := classify(vk, mods); ok && (!injected || h.allowInjected) {
					select {
					case h.keys <- Key{End: end, Injected: injected, At: start}:
					default:
					}
				}
			}
			kbdInfo = kbdllHookStruct{}
			h.track(start)
		}
		r, _, _ := win.ProcCallNextHookEx.Call(0, code, wparam, lparam)
		return r
	})
)

// classify는 가상 키 코드와 그 순간의 보조 키 상태만으로 입력 세션 관점을 가른다. 키 값 자체는 이 함수 밖으로 나가지 않는다.
// ok=false인 키(보조 키, 기능 키, 이동 키, Ctrl·Alt·Win과 함께 누른 단축키)는 세션과 무관하다.
func classify(vk uint32, mods modifierState) (end, ok bool) {
	switch {
	case vk == vkReturn || vk == vkTab:
		return true, true
	case isEditCombo(vk, mods):
		return false, true
	case mods.Ctrl || mods.Alt || mods.Win:
		return false, false
	case vk >= 0x10 && vk <= 0x14, // Shift, Ctrl, Alt, Pause, CapsLock
		vk == 0x15 || vk == 0x19,     // 한/영, 한자
		vk == 0x1B,                   // Esc
		vk >= 0x21 && vk <= 0x28,     // PgUp·PgDn·End·Home·방향키
		vk == 0x2C || vk == vkInsert, // PrintScreen, Insert(토글 키. Shift+Insert는 위에서 이미 붙여넣기로 처리된다)
		vk >= 0x5B && vk <= 0x5D,     // Win, 메뉴 키
		vk >= 0x70 && vk <= 0x87,     // F1~F24
		vk == 0x90 || vk == 0x91,     // NumLock, ScrollLock
		vk >= 0xA0 && vk <= 0xB7:     // 좌우 Shift·Ctrl·Alt, 브라우저·미디어 키
		return false, false
	}
	return false, true
}

// isEditCombo는 붙여넣기·잘라내기·실행취소·다시실행(Ctrl+V/X/Z/Y, Shift+Insert, Shift+Delete)을 본다.
// 이 키들은 단축키이긴 해도 입력란 내용을 바꾸므로(세션은 끝나지 않는 "타이핑"으로) 다른 단축키와 다르게 다룬다.
func isEditCombo(vk uint32, mods modifierState) bool {
	switch {
	case mods.Ctrl && !mods.Alt && !mods.Win && (vk == vkV || vk == vkX || vk == vkZ || vk == vkY):
		return true
	case mods.Shift && (vk == vkInsert || vk == vkDelete):
		return true
	}
	return false
}

// track은 콜백 소요 시간의 최댓값을 남긴다(목표 10ms 이내, 스펙 10절). 콜백은 모두 훅 스레드에서 차례로 돈다.
func (h *Hook) track(start time.Time) {
	if d := time.Since(start).Nanoseconds(); d > h.maxNanos.Load() {
		h.maxNanos.Store(d)
	}
}

// StartMouseHook은 마우스 훅만 건다(M0 API).
func StartMouseHook(allowInjected bool) (*Hook, error) {
	return StartHook(HookOptions{AllowInjected: allowInjected})
}

// StartHook은 전용 OS 스레드에 마우스 훅(과 선택적으로 키보드 훅·전역 단축키)을 걸고 메시지 루프를 돌린다.
func StartHook(opt HookOptions) (*Hook, error) {
	h := &Hook{events: make(chan Click, 64), keys: make(chan Key, 64), hotkeys: make(chan int, 8),
		done: make(chan struct{}), allowInjected: opt.AllowInjected}
	if !active.CompareAndSwap(nil, h) {
		return nil, errors.New("이미 입력 훅이 실행 중이다")
	}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer close(h.done)
		h.threadID = windows.GetCurrentThreadId()
		hinst, _, _ := win.ProcGetModuleHandleW.Call(0)
		var hooks []uintptr
		set := func(id int, proc uintptr) error {
			hk, _, e := win.ProcSetWindowsHookExW.Call(uintptr(id), proc, hinst, 0)
			if hk == 0 {
				return fmt.Errorf("SetWindowsHookExW(%d): %w", id, e)
			}
			hooks = append(hooks, hk)
			return nil
		}
		err := set(whMouseLL, mouseProc)
		if err == nil && opt.Keyboard {
			err = set(whKeyboardLL, keyProc)
		}
		if err != nil {
			for _, hk := range hooks {
				win.ProcUnhookWindowsHookEx.Call(hk)
			}
			active.Store(nil)
			ready <- err
			return
		}
		// 단축키는 이 스레드에 걸어야 WM_HOTKEY가 이 스레드의 메시지 큐로 온다. 해제도 같은 스레드에서 한다(defer).
		for _, k := range opt.Hotkeys {
			if r, _, _ := win.ProcRegisterHotKey.Call(0, uintptr(k.ID), uintptr(k.Mods|modNoRepeat), uintptr(k.VK)); r == 0 {
				h.failed = append(h.failed, k.ID)
			} else {
				defer win.ProcUnregisterHotKey.Call(0, uintptr(k.ID))
			}
		}
		ready <- nil
		win.RunLoopFunc(func(m *win.Msg) {
			if m.Message == win.WM_HOTKEY {
				select {
				case h.hotkeys <- int(m.WParam):
				default:
				}
			}
		})
		for _, hk := range hooks {
			win.ProcUnhookWindowsHookEx.Call(hk)
		}
		active.CompareAndSwap(h, nil)
		close(h.events)
		close(h.keys)
		close(h.hotkeys)
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	return h, nil
}

func (h *Hook) Events() <-chan Click { return h.events }

// Keys는 HookOptions.Keyboard일 때만 값이 온다.
func (h *Hook) Keys() <-chan Key { return h.keys }

// Hotkeys는 눌린 단축키의 ID를 보낸다.
func (h *Hook) Hotkeys() <-chan int { return h.hotkeys }

// FailedHotkeys는 다른 프로그램이 이미 써서 등록하지 못한 단축키 ID다.
func (h *Hook) FailedHotkeys() []int { return h.failed }

func (h *Hook) MaxCallback() time.Duration { return time.Duration(h.maxNanos.Load()) }

// DroppedClicks는 events 채널(64칸)이 꽉 차 버린 클릭 수다. 0이 아니면 구독 쪽이 못 따라가고 있다는 뜻이다.
func (h *Hook) DroppedClicks() int64 { return h.dropped.Load() }

// Stop은 훅 스레드의 메시지 루프를 끝내고 훅과 단축키를 뗀다. 두 번 불러도 안전하다(이미 끝났으면 바로 돌아온다).
// SetWindowsHookExW를 부른 시점에 이미 이 스레드의 메시지 큐가 생겨 있으므로 PostThreadMessageW는 보통 바로 된다.
// 그래도 실패하면(스레드가 이미 죽은 드문 경우) 잠깐 재시도한다.
// ponytail: 그래도 안 끝나면 최대 ~2.2초(재시도 0.2초 + done 대기 2초) 뒤 그냥 돌아온다. 그 뒤에도 훅 스레드가
// 살아는 있다면 active는 h를 쥔 채로 남아 다음 StartHook이 "이미 입력 훅이 실행 중이다"로 계속 실패하고,
// SetWindowsHookExW로 건 훅과 단축키도 안 떨어진 채 남는다. 더 다듬으려면 타임아웃 뒤 그 핸들들을 강제로 정리하는
// 경로를 추가한다.
func (h *Hook) Stop() {
	select {
	case <-h.done:
		return
	default:
	}
	for retryUntil := time.Now().Add(200 * time.Millisecond); ; time.Sleep(10 * time.Millisecond) {
		if r, _, _ := win.ProcPostThreadMessageW.Call(uintptr(h.threadID), win.WM_QUIT, 0, 0); r != 0 || time.Now().After(retryUntil) {
			break
		}
	}
	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
	}
}
