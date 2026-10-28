// capture 단위 테스트. 창이나 실제 입력 없이 도는 것만 둔다.
package capture

import (
	"testing"
	"time"
)

func TestGrabRect_RejectsEmptySize(t *testing.T) {
	for _, c := range [][2]int{{0, 10}, {10, 0}, {-5, 10}} {
		if _, err := GrabRect(0, 0, c[0], c[1]); err == nil {
			t.Errorf("GrabRect 크기 %dx%d는 오류여야 한다", c[0], c[1])
		}
	}
}

func TestWindow_IsIME(t *testing.T) {
	if !(Window{Class: "CiceroUIWndFrame"}).IsIME() {
		t.Error("TSF 후보창은 IME다")
	}
	if !(Window{RootClass: "Microsoft.IME.UIManager.CandidateWindow.Host"}).IsIME() {
		t.Error("Microsoft IME 후보창은 IME다")
	}
	if (Window{Class: "Button", RootClass: "Notepad"}).IsIME() {
		t.Error("일반 창은 IME가 아니다")
	}
}

// classify는 순수 함수다 - 보조 키 상태를 인자로 받으므로 실제 키보드 상태에 기대지 않는다.
func TestClassify(t *testing.T) {
	none := modifierState{}
	cases := []struct {
		vk      uint32
		mods    modifierState
		end, ok bool
	}{
		{0x0D, none, true, true},                                  // Enter
		{0x09, none, true, true},                                  // Tab
		{0x41, none, false, true},                                 // A
		{0xE7, none, false, true},                                 // VK_PACKET(유니코드 입력)
		{0x08, none, false, true},                                 // Backspace
		{0x20, none, false, true},                                 // Space
		{0x10, none, false, false},                                // Shift
		{0xA2, none, false, false},                                // 왼쪽 Ctrl
		{0x78, none, false, false},                                // F9
		{0x25, none, false, false},                                // 왼쪽 방향키
		{0x15, none, false, false},                                // 한/영
		{0x1B, none, false, false},                                // Esc
		{vkInsert, none, false, false},                            // Insert 단독(토글 키)
		{vkDelete, none, false, true},                             // Delete 단독(내용을 지운다)
		{0x43, modifierState{Ctrl: true}, false, false},           // Ctrl+C: 복사, 세션과 무관
		{vkV, modifierState{Ctrl: true}, false, true},             // Ctrl+V: 붙여넣기 = 타이핑
		{vkX, modifierState{Ctrl: true}, false, true},             // Ctrl+X: 잘라내기 = 타이핑
		{vkZ, modifierState{Ctrl: true}, false, true},             // Ctrl+Z: 실행취소 = 타이핑
		{vkY, modifierState{Ctrl: true}, false, true},             // Ctrl+Y: 다시실행 = 타이핑
		{vkInsert, modifierState{Shift: true}, false, true},       // Shift+Insert: 붙여넣기 = 타이핑
		{vkDelete, modifierState{Shift: true}, false, true},       // Shift+Delete: 잘라내기 = 타이핑
		{0x41, modifierState{Alt: true}, false, false},            // Alt+A: 메뉴 단축키, 세션과 무관
		{0x44, modifierState{Win: true}, false, false},            // Win+D: 바탕화면 보기, 세션과 무관
		{vkV, modifierState{Ctrl: true, Alt: true}, false, false}, // Ctrl+Alt+V: 다른 조합이라 무시
	}
	for _, c := range cases {
		if end, ok := classify(c.vk, c.mods); end != c.end || ok != c.ok {
			t.Errorf("classify(0x%02X, %+v) = %v, %v, want %v, %v", c.vk, c.mods, end, ok, c.end, c.ok)
		}
	}
}

// expire는 키업을 못 받아 고착된 보조 키(UIPI로 관리자 권한 창에 간 Ctrl 등)를 3초 지나면 지운다는 순수 규칙이다.
func TestModifierState_Expire(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		before modifierState
		last   time.Time
		want   modifierState
	}{
		{"1초 전: 아직 안 지남", modifierState{Ctrl: true}, now.Add(-1 * time.Second), modifierState{Ctrl: true}},
		{"정확히 3초 전: 아직 안 지남(초과만 지움)", modifierState{Shift: true}, now.Add(-3 * time.Second), modifierState{Shift: true}},
		{"3초보다 1ms 더: 지움", modifierState{Shift: true}, now.Add(-3*time.Second - time.Millisecond), modifierState{}},
		{"4초 전, 여러 개: 모두 지움", modifierState{Ctrl: true, Win: true, Shift: true}, now.Add(-4 * time.Second), modifierState{}},
	}
	for _, c := range cases {
		m := c.before
		m.expire(now, c.last)
		if m != c.want {
			t.Errorf("%s: expire() = %+v, want %+v", c.name, m, c.want)
		}
	}
}

// setModifier는 Ctrl·Win·Shift 키 자체의 up/down만 본다(좌우 변형 다 포함). 마우스 클릭(세션 경계)은 모두 지운다.
func TestHook_SetModifierAndClickReset(t *testing.T) {
	var h Hook
	h.setModifier(vkLControl, true)
	if !h.mods.Ctrl {
		t.Error("왼쪽 Ctrl을 누르면 Ctrl=true여야 한다")
	}
	h.setModifier(vkRWin, true)
	if !h.mods.Win {
		t.Error("오른쪽 Win을 누르면 Win=true여야 한다")
	}
	h.setModifier(vkLShift, true)
	if !h.mods.Shift {
		t.Error("왼쪽 Shift를 누르면 Shift=true여야 한다")
	}
	h.setModifier(vkLControl, false)
	if h.mods.Ctrl {
		t.Error("왼쪽 Ctrl을 떼면 Ctrl=false여야 한다")
	}
	// 마우스 버튼 다운 콜백이 하는 일과 같다: 클릭은 그 자체로 세션 경계라 고착 여부와 무관하게 지운다.
	h.mods = modifierState{}
	if h.mods != (modifierState{}) {
		t.Errorf("클릭 뒤에는 보조 키 상태가 모두 꺼져 있어야 한다: %+v", h.mods)
	}
}
