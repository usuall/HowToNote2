//go:build integration

// 테스트 창 위에서 SendInput으로 클릭을 만들고, 전역 훅이 좌표와 주입 여부를 전하는지 본다.
// 실제 마우스를 움직이므로 실행 중에는 마우스를 건드리지 않는다.
package capture_test

import (
	"testing"
	"time"

	"howtonote/internal/capture"
	"howtonote/internal/win"
)

func clickAt(t *testing.T, x, y int32) {
	t.Helper()
	if err := win.Click(x, y); err != nil {
		t.Fatalf("SendInput: %v", err)
	}
}

func openTarget(t *testing.T) *win.Window {
	t.Helper()
	w, err := win.OpenWindow(400, 400, 200, 120, 0x00FFFFFF, nil, nil) // 클릭이 다른 앱에 닿지 않게 받침 창을 깐다
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	return w
}

func TestMouseHook_ReportsInjectedClick(t *testing.T) {
	w := openTarget(t)
	defer w.Close()
	h, err := capture.StartMouseHook(true)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	clickAt(t, 500, 460)
	select {
	case c := <-h.Events():
		if c.X != 500 || c.Y != 460 || !c.Injected || c.Right {
			t.Errorf("클릭 = %+v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("클릭 이벤트가 오지 않았다")
	}
	if d := h.MaxCallback(); d > 10*time.Millisecond {
		t.Errorf("훅 콜백 최대 %v, 10ms 이내여야 한다", d)
	}
}

func TestMouseHook_IgnoresInjectedByDefault(t *testing.T) {
	w := openTarget(t)
	defer w.Close()
	h, err := capture.StartMouseHook(false)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	clickAt(t, 500, 460)
	select {
	case c := <-h.Events():
		t.Fatalf("주입된 클릭은 무시해야 한다: %+v", c)
	case <-time.After(500 * time.Millisecond):
	}
}
