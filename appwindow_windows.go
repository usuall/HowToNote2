// 앱 창 직접 다루기: 우리 HWND 찾기, 녹화 툴바 모드로 줄이기, 녹화 전 상태로 되돌리기(스펙 3.3절).
// Wails v2는 창이 하나이고 HWND를 내주지 않아, 같은 프로세스의 wailsWindow 클래스 창을 찾아 Win32로 다룬다.
package main

import (
	"sync"

	"golang.org/x/sys/windows"

	"howtonote/internal/win"
)

// wailsClass는 Wails v2.16.0 internal/frontend/desktop/windows/window.go의 기본 창 클래스 이름이다
// (options.Windows.WindowClassName을 비워 둔 경우).
const wailsClass = "wailsWindow"

// 툴바 크기(DIP). 썸네일, 상태, 글자 라벨 버튼 두 개, 단축키 안내가 들어가는 크기로 창 제목 표시줄 높이를 포함한다.
const (
	toolbarW      = 460
	toolbarH      = 170
	toolbarMargin = 16
)

var (
	findMu   sync.Mutex
	findPID  uint32
	found    uintptr
	findProc = windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid)
		if pid == findPID && win.ClassName(hwnd) == wailsClass {
			found = hwnd
			return 0 // 찾았으니 그만 돈다
		}
		return 1
	})
)

// findOwnWindow는 이 프로세스의 Wails 창을 찾는다. 없으면 0이다.
func findOwnWindow() uintptr {
	findMu.Lock()
	defer findMu.Unlock()
	findPID, found = windows.GetCurrentProcessId(), 0
	_ = windows.EnumWindows(findProc, nil) // 콜백이 0으로 멈추면 오류를 돌려주므로 무시한다
	return found
}

// enterToolbar는 창을 작업 영역 오른쪽 위의 작은 항상 위 창으로 바꾸고, 되돌릴 상태를 돌려준다(못 읽었으면 nil).
func enterToolbar(hwnd uintptr) *win.Placement {
	var saved *win.Placement
	if p, ok := win.GetPlacement(hwnd); ok {
		saved = &p
	}
	win.ProcShowWindow.Call(hwnd, win.SW_RESTORE) // 최대화를 풀어야 크기를 바꿀 수 있다
	hmon, _, _ := win.ProcMonitorFromWindow.Call(hwnd, win.MONITOR_DEFAULTTONEAREST)
	_, work, _ := win.MonitorRects(hmon)
	dpi, _, _ := win.ProcGetDpiForWindow.Call(hwnd)
	if dpi == 0 {
		dpi = 96
	}
	px := func(dip int) int { return dip * int(dpi) / 96 }
	w, h := px(toolbarW), px(toolbarH)
	x, y := work.Max.X-w-px(toolbarMargin), work.Min.Y+px(toolbarMargin)
	win.ProcSetWindowPos.Call(hwnd, win.HWND_TOPMOST, uintptr(x), uintptr(y), uintptr(w), uintptr(h), win.SWP_NOACTIVATE)
	return saved
}

// restoreWindow는 항상 위를 풀고 녹화 전 위치와 상태(최대화 포함)로 되돌린다. 녹화 전 상태를 못 읽었으면(saved nil)
// 툴바 크기로 둔다: 사용자가 창 크기를 바꿀 수는 있지만, 숨겨진 창은 되찾을 길이 없다.
func restoreWindow(hwnd uintptr, saved *win.Placement) {
	win.ProcSetWindowPos.Call(hwnd, win.HWND_NOTOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOACTIVATE)
	if saved != nil {
		win.SetPlacement(hwnd, *saved)
	}
}
