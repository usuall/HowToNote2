// 마우스 아래 요소, UIA 응답 시간, 창 클래스를 1초마다 출력한다. 앱 종류별 실측, IME 후보창 클래스 확인, 입력란의
// 읽기 전용·Value 패턴 여부(입력 세션이 열리는지) 확인용. Ctrl+C로 끝낸다.
package main

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"howtonote/internal/capture"
	"howtonote/internal/uia"
	"howtonote/internal/win"
)

func main() {
	_ = win.SetPerMonitorV2()
	s, err := uia.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer s.Close()
	p := new(win.Point)
	for {
		win.ProcGetCursorPos.Call(uintptr(unsafe.Pointer(p)))
		start := time.Now()
		el, err := s.ElementAt(p.X, p.Y, 2*time.Second)
		ms := time.Since(start).Milliseconds()
		value := el.Value
		if el.IsPassword {
			value = "●●●●"
		}
		w := capture.WindowAt(p.X, p.Y)
		fmt.Printf("%5dms (%d,%d) 종류=%d(%s) 이름=%q 값=%q 읽기전용=%v Value패턴=%v id=%q pid=%d 창=%q 클래스=%s/%s IME=%v 오류=%v\n",
			ms, p.X, p.Y, el.ControlType, el.LocalizedType, el.Name, value, el.ReadOnly, el.HasValue, el.AutomationID, el.ProcessID,
			w.Title, w.Class, w.RootClass, w.IsIME(), err)
		time.Sleep(time.Second)
	}
}
