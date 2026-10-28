// OCR 단위 테스트: tesseract 찾기, 결과 정리, 확대. tesseract 없이 돈다.
package ocr

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestFind_MissingReturnsNil(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if e := Find(""); e != nil {
		t.Fatalf("PATH에 없으면 nil이어야 한다: %+v", e)
	}
	if e := Find(filepath.Join(t.TempDir(), "tesseract.exe")); e != nil {
		t.Fatalf("없는 설정 경로면 nil이어야 한다: %+v", e)
	}
}

func TestFind_UsesConfiguredFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tesseract.exe")
	if err := os.WriteFile(p, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if e := Find(p); e == nil || e.path != p {
		t.Fatalf("설정 경로를 써야 한다: %+v", e)
	}
}

func TestClean(t *testing.T) {
	cases := []struct{ in, want string }{
		{" 저장 \r\n\n", "저장"},
		{"파일  열기\n", "파일 열기"},
		{"|| — ..\n", ""}, // 글자·숫자 없는 잡음
	}
	for _, c := range cases {
		if got := clean(c.in); got != c.want {
			t.Errorf("clean(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUpscale(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	src.SetRGBA(2, 1, color.RGBA{R: 255, A: 255})
	sub := src.SubImage(image.Rect(2, 1, 4, 3)).(*image.RGBA) // 원점이 (2,1)인 부분 이미지도 다뤄야 한다
	dst := upscale(sub, 2)
	if dst.Bounds() != image.Rect(0, 0, 4, 4) {
		t.Fatalf("크기 = %v", dst.Bounds())
	}
	if dst.RGBAAt(1, 1).R != 255 || dst.RGBAAt(2, 0).R != 0 {
		t.Error("픽셀 복제가 틀렸다")
	}
}
