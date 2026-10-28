//go:build integration

// 테스트용 한국어 IME 조작. 통합 테스트가 실제 가상 키(VK_PACKET 아님)를 IME에 거쳐 한글로 조합하게 한다. 앱 코드에서는 쓰지 않는다.
package win

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	imm32 = windows.NewLazySystemDLL("imm32.dll")

	ProcSendMessageW           = user32.NewProc("SendMessageW")
	ProcGetFocus               = user32.NewProc("GetFocus")
	ProcLoadKeyboardLayoutW    = user32.NewProc("LoadKeyboardLayoutW")
	ProcImmGetContext          = imm32.NewProc("ImmGetContext")
	ProcImmReleaseContext      = imm32.NewProc("ImmReleaseContext")
	ProcImmSetOpenStatus       = imm32.NewProc("ImmSetOpenStatus")
	ProcImmGetConversionStatus = imm32.NewProc("ImmGetConversionStatus")
	ProcImmSetConversionStatus = imm32.NewProc("ImmSetConversionStatus")
)

const (
	klfActivate    = 0x00000001
	langKorean     = 0x0412
	imeCmodeHangul = 0x0001 // IME_CMODE_HANGUL(= IME_CMODE_NATIVE)
)

// ErrNoKoreanIME는 이 PC에 한국어 자판·IME가 없다는 뜻이다. 테스트는 이때만 건너뛴다(그 밖의 실패는 테스트 실패다).
var ErrNoKoreanIME = errors.New("한국어 자판·IME가 없다")

// HangulOn은 이 스레드의 자판을 한국어로 바꾸고 포커스 창의 입력 컨텍스트를 한글 모드로 켠다. 창 스레드에서 부른다
// (Window.Do): IMM은 다른 스레드의 입력 컨텍스트를 바꾸지 못한다. 되돌릴 이전 변환 상태를 돌려준다.
func HangulOn() (prevConv uint32, err error) {
	hkl, _, _ := ProcLoadKeyboardLayoutW.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("00000412"))), klfActivate)
	if hkl&0xFFFF != langKorean {
		return 0, fmt.Errorf("%w: 00000412를 켜지 못했다(HKL %#x)", ErrNoKoreanIME, hkl)
	}
	h, _, _ := ProcGetFocus.Call()
	if h == 0 {
		return 0, errors.New("이 창 스레드에 포커스가 없다(테스트 창이 앞으로 오지 못했다)")
	}
	himc, _, _ := ProcImmGetContext.Call(h)
	if himc == 0 {
		return 0, fmt.Errorf("%w: 포커스 창 %#x에 입력 컨텍스트가 없다", ErrNoKoreanIME, h)
	}
	defer ProcImmReleaseContext.Call(h, himc)
	conv, sent := new(uint32), new(uint32)
	ProcImmGetConversionStatus.Call(himc, uintptr(unsafe.Pointer(conv)), uintptr(unsafe.Pointer(sent)))
	prevConv = *conv
	ProcImmSetOpenStatus.Call(himc, 1)
	ProcImmSetConversionStatus.Call(himc, uintptr(*conv|imeCmodeHangul), uintptr(*sent))
	ProcImmGetConversionStatus.Call(himc, uintptr(unsafe.Pointer(conv)), uintptr(unsafe.Pointer(sent)))
	if *conv&imeCmodeHangul == 0 {
		return prevConv, fmt.Errorf("한글 모드가 켜지지 않았다(변환 상태 %#x)", *conv)
	}
	return prevConv, nil
}

// SetConversion은 포커스 창의 입력 컨텍스트 변환 상태를 conv로 되돌린다(HangulOn 뒤 정리용). 창 스레드에서 부른다.
func SetConversion(conv uint32) {
	h, _, _ := ProcGetFocus.Call()
	himc, _, _ := ProcImmGetContext.Call(h)
	if himc == 0 {
		return
	}
	defer ProcImmReleaseContext.Call(h, himc)
	conv0, sent := new(uint32), new(uint32)
	ProcImmGetConversionStatus.Call(himc, uintptr(unsafe.Pointer(conv0)), uintptr(unsafe.Pointer(sent)))
	ProcImmSetConversionStatus.Call(himc, uintptr(conv), uintptr(*sent))
}
