// Win32 API 얇은 래퍼. HowtoNote가 쓰는 함수·구조체·상수만 둔다.
package win

import (
	"fmt"
	"image"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")

	ProcRegisterClassExW              = user32.NewProc("RegisterClassExW")
	ProcCreateWindowExW               = user32.NewProc("CreateWindowExW")
	ProcDefWindowProcW                = user32.NewProc("DefWindowProcW")
	ProcDestroyWindow                 = user32.NewProc("DestroyWindow")
	ProcPostMessageW                  = user32.NewProc("PostMessageW")
	ProcPostThreadMessageW            = user32.NewProc("PostThreadMessageW")
	ProcPostQuitMessage               = user32.NewProc("PostQuitMessage")
	ProcGetMessageW                   = user32.NewProc("GetMessageW")
	ProcPeekMessageW                  = user32.NewProc("PeekMessageW")
	ProcTranslateMessage              = user32.NewProc("TranslateMessage")
	ProcDispatchMessageW              = user32.NewProc("DispatchMessageW")
	ProcSetWindowDisplayAffinity      = user32.NewProc("SetWindowDisplayAffinity")
	ProcSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	ProcGetDC                         = user32.NewProc("GetDC")
	ProcReleaseDC                     = user32.NewProc("ReleaseDC")
	ProcGetCursorPos                  = user32.NewProc("GetCursorPos")
	ProcSetCursorPos                  = user32.NewProc("SetCursorPos")
	ProcSendInput                     = user32.NewProc("SendInput")
	ProcSetWindowsHookExW             = user32.NewProc("SetWindowsHookExW")
	ProcCallNextHookEx                = user32.NewProc("CallNextHookEx")
	ProcUnhookWindowsHookEx           = user32.NewProc("UnhookWindowsHookEx")
	ProcGetWindowTextW                = user32.NewProc("GetWindowTextW")
	ProcWindowFromPoint               = user32.NewProc("WindowFromPoint")
	ProcGetAncestor                   = user32.NewProc("GetAncestor")
	ProcMonitorFromPoint              = user32.NewProc("MonitorFromPoint")
	ProcMonitorFromWindow             = user32.NewProc("MonitorFromWindow")
	ProcGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	ProcRegisterHotKey                = user32.NewProc("RegisterHotKey")
	ProcUnregisterHotKey              = user32.NewProc("UnregisterHotKey")
	ProcAttachThreadInput             = user32.NewProc("AttachThreadInput")
	ProcSetFocus                      = user32.NewProc("SetFocus")
	ProcShowWindow                    = user32.NewProc("ShowWindow")
	ProcSetWindowPos                  = user32.NewProc("SetWindowPos")
	ProcGetWindowPlacement            = user32.NewProc("GetWindowPlacement")
	ProcSetWindowPlacement            = user32.NewProc("SetWindowPlacement")
	ProcGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	ProcGetDoubleClickTime            = user32.NewProc("GetDoubleClickTime")
	ProcGetSystemMetrics              = user32.NewProc("GetSystemMetrics")

	ProcCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	ProcCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	ProcSelectObject           = gdi32.NewProc("SelectObject")
	ProcBitBlt                 = gdi32.NewProc("BitBlt")
	ProcGetDIBits              = gdi32.NewProc("GetDIBits")
	ProcDeleteObject           = gdi32.NewProc("DeleteObject")
	ProcDeleteDC               = gdi32.NewProc("DeleteDC")
	ProcCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")

	ProcGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	ProcRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")
	ProcLstrlenW         = kernel32.NewProc("lstrlenW")

	ProcGetDpiForMonitor = shcore.NewProc("GetDpiForMonitor")
)

