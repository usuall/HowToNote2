// tesseract 외부 프로세스로 클릭 지점 주변 한 줄을 읽는다. UIA 이름이 없을 때만 쓰는 대체 경로다(스펙 4.3절).
// tesseract가 없으면 Find가 nil을 돌려주고, 호출 쪽은 OCR을 건너뛴다(번들은 M4).
// 화면 조각은 표준 입력으로 넘겨 디스크에 남기지 않는다(스펙 9절).
package ocr

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode"
)

const (
	roiW, roiH     = 360, 64    // 버튼 한 줄과 좌우 글자를 담는 크기(물리 픽셀)
	createNoWindow = 0x08000000 // GUI 앱에서 콘솔 창이 깜빡이지 않게
)

type Engine struct{ path string }

// Find는 설정 경로, PATH 순서로 tesseract.exe를 찾는다. 없으면 nil이다.
func Find(configured string) *Engine {
	if configured != "" {
		if st, err := os.Stat(configured); err == nil && !st.IsDir() {
			return &Engine{path: configured}
		}
	}
	if p, err := exec.LookPath("tesseract"); err == nil {
		return &Engine{path: p}
	}
	return nil
}

// TextAround는 img에서 p 주변 한 줄을 잘라 읽는다. 읽지 못하면 빈 문자열이다.
func (e *Engine) TextAround(img *image.RGBA, p image.Point, timeout time.Duration) string {
	r := image.Rect(p.X-roiW/2, p.Y-roiH/2, p.X+roiW/2, p.Y+roiH/2).Intersect(img.Bounds())
	if r.Empty() {
		return ""
	}
	var in bytes.Buffer
	if err := png.Encode(&in, upscale(img.SubImage(r).(*image.RGBA), 2)); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.path, "stdin", "stdout", "-l", "kor+eng", "--psm", "7") // psm 7: 한 줄
	cmd.Stdin = &in
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return clean(string(out))
}

// upscale은 작은 UI 글자를 tesseract가 읽기 좋게 n배로 키운다(최근접).
func upscale(src *image.RGBA, n int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*n, b.Dy()*n))
	for y := 0; y < dst.Rect.Dy(); y++ {
		for x := 0; x < dst.Rect.Dx(); x++ {
			dst.SetRGBA(x, y, src.RGBAAt(b.Min.X+x/n, b.Min.Y+y/n))
		}
	}
	return dst
}

// clean은 한 줄로 합치고, 글자나 숫자가 하나도 없으면(잡음) 버린다.
func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
		return ""
	}
	return s
}
