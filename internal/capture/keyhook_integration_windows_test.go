//go:build integration

// 키보드 훅(입력 세션 감지)과 전역 단축키를 SendInput으로 확인한다. 키 값이 아니라 종류(End)만 오는지 본다.
// 키는 다른 앱에 들어가지 않게 테스트 창을 클릭해 앞으로 가져온 뒤에만 보낸다.
package capture_test

import (
	"testing"
	"time"

	"howtonote/internal/capture"
	"howtonote/internal/win"
)

const (
	vkTab     = 0x09
	vkShift   = 0x10
	vkControl = 0x11
	vkF9      = 0x78
)

func TestKeyboardHook_ReportsKindOnly(t *testing.T) {
	w := openTarget(t)
	defer w.Close()
	h, err := capture.StartHook(capture.HookOptions{AllowInjected: true, Keyboard: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	if err := win.Click(500, 460); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := win.TypeText("홍길"); err != nil {
		t.Fatal(err)
	}
	if err := win.PressKeys(vkTab); err != nil {
		t.Fatal(err)
	}
	for i, wantEnd := range []bool{false, false, true} {
		select {
		case k := <-h.Keys():
			if k.End != wantEnd || !k.Injected {
				t.Errorf("키 %d = %+v, End=%v여야 한다", i, k, wantEnd)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("키 %d 이벤트가 오지 않았다", i)
		}
	}
	if d := h.MaxCallback(); d > 10*time.Millisecond {
		t.Errorf("훅 콜백 최대 %v, 10ms 이내여야 한다", d)
	}
}

// AllowInjected가 기본값(false)이면 SendInput으로 만든 키는 무시해야 한다(TestMouseHook_IgnoresInjectedByDefault와 짝).
func TestKeyboardHook_IgnoresInjectedByDefault(t *testing.T) {
	w := openTarget(t)
	defer w.Close()
	h, err := capture.StartHook(capture.HookOptions{Keyboard: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	if err := win.Click(500, 460); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := win.PressKeys(vkTab); err != nil {
		t.Fatal(err)
	}
	select {
	case k := <-h.Keys():
		t.Fatalf("주입된 키는 무시해야 한다: %+v", k)
	case <-time.After(500 * time.Millisecond):
	}
}

// 관리자 권한(UAC) 창으로 넘어가며 뗀 Ctrl의 키업은 우리 훅에 안 올 수 있다(UIPI) - 그러면 Ctrl이 고착되어
// 그 뒤 모든 키가 "단축키"로만 보인다. 여기서는 KeyUp을 일부러 보내지 않아 그 고착을 흉내 내고, 클릭(입력 세션
// 경계)이 그 고착을 지우는지 본다.
func TestKeyboardHook_ClickResetsStuckModifier(t *testing.T) {
	const vkA, vkB = 0x41, 0x42
	w := openTarget(t)
	defer w.Close()
	h, err := capture.StartHook(capture.HookOptions{AllowInjected: true, Keyboard: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	if err := win.Click(500, 460); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	// Ctrl을 누른 채로 두고 뗀 적이 없다고 해 둔다(KeyUp을 보내지 않는다).
	if err := win.KeyDown(vkControl); err != nil {
		t.Fatal(err)
	}
	defer win.KeyUp(vkControl) // 테스트가 실패해도 시스템에 Ctrl이 눌린 채로 남지 않게 한다
	if err := win.PressKeys(vkA); err != nil {
		t.Fatal(err)
	}
	select {
	case k := <-h.Keys():
		t.Fatalf("Ctrl이 눌린 채라 단축키로 보여 무시돼야 하는데 왔다: %+v", k)
	case <-time.After(300 * time.Millisecond):
	}

	// 클릭은 세션 경계이므로 고착된 Ctrl 상태를 지워야 한다.
	if err := win.Click(500, 460); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := win.PressKeys(vkB); err != nil {
		t.Fatal(err)
	}
	select {
	case k := <-h.Keys():
		if k.End || !k.Injected {
			t.Errorf("클릭으로 고착을 지운 뒤 평범한 키 = %+v, End=false Injected=true여야 한다", k)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("클릭 뒤 키 이벤트가 오지 않았다 - 고착된 Ctrl이 안 지워진 것 같다")
	}
}

// Stop() 뒤에는 같은 단축키를 다시 걸 수 있어야 한다(UnregisterHotKey가 실제로 끝났다는 뜻).
func TestHook_HotkeyReusableAfterStop(t *testing.T) {
	w := openTarget(t)
	defer w.Close()
	hotkey := capture.Hotkey{ID: 7, Mods: capture.ModControl | capture.ModShift, VK: vkF9}
	h1, err := capture.StartHook(capture.HookOptions{Hotkeys: []capture.Hotkey{hotkey}})
	if err != nil {
		t.Fatal(err)
	}
	if len(h1.FailedHotkeys()) > 0 {
		h1.Stop()
		t.Skip("다른 프로그램이 Ctrl+Shift+F9를 쓰고 있다")
	}
	h1.Stop()
	h2, err := capture.StartHook(capture.HookOptions{Hotkeys: []capture.Hotkey{hotkey}})
	if err != nil {
		t.Fatal(err)
	}
	defer h2.Stop()
	if failed := h2.FailedHotkeys(); len(failed) > 0 {
		t.Errorf("Stop() 뒤 같은 단축키를 다시 걸지 못했다: %v", failed)
	}
}

func TestHook_DeliversHotkey(t *testing.T) {
	w := openTarget(t)
	defer w.Close()
	h, err := capture.StartHook(capture.HookOptions{Hotkeys: []capture.Hotkey{
		{ID: 7, Mods: capture.ModControl | capture.ModShift, VK: vkF9},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	if len(h.FailedHotkeys()) > 0 {
		t.Skip("다른 프로그램이 Ctrl+Shift+F9를 쓰고 있다")
	}
	if err := win.Click(500, 460); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := win.PressKeys(vkControl, vkShift, vkF9); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-h.Hotkeys():
		if id != 7 {
			t.Errorf("단축키 ID = %d, 7이어야 한다", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("단축키 이벤트가 오지 않았다")
	}
}
