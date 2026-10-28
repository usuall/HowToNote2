//go:build integration

// 테스트용 창을 전용 OS 스레드에서 띄운다. 창 스레드가 메시지를 돌려야 UIA·캡처 테스트가 동작한다.
package win

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Child struct {
	Class, Text string
	Style       uint32
	X, Y, W, H  int32
}

type Window struct {
	HWND uintptr
	done chan struct{}
}

// wmTestCall은 Do가 창 스레드에 일을 넘기는 메시지다(WM_APP 영역).
const wmTestCall = 0x8000 + 1

var (
	classMu  sync.Mutex
	classes  = map[uint32]*uint16{}
	callMu   sync.Mutex
	callFn   func() // Do가 넘긴 일. callMu를 쥔 채 SendMessage로 기다리므로 한 번에 하나다
	testProc = windows.NewCallback(func(hwnd, msg, wp, lp uintptr) uintptr {
		switch msg {
		case WM_DESTROY:
			ProcPostQuitMessage.Call(0)
			return 0
		case wmTestCall:
			callFn()
			return 0
		}
		r, _, _ := ProcDefWindowProcW.Call(hwnd, msg, wp, lp)
		return r
	})
)

// classFor는 배경색(COLORREF 0x00BBGGRR)마다 창 클래스를 하나씩 등록한다.
func classFor(color uint32) (*uint16, error) {
	classMu.Lock()
	defer classMu.Unlock()
	if c, ok := classes[color]; ok {
		return c, nil
	}
	name, _ := windows.UTF16PtrFromString(fmt.Sprintf("HowtoNoteTest%06X", color))
	brush, _, _ := ProcCreateSolidBrush.Call(uintptr(color))
	hinst, _, _ := ProcGetModuleHandleW.Call(0)
	wc := wndClassExW{WndProc: testProc, Instance: hinst, Background: brush, ClassName: name}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, e := ProcRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return nil, fmt.Errorf("RegisterClassExW: %w", e)
	}
	classes[color] = name
	return name, nil
}

// OpenWindow는 항상 위에 뜨는 테두리 없는 창과 자식 컨트롤을 만든다. setup은 창 스레드에서 실행된다.
func OpenWindow(x, y, w, h int32, color uint32, children []Child, setup func(hwnd uintptr) error) (*Window, error) {
	win := &Window{done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer close(win.done)
		cls, err := classFor(color)
		if err != nil {
			ready <- err
			return
		}
		hinst, _, _ := ProcGetModuleHandleW.Call(0)
		title, _ := windows.UTF16PtrFromString("HowtoNote 테스트")
		hwnd, _, e := ProcCreateWindowExW.Call(WS_EX_TOPMOST|WS_EX_TOOLWINDOW,
			uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)),
			WS_POPUP|WS_VISIBLE, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, hinst, 0)
		if hwnd == 0 {
			ready <- fmt.Errorf("CreateWindowExW: %w", e)
			return
		}
		for _, c := range children {
			cc, _ := windows.UTF16PtrFromString(c.Class)
			ct, _ := windows.UTF16PtrFromString(c.Text)
			ch, _, e := ProcCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cc)), uintptr(unsafe.Pointer(ct)),
				uintptr(WS_CHILD|WS_VISIBLE|c.Style), uintptr(c.X), uintptr(c.Y), uintptr(c.W), uintptr(c.H), hwnd, 0, hinst, 0)
			if ch == 0 {
				ProcDestroyWindow.Call(hwnd)
				ready <- fmt.Errorf("자식 %s 생성: %w", c.Class, e)
				return
			}
		}
		if setup != nil {
			if err := setup(hwnd); err != nil {
				ProcDestroyWindow.Call(hwnd)
				ready <- err
				return
			}
		}
		win.HWND = hwnd
		ready <- nil
		RunLoop()
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	return win, nil
}

// Do는 f를 창 스레드에서 실행하고 끝날 때까지 기다린다. 입력 컨텍스트(IME)처럼 그 창의 스레드에서만 바꿀 수 있는
// 상태를 다룰 때 쓴다.
func (w *Window) Do(f func()) {
	callMu.Lock()
	defer callMu.Unlock()
	callFn = f
	ProcSendMessageW.Call(w.HWND, wmTestCall, 0, 0)
	callFn = nil
}

// Close는 창을 닫고 창 스레드가 끝날 때까지 기다린다.
func (w *Window) Close() {
	ProcPostMessageW.Call(w.HWND, WM_CLOSE, 0, 0)
	<-w.done
}
