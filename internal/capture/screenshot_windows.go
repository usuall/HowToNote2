// GDI BitBlt 화면 캡처. 프로세스가 Per-Monitor V2 DPI 인식이면 좌표와 결과가 물리 픽셀이다.
package capture

import (
	"errors"
	"fmt"
	"image"
	"unsafe"

	"howtonote/internal/win"
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

const (
	srcCopy    = 0x00CC0020
	captureBlt = 0x40000000  // 레이어드 창(반투명 창)까지 찍는다
	hgdiError  = ^uintptr(0) // SelectObject 실패 값(HGDI_ERROR)
)

// GrabRect는 화면의 (x, y, w, h) 영역을 찍는다. GDI 핸들을 만들지 못하면(메모리 부족 등) 빈 이미지 대신 오류를 돌려준다.
func GrabRect(x, y, w, h int) (*image.RGBA, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("캡처 크기가 잘못되었다: %dx%d", w, h)
	}
	screen, _, _ := win.ProcGetDC.Call(0)
	if screen == 0 {
		return nil, errors.New("GetDC 실패")
	}
	defer win.ProcReleaseDC.Call(0, screen)
	mem, _, _ := win.ProcCreateCompatibleDC.Call(screen)
	if mem == 0 {
		return nil, errors.New("CreateCompatibleDC 실패")
	}
	defer win.ProcDeleteDC.Call(mem)
	bmp, _, _ := win.ProcCreateCompatibleBitmap.Call(screen, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap(%dx%d) 실패", w, h)
	}
	defer win.ProcDeleteObject.Call(bmp)
	old, _, _ := win.ProcSelectObject.Call(mem, bmp)
	if old == 0 || old == hgdiError {
		return nil, errors.New("SelectObject 실패")
	}
	r, _, e := win.ProcBitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), screen, uintptr(x), uintptr(y), srcCopy|captureBlt)
	win.ProcSelectObject.Call(mem, old) // GetDIBits 전에 비트맵을 DC에서 빼야 한다
	if r == 0 {
		return nil, fmt.Errorf("BitBlt: %w", e)
	}
	bi := &bitmapInfoHeader{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32} // 음수 높이 = 위에서 아래 순서
	bi.Size = uint32(unsafe.Sizeof(*bi))
	buf := make([]byte, w*h*4)
	if r, _, e := win.ProcGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(bi)), 0); r == 0 {
		return nil, fmt.Errorf("GetDIBits: %w", e)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(buf); i += 4 { // BGRA → RGBA
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = buf[i+2], buf[i+1], buf[i], 255
	}
	return img, nil
}
