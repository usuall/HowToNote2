//go:build integration

// 자홍색 창을 띄우고 BitBlt로 찍어, 캡처 제외를 켠 창만 사진에서 빠지는지 본다.
package capture_test

import (
	"image"
	"os"
	"testing"
	"time"

	"howtonote/internal/capture"
	"howtonote/internal/win"
)

func TestMain(m *testing.M) {
	_ = win.SetPerMonitorV2()
	os.Exit(m.Run())
}

const magenta = 0x00FF00FF // COLORREF 0x00BBGGRR

func magentaRatio(img *image.RGBA) float64 {
	n := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 240 && img.Pix[i+1] < 15 && img.Pix[i+2] > 240 {
			n++
		}
	}
	return float64(n) / float64(len(img.Pix)/4)
}

func grabUnder(t *testing.T, setup func(uintptr) error) float64 {
	t.Helper()
	w, err := win.OpenWindow(300, 300, 200, 100, magenta, nil, setup)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(500 * time.Millisecond) // DWM이 창을 합성할 시간
	img, err := capture.GrabRect(300, 300, 200, 100)
	if err != nil {
		t.Fatal(err)
	}
	return magentaRatio(img)
}

func TestGrabRect_SeesNormalWindow(t *testing.T) {
	if r := grabUnder(t, nil); r < 0.9 {
		t.Fatalf("일반 창이 찍혀야 하는데 자홍색 비율 %.2f", r)
	}
}

func TestExcludeFromCapture_HidesWindow(t *testing.T) {
	if r := grabUnder(t, capture.ExcludeFromCapture); r > 0.01 {
		t.Fatalf("캡처 제외 창이 찍혔다: 자홍색 비율 %.2f", r)
	}
}
