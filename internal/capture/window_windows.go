// 클릭 지점의 모니터·창 정보(경계, 제목, 프로세스)와 녹화에서 무시할 창(IME 후보창) 판별.
package capture

import (
	"errors"
	"image"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"howtonote/internal/win"
)

// Window는 화면 한 지점 아래의 최상위 창이다.
type Window struct {
	HWND      uintptr
	Class     string // 지점 바로 아래 창(자식일 수 있다)의 클래스
	RootClass string // 최상위 창의 클래스
	Title     string
	Process   string          // 실행 파일 이름(예: notepad.exe). 경로는 남기지 않는다
	Bounds    image.Rectangle // 그림자를 뺀 창 경계(DWM 확장 프레임, 물리 픽셀)
}

// imeClasses는 IME 후보창·입력기 UI의 창 클래스다. 여기를 누르는 것은 글자 고르기라 스텝으로 남기지 않는다.
// ponytail: 알려진 클래스만 둔다. 다른 IME에서 스텝이 생기면 cmd/uiaprobe로 클래스를 확인해 더한다
var imeClasses = map[string]bool{
	"Microsoft.IME.UIManager.CandidateWindow.Host": true, // Windows 10/11 Microsoft IME 후보창
	"CiceroUIWndFrame": true, // TSF 후보창·입력기 막대
	"MSCTFIME UI":      true,
	"IME":              true,
}

func (w Window) IsIME() bool { return imeClasses[w.Class] || imeClasses[w.RootClass] }

// MonitorAt은 (x, y)가 있는(없으면 가장 가까운) 모니터의 영역과 배율(1.0 = 96dpi)을 돌려준다.
func MonitorAt(x, y int32) (image.Rectangle, float64, error) {
	hmon, _, _ := win.ProcMonitorFromPoint.Call(win.PointArg(x, y), win.MONITOR_DEFAULTTONEAREST)
	full, _, ok := win.MonitorRects(hmon)
	if !ok {
		return image.Rectangle{}, 1, errors.New("GetMonitorInfoW 실패")
	}
	scale := 1.0
	dx, dy := new(uint32), new(uint32)
	if r, _, _ := win.ProcGetDpiForMonitor.Call(hmon, win.MDT_EFFECTIVE_DPI, uintptr(unsafe.Pointer(dx)), uintptr(unsafe.Pointer(dy))); !win.Failed(r) && *dx > 0 {
		scale = float64(*dx) / 96
	}
	return full, scale, nil
}

// WindowAt은 (x, y) 아래 최상위 창이다. 메뉴 팝업(#32768) 위라면 메뉴를 띄운 앞 창을 쓴다(메뉴 조각만 찍히지 않게).
func WindowAt(x, y int32) Window {
	child, _, _ := win.ProcWindowFromPoint.Call(win.PointArg(x, y))
	root, _, _ := win.ProcGetAncestor.Call(child, win.GA_ROOT)
	w := Window{HWND: root, Class: win.ClassName(child), RootClass: win.ClassName(root)}
	if w.RootClass == "#32768" {
		w.HWND = uintptr(windows.GetForegroundWindow())
	}
	describe(&w)
	return w
}

// ForegroundWindow는 지금 앞에 있는 창이다. 입력 스텝처럼 클릭 지점이 없을 때 쓴다.
func ForegroundWindow() Window {
	h := uintptr(windows.GetForegroundWindow())
	w := Window{HWND: h, Class: win.ClassName(h), RootClass: win.ClassName(h)}
	describe(&w)
	return w
}

// describe는 제목, 경계, 프로세스 이름을 채운다.
func describe(w *Window) {
	if w.HWND == 0 {
		return
	}
	buf := make([]uint16, 512)
	n, _, _ := win.ProcGetWindowTextW.Call(w.HWND, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	w.Title = windows.UTF16ToString(buf[:n])
	r := new(windows.Rect)
	if windows.DwmGetWindowAttribute(windows.HWND(w.HWND), windows.DWMWA_EXTENDED_FRAME_BOUNDS, unsafe.Pointer(r), uint32(unsafe.Sizeof(*r))) == nil {
		w.Bounds = image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom))
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(windows.HWND(w.HWND), &pid); err == nil {
		w.Process = processName(pid)
	}
}

func processName(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:n]))
}
