//go:build integration

// 테스트용 입력 주입(SendInput). 통합 테스트가 사람처럼 클릭하고 글자를 친다. 앱 코드에서는 쓰지 않는다.
package win

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// mouseInput과 keyInput은 INPUT 구조체(x64 40바이트)다. union이 8바이트 정렬이라 Type 뒤에 4바이트가 빈다.
type mouseInput struct {
	Type      uint32
	_         uint32
	Dx, Dy    int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type keyInput struct {
	Type      uint32
	_         uint32
	VK        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
	_         [8]byte // union에서 가장 큰 MOUSEINPUT 크기에 맞춘다
}

const (
	inputMouse       = 0
	inputKeyboard    = 1
	mouseLeftDown    = 0x0002
	mouseLeftUp      = 0x0004
	mouseRightDown   = 0x0008
	mouseRightUp     = 0x0010
	keyeventfKeyUp   = 0x0002
	keyeventfUnicode = 0x0004
)

func send[T mouseInput | keyInput](in []T) error {
	r, _, e := ProcSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	if int(r) != len(in) {
		return fmt.Errorf("SendInput: %w", e)
	}
	return nil
}

// Click은 커서를 (x, y)로 옮기고 왼쪽 버튼을 눌렀다 뗀다.
func Click(x, y int32) error {
	ProcSetCursorPos.Call(uintptr(x), uintptr(y))
	return send([]mouseInput{{Type: inputMouse, Flags: mouseLeftDown}, {Type: inputMouse, Flags: mouseLeftUp}})
}

// RightClick은 커서를 (x, y)로 옮기고 오른쪽 버튼을 눌렀다 뗀다.
func RightClick(x, y int32) error {
	ProcSetCursorPos.Call(uintptr(x), uintptr(y))
	return send([]mouseInput{{Type: inputMouse, Flags: mouseRightDown}, {Type: inputMouse, Flags: mouseRightUp}})
}

// MouseDown은 커서를 (x, y)로 옮기고 왼쪽 버튼을 누른 채로 둔다(떼려면 MouseUp). BS_AUTOCHECKBOX처럼
// 뗄 때(WM_LBUTTONUP) 상태가 바뀌는 컨트롤에서 "누른 순간"과 "상태가 실제로 바뀐 순간"을 떼어 보고 싶을 때 쓴다 -
// Click은 누르고 바로 떼므로 이 용도로는 못 쓴다.
func MouseDown(x, y int32) error {
	ProcSetCursorPos.Call(uintptr(x), uintptr(y))
	return send([]mouseInput{{Type: inputMouse, Flags: mouseLeftDown}})
}

// MouseUp은 MouseDown으로 누른 왼쪽 버튼을 뗀다.
func MouseUp(x, y int32) error {
	ProcSetCursorPos.Call(uintptr(x), uintptr(y))
	return send([]mouseInput{{Type: inputMouse, Flags: mouseLeftUp}})
}

// TypeText는 글자를 유니코드 입력(VK_PACKET)으로 친다. IME를 거치지 않아 한글도 그대로 들어간다.
func TypeText(s string) error {
	var in []keyInput
	for _, r := range s {
		in = append(in,
			keyInput{Type: inputKeyboard, Scan: uint16(r), Flags: keyeventfUnicode},
			keyInput{Type: inputKeyboard, Scan: uint16(r), Flags: keyeventfUnicode | keyeventfKeyUp})
	}
	return send(in)
}

// PressKeys는 가상 키를 차례로 누른 뒤 거꾸로 뗀다(예: Ctrl, Shift, F9 → 단축키).
func PressKeys(vks ...uint16) error {
	var in []keyInput
	for _, vk := range vks {
		in = append(in, keyInput{Type: inputKeyboard, VK: vk})
	}
	for i := len(vks) - 1; i >= 0; i-- {
		in = append(in, keyInput{Type: inputKeyboard, VK: vks[i], Flags: keyeventfKeyUp})
	}
	return send(in)
}

// KeyDown은 가상 키 하나를 누른 채로 둔다(떼려면 KeyUp). 키 업이 안 오는 상황(예: 관리자 권한 창으로 넘어가며
// 뗀 Ctrl)을 테스트로 흉내 낼 때 쓴다 - PressKeys는 항상 누르고 바로 떼므로 이 용도로는 못 쓴다.
func KeyDown(vk uint16) error {
	return send([]keyInput{{Type: inputKeyboard, VK: vk}})
}

// KeyUp은 KeyDown으로 누른 키를 뗀다.
func KeyUp(vk uint16) error {
	return send([]keyInput{{Type: inputKeyboard, VK: vk, Flags: keyeventfKeyUp}})
}

// FocusAt은 클릭도 키도 없이 (x, y) 아래 컨트롤로 키보드 포커스를 옮긴다. 자동 넘김·Ctrl+L처럼 입력 세션을 끝내는
// 신호 없이 포커스만 옮겨 가는 경우를 흉내 낸다. SetFocus는 같은 스레드 입력 상태의 창에만 되므로 그 창 스레드에 잠깐 붙는다.
func FocusAt(x, y int32) error {
	h, _, _ := ProcWindowFromPoint.Call(PointArg(x, y))
	if h == 0 {
		return errors.New("FocusAt: 그 지점에 창이 없다")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	me := windows.GetCurrentThreadId()
	target, err := windows.GetWindowThreadProcessId(windows.HWND(h), nil)
	if err != nil {
		return err
	}
	if r, _, e := ProcAttachThreadInput.Call(uintptr(me), uintptr(target), 1); r == 0 {
		return fmt.Errorf("AttachThreadInput: %w", e)
	}
	defer ProcAttachThreadInput.Call(uintptr(me), uintptr(target), 0)
	ProcSetFocus.Call(h)
	return nil
}
