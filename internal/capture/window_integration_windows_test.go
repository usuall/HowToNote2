//go:build integration

// 테스트 창 위 지점의 창 정보(제목, 클래스, 프로세스, 경계)와 모니터, 캡처 제외 되돌리기를 확인한다.
package capture_test

import (
	"image"
	"strings"
	"testing"
	"time"

	"howtonote/internal/capture"
	"howtonote/internal/win"
)

func TestWindowAt_ReadsWindowInfo(t *testing.T) {
	w, err := win.OpenWindow(320, 240, 300, 200, 0x00FFFFFF, []win.Child{
		{Class: "BUTTON", Text: "확인", X: 20, Y: 20, W: 100, H: 30},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(300 * time.Millisecond)

	got := capture.WindowAt(360, 270) // 버튼 위
	if got.HWND != w.HWND || got.Class != "Button" || got.Title != "HowtoNote 테스트" {
		t.Errorf("창 = %+v", got)
	}
	if !strings.HasSuffix(got.Process, ".exe") || strings.ContainsAny(got.Process, `\/`) {
		t.Errorf("프로세스 이름 = %q, 경로 없는 exe 이름이어야 한다", got.Process)
	}
	if want := image.Rect(320, 240, 620, 440); got.Bounds != want {
		t.Errorf("경계 = %v, want %v", got.Bounds, want)
	}
	if got.IsIME() {
		t.Error("테스트 창은 IME 창이 아니다")
	}
	if fg := capture.ForegroundWindow(); fg.HWND == 0 {
		t.Error("앞 창을 찾지 못했다")
	}

	mon, scale, err := capture.MonitorAt(360, 270)
	if err != nil || !image.Pt(360, 270).In(mon) || scale < 1 {
		t.Errorf("모니터 = %v, 배율 %.2f, 오류 %v", mon, scale, err)
	}
}

func TestIncludeInCapture_UndoesExclude(t *testing.T) {
	r := grabUnder(t, func(h uintptr) error {
		if err := capture.ExcludeFromCapture(h); err != nil {
			return err
		}
		return capture.IncludeInCapture(h)
	})
	if r < 0.9 {
		t.Fatalf("되돌린 창이 찍혀야 하는데 자홍색 비율 %.2f", r)
	}
}