const (
	WM_DESTROY = 0x0002
	WM_CLOSE   = 0x0010
	WM_QUIT    = 0x0012
	WM_HOTKEY  = 0x0312
	PM_REMOVE  = 0x0001

	WS_POPUP            = 0x80000000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_EX_TOPMOST       = 0x00000008
	WS_EX_TOOLWINDOW    = 0x00000080
	ES_PASSWORD         = 0x0020
	ES_READONLY         = 0x0800

	WDA_EXCLUDEFROMCAPTURE = 0x00000011
	WDA_NONE               = 0x00000000

	GA_ROOT                  = 2
	MONITOR_DEFAULTTONEAREST = 2
	MDT_EFFECTIVE_DPI        = 0

	SW_RESTORE     = 9
	HWND_TOPMOST   = ^uintptr(0) // (HWND)-1
	HWND_NOTOPMOST = ^uintptr(1) // (HWND)-2
	SWP_NOSIZE     = 0x0001
	SWP_NOMOVE     = 0x0002
	SWP_NOACTIVATE = 0x0010

	SM_CXDOUBLECLK = 36 // 더블클릭으로 보는 사각형의 너비(px)
	SM_CYDOUBLECLK = 37
)

type Point struct{ X, Y int32 }

type Msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      Point
	Private uint32
}

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

// SetPerMonitorV2는 프로세스를 Per-Monitor V2 DPI 인식으로 만든다. 그래야 좌표가 물리 픽셀이 된다.
// 매니페스트로 이미 켜져 있으면 실패하는데, 그 경우는 무시해도 된다.
func SetPerMonitorV2() error {
	r, _, err := ProcSetProcessDpiAwarenessContext.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
	if r == 0 {
		return err
	}
	return nil
}

// Pump는 이 스레드에 쌓인 메시지를 모두 처리하고 바로 돌아온다.
func Pump() {
	m := new(Msg)
	for {
		r, _, _ := ProcPeekMessageW.Call(uintptr(unsafe.Pointer(m)), 0, 0, 0, PM_REMOVE)
		if r == 0 {
			return
		}
		ProcTranslateMessage.Call(uintptr(unsafe.Pointer(m)))
		ProcDispatchMessageW.Call(uintptr(unsafe.Pointer(m)))
	}
}

// RunLoop는 WM_QUIT을 받을 때까지 메시지 루프를 돈다.
func RunLoop() { RunLoopFunc(nil) }

