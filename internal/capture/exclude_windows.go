// 녹화 툴바가 스크린샷에 찍히지 않게 한다(Windows 10 2004 이상). 녹화가 끝나면 되돌린다.
package capture

import (
	"fmt"

	"howtonote/internal/win"
)

// ExcludeFromCapture가 실패하면 OS가 지원하지 않는 것이다. 이때 툴바가 스크린샷에 찍힐 수 있다.
func ExcludeFromCapture(hwnd uintptr) error { return setAffinity(hwnd, win.WDA_EXCLUDEFROMCAPTURE) }

// IncludeInCapture는 ExcludeFromCapture를 되돌린다. 녹화 밖에서는 다른 캡처 도구에 창이 보여야 한다.
func IncludeInCapture(hwnd uintptr) error { return setAffinity(hwnd, win.WDA_NONE) }

func setAffinity(hwnd, mode uintptr) error {
	r, _, e := win.ProcSetWindowDisplayAffinity.Call(hwnd, mode)
	if r == 0 {
		return fmt.Errorf("SetWindowDisplayAffinity: %w", e)
	}
	return nil
}
