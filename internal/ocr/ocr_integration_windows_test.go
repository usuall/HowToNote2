//go:build integration

// 실제 창의 글자를 찍어 tesseract로 읽는다. tesseract가 없으면 건너뛴다(번들은 M4).
package ocr_test

import (
	"image"
	"strings"
	"testing"
	"time"

	"howtonote/internal/capture"
	"howtonote/internal/ocr"
	"howtonote/internal/win"
)

func TestTextAround_ReadsStaticText(t *testing.T) {
	_ = win.SetPerMonitorV2()
	e := ocr.Find("")
	if e == nil {
		t.Skip("tesseract 없음: PATH에 넣거나 settings.json의 tesseract_path로 지정한다")
	}
	w, err := win.OpenWindow(300, 300, 400, 120, 0x00FFFFFF, []win.Child{
		{Class: "STATIC", Text: "Save Settings", X: 40, Y: 40, W: 300, H: 30},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(500 * time.Millisecond)
	img, err := capture.GrabRect(300, 300, 400, 120)
	if err != nil {
		t.Fatal(err)
	}
	got := e.TextAround(img, image.Pt(120, 52), 5*time.Second)
	if !strings.Contains(strings.ToLower(got), "save") {
		t.Fatalf("OCR = %q, 'Save'가 있어야 한다", got)
	}
}