// RunLoopFunc는 RunLoop와 같되, 창이 없는 스레드 메시지(hwnd 0, 예: RegisterHotKey의 WM_HOTKEY)를 onThread로 넘긴다.
// DispatchMessage는 hwnd가 0인 메시지를 아무 데도 보내지 않기 때문이다.
func RunLoopFunc(onThread func(m *Msg)) {
	m := new(Msg)
	for {
		r, _, _ := ProcGetMessageW.Call(uintptr(unsafe.Pointer(m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if m.Hwnd == 0 && onThread != nil {
			onThread(m)
			continue
		}
		ProcTranslateMessage.Call(uintptr(unsafe.Pointer(m)))
		ProcDispatchMessageW.Call(uintptr(unsafe.Pointer(m)))
	}
}

// ComCall은 COM 객체의 vtable slot 번째 메서드를 부른다. 첫 인자(this)는 자동으로 넣는다.
// args로 넘기는 포인터(출력 인자 등)를 uintptr로 바꿔 넘기면, 탈출 분석은 그 변환을 추적하지 못해
// new()로 만든 값도 스택에 남을 수 있다(실제로 escape analysis가 "does not escape"라고 판단한 사례가 있었다).
// 아래 //go:uintptrescapes 지시어가 golang.org/x/sys/windows의 LazyProc.Call과 같은 방식으로
// 이 함수에 uintptr로 들어오는 인자를 호출이 끝날 때까지 힙에 붙들어 둔다.
//
//go:uintptrescapes
func ComCall(obj unsafe.Pointer, slot int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, slot*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

// Failed는 HRESULT가 실패 코드인지 본다. 하위 32비트만 의미가 있고, S_FALSE 같은 양수는 성공이다.
func Failed(hr uintptr) bool { return int32(hr) < 0 }

// StorePtr는 OS가 넘긴 주소 dst에 포인터 크기 값 v를 쓴다(COM 출력 인자용).
func StorePtr(dst, v uintptr) {
	ProcRtlMoveMemory.Call(dst, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
}

// CopyFrom은 OS가 넘긴 주소 src에서 n바이트를 dst로 복사한다.
// uintptr을 Go 포인터로 바꾸지 않아 go vet 경고가 없다.
func CopyFrom(dst unsafe.Pointer, src uintptr, n uintptr) {
	ProcRtlMoveMemory.Call(uintptr(dst), src, n)
}

// UTF16FromPtr는 OS가 넘긴 NUL 종료 UTF-16 문자열을 Go 문자열로 복사한다.
func UTF16FromPtr(p uintptr) string {
	if p == 0 {
		return ""
	}
	n, _, _ := ProcLstrlenW.Call(p)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	CopyFrom(unsafe.Pointer(&buf[0]), p, n*2)
	return windows.UTF16ToString(buf)
}

var (
	hiddenOnce  sync.Once
	hiddenClass *uint16
	hiddenErr   error
	defProc     = windows.NewCallback(func(hwnd, msg, wp, lp uintptr) uintptr {
		r, _, _ := ProcDefWindowProcW.Call(hwnd, msg, wp, lp)
		return r
	})
)

// CreateHiddenWindow는 화면에 보이지 않는 최상위 창을 현재 스레드에 만든다(숨긴 WebView2의 부모).
func CreateHiddenWindow(w, h int32) (uintptr, error) {
	hiddenOnce.Do(func() {
		hiddenClass, _ = windows.UTF16PtrFromString("HowtoNoteHidden")
		hinst, _, _ := ProcGetModuleHandleW.Call(0)
		wc := wndClassExW{WndProc: defProc, Instance: hinst, ClassName: hiddenClass}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, e := ProcRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			hiddenErr = fmt.Errorf("RegisterClassExW: %w", e)
		}
	})
	if hiddenErr != nil {
		return 0, hiddenErr
	}
	hinst, _, _ := ProcGetModuleHandleW.Call(0)
	hwnd, _, e := ProcCreateWindowExW.Call(0, uintptr(unsafe.Pointer(hiddenClass)), 0,
		WS_OVERLAPPEDWINDOW, 0, 0, uintptr(w), uintptr(h), 0, 0, hinst, 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW: %w", e)
	}
	return hwnd, nil
}

// PointArg는 POINT를 값으로 받는 API(WindowFromPoint, MonitorFromPoint, ElementFromPoint)에 넘길 인자다.
// x64에서 POINT(8바이트)는 레지스터 하나로 넘어간다.
func PointArg(x, y int32) uintptr { return uintptr(uint32(x)) | uintptr(uint32(y))<<32 }

// ClassName은 창 클래스 이름이다. 실패하면 빈 문자열.
func ClassName(hwnd uintptr) string {
	buf := make([]uint16, 256)
	n, err := windows.GetClassName(windows.HWND(hwnd), &buf[0], int32(len(buf)))
	if err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

type monitorInfo struct {
	Size    uint32
	Monitor windows.Rect
	Work    windows.Rect
	Flags   uint32
}

// MonitorRects는 모니터 핸들의 전체 영역과 작업 영역(작업 표시줄 제외)을 물리 픽셀로 돌려준다.
func MonitorRects(hmon uintptr) (full, work image.Rectangle, ok bool) {
	mi := new(monitorInfo)
	mi.Size = uint32(unsafe.Sizeof(*mi))
	if r, _, _ := ProcGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(mi))); r == 0 {
		return full, work, false
	}
	m, w := mi.Monitor, mi.Work
	return image.Rect(int(m.Left), int(m.Top), int(m.Right), int(m.Bottom)),
		image.Rect(int(w.Left), int(w.Top), int(w.Right), int(w.Bottom)), true
}

// Placement는 WINDOWPLACEMENT다. 최대화 여부까지 담겨 녹화 전 창 상태를 그대로 되돌릴 수 있다.
type Placement struct {
	Length  uint32
	Flags   uint32
	ShowCmd uint32
	MinPos  Point
	MaxPos  Point
	Normal  windows.Rect
}

// GetPlacement는 창 배치를 읽는다. 실패하면 ok=false다: 빈 배치의 ShowCmd 0은 SW_HIDE라 그대로 되돌리면 창이 사라진다.
func GetPlacement(hwnd uintptr) (p Placement, ok bool) {
	p.Length = uint32(unsafe.Sizeof(p))
	r, _, _ := ProcGetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&p)))
	return p, r != 0
}

func SetPlacement(hwnd uintptr, p Placement) {
	p.Length = uint32(unsafe.Sizeof(p))
	ProcSetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&p)))
}
