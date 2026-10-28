//go:build integration

// Win32 버튼·입력란을 띄우고 UIA로 이름, 종류, 값, 비밀번호 여부를 읽는다.
package uia_test

import (
	"image"
	"os"
	"testing"
	"time"

	"howtonote/internal/uia"
	"howtonote/internal/win"
)

func TestMain(m *testing.M) {
	_ = win.SetPerMonitorV2()
	os.Exit(m.Run())
}

func TestElementAt_Win32Controls(t *testing.T) {
	const ox, oy = 200, 200
	w, err := win.OpenWindow(ox, oy, 400, 200, 0x00FFFFFF, []win.Child{
		{Class: "BUTTON", Text: "저장", X: 20, Y: 20, W: 120, H: 40},
		{Class: "EDIT", Text: "홍길동", X: 20, Y: 80, W: 200, H: 30},
		{Class: "EDIT", Text: "secret", Style: win.ES_PASSWORD, X: 20, Y: 130, W: 200, H: 30},
		{Class: "EDIT", Text: "읽기 전용", Style: win.ES_READONLY, X: 240, Y: 80, W: 140, H: 30},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(300 * time.Millisecond)

	s, err := uia.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	btn, err := s.ElementAt(ox+80, oy+40, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if btn.Name != "저장" || btn.ControlType != uia.ButtonControlType {
		t.Errorf("버튼 = %+v", btn)
	}

	ed, err := s.ElementAt(ox+120, oy+95, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Win32 입력란은 Value 패턴이 있다(HasValue). 30043을 잘못 읽으면 읽기 전용 판정(R22)이 뒤집힌다
	if ed.Value != "홍길동" || ed.ControlType != uia.EditControlType || ed.IsPassword || ed.ReadOnly || !ed.HasValue {
		t.Errorf("입력란 = %+v", ed)
	}

	ro, err := s.ElementAt(ox+310, oy+95, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if ro.ControlType != uia.EditControlType || !ro.ReadOnly || !ro.HasValue {
		t.Errorf("읽기 전용 입력란 = %+v, ReadOnly여야 한다", ro)
	}

	pw, err := s.ElementAt(ox+120, oy+145, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !pw.IsPassword {
		t.Errorf("비밀번호 입력란인데 IsPassword=false: %+v", pw)
	}
	if pw.Value != "" {
		t.Errorf("비밀번호 입력란의 Value는 읽지 않아야 하는데 값이 있음: %+v", pw)
	}
}

// 녹화기의 입력 세션은 Focused()로 입력란을 읽는다. M0에서 시험하지 않은 slot 8(GetFocusedElement)을 여기서 고정한다.
func TestFocused_ReturnsClickedEdit(t *testing.T) {
	const ox, oy = 240, 240
	w, err := win.OpenWindow(ox, oy, 360, 140, 0x00FFFFFF, []win.Child{
		{Class: "STATIC", Text: "이름", X: 20, Y: 24, W: 60, H: 24},
		{Class: "EDIT", Text: "홍길동", X: 90, Y: 20, W: 200, H: 30},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(300 * time.Millisecond)
	if err := win.Click(ox+190, oy+35); err != nil { // 클릭해야 창이 앞으로 오고 입력란이 포커스를 받는다
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	s, err := uia.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	el, err := s.Focused(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if el.ControlType != uia.EditControlType || el.Value != "홍길동" {
		t.Fatalf("포커스 요소 = %+v, 입력란이어야 한다", el)
	}
	// Win32 UIA 프록시는 이름 없는 EDIT의 이름을 바로 앞에 만든 STATIC 글자에서 가져온다. 녹화 제목 '이름' 입력란이 여기에 기댄다.
	if el.Name != "이름" {
		t.Errorf("입력란 이름 = %q, 바로 앞 STATIC의 '이름'이어야 한다", el.Name)
	}
	if !image.Pt(ox+190, oy+35).In(el.Rect) {
		t.Errorf("Rect %v에 클릭 지점이 없다", el.Rect)
	}
}

func TestElementAt_CheckBoxToggleState(t *testing.T) {
	const ox, oy, bsAutoCheckBox = 240, 240, 0x0003
	w, err := win.OpenWindow(ox, oy, 300, 80, 0x00FFFFFF, []win.Child{
		{Class: "BUTTON", Text: "자동 저장", Style: bsAutoCheckBox, X: 20, Y: 20, W: 200, H: 30},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(300 * time.Millisecond)
	s, err := uia.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	before, err := s.ElementAt(ox+60, oy+35, time.Second)
	if err != nil || before.ControlType != uia.CheckBoxControlType || before.Name != "자동 저장" || before.ToggleState != 0 {
		t.Fatalf("누르기 전 = %+v, 오류 %v", before, err)
	}
	if err := win.Click(ox+60, oy+35); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	after, err := s.ElementAt(ox+60, oy+35, time.Second)
	if err != nil || after.ToggleState != 1 {
		t.Errorf("누른 뒤 = %+v, 오류 %v, ToggleState 1이어야 한다", after, err)
	}
}

// M1-carry-1: IUIAutomation2로 연결·트랜잭션 시간 제한을 호출자 예산(300ms)만큼 줄였는지 되읽어 확인한다.
// 대상 OS는 Win10/11뿐이고 CUIAutomation8은 그 위에서 항상 IUIAutomation2를 지원하므로, ok=false는
// R16(CUIAutomation8 사용)이 되돌려졌다는 신호다 - 조용히 건너뛰지 않고 실패로 잡는다.
func TestService_UIA2Timeouts(t *testing.T) {
	s, err := uia.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	conn, txn, ok := uia.UIA2TimeoutsForTest(s)
	if !ok {
		t.Fatal("이 OS에 IUIAutomation2가 없다 - CUIAutomation8을 쓰는지 확인")
	}
	if conn != 300 || txn != 300 {
		t.Errorf("되읽은 시간 제한 = conn %d, txn %d, 둘 다 300이어야 한다", conn, txn)
	}
}

// M1-carry-5: Close는 여러 번 불러도 안전하고, 닫은 뒤의 요청은 ErrTimeout이 아니라 ErrClosed를 바로 돌려준다.
func TestClose_IdempotentAndErrClosedAfter(t *testing.T) {
	s, err := uia.Start()
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s.Close() // 두 번째 Close는 패닉하면 안 된다
	if _, err := s.ElementAt(0, 0, time.Second); err != uia.ErrClosed {
		t.Errorf("닫힌 뒤 ElementAt 오류 = %v, ErrClosed여야 한다", err)
	}
}
