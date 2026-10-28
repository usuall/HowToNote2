//go:build integration

// 녹화기 통합 테스트: Win32 테스트 창을 SendInput으로 조작하고 저장된 스텝(종류, 제목, 이미지, 클릭 표시)을 본다.
// 실제 마우스와 키보드를 쓰므로 실행 중에는 손대지 않는다.
package recorder_test

import (
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"howtonote/internal/capture"
	"howtonote/internal/recorder"
	"howtonote/internal/store"
	"howtonote/internal/uia"
	"howtonote/internal/win"
)

func TestMain(m *testing.M) {
	_ = win.SetPerMonitorV2()
	os.Exit(m.Run())
}

const (
	ox, oy         = 300, 300           // 테스트 창 왼쪽 위(화면 물리 픽셀)
	editX, editY   = ox + 190, oy + 35  // 입력란 가운데
	saveX, saveY   = ox + 80, oy + 100  // '저장' 버튼 가운데
	checkX, checkY = ox + 30, oy + 39   // openCheckboxForm·openCheckAndButtonForm의 체크박스 가운데
	edit2X, edit2Y = ox + 190, oy + 155 // openTwoFieldForm의 두 번째 입력란 가운데
)

// bsAutoCheckbox는 Win32 BUTTON 컨트롤의 BS_AUTOCHECKBOX 스타일이다. 클릭하면 OS가 알아서 체크 상태를 토글한다.
const bsAutoCheckbox = 0x0003

// newTestForm은 420x200 테스트 창을 자식 컨트롤과 함께 띄운다. openForm·openCheckboxForm·
// openCheckAndButtonForm이 공유한다.
func newTestForm(t *testing.T, children []win.Child) *win.Window {
	t.Helper()
	w, err := win.OpenWindow(ox, oy, 420, 200, 0x00FFFFFF, children, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	time.Sleep(300 * time.Millisecond)
	return w
}

// openForm은 라벨이 붙은 입력란 하나와 '저장' 버튼이 있는 테스트 창을 띄운다.
// Win32 UIA 프록시는 EDIT의 이름을 바로 앞에 만든 STATIC 글자에서 가져온다(uia 테스트 TestFocused_ReturnsClickedEdit).
func openForm(t *testing.T, label string, editStyle uint32) *win.Window {
	t.Helper()
	return newTestForm(t, []win.Child{
		{Class: "STATIC", Text: label, X: 20, Y: 24, W: 66, H: 24},
		{Class: "EDIT", Style: editStyle, X: 90, Y: 20, W: 200, H: 30},
		{Class: "BUTTON", Text: "저장", X: 20, Y: 80, W: 120, H: 40},
	})
}

// openTwoFieldForm은 openForm에 라벨이 붙은 두 번째 입력란(style2)을 더한 창이다. 첫 입력란과 '저장'은 openForm과 같은 자리다.
// 두 번째 입력란의 이름도 바로 앞에 만든 STATIC(label2)에서 온다.
func openTwoFieldForm(t *testing.T, label2 string, style2 uint32) *win.Window {
	t.Helper()
	return newTestForm(t, []win.Child{
		{Class: "STATIC", Text: "이름", X: 20, Y: 24, W: 66, H: 24},
		{Class: "EDIT", X: 90, Y: 20, W: 200, H: 30},
		{Class: "BUTTON", Text: "저장", X: 20, Y: 80, W: 120, H: 40},
		{Class: "STATIC", Text: label2, X: 20, Y: 144, W: 66, H: 24},
		{Class: "EDIT", Style: style2, X: 90, Y: 140, W: 200, H: 30},
	})
}

// openCheckboxForm은 체크박스 하나만 있는 테스트 창을 띄운다(C1: 클릭 후 상태를 따라야 하는 제목 검증용).
func openCheckboxForm(t *testing.T, label string) *win.Window {
	t.Helper()
	return newTestForm(t, []win.Child{
		{Class: "BUTTON", Text: label, Style: bsAutoCheckbox, X: 20, Y: 24, W: 160, H: 30},
	})
}

// openCheckAndButtonForm은 체크박스와 '저장' 버튼이 같이 있는 테스트 창이다. 체크박스 클릭이 다음 클릭의
// 촬영(stage 1)을 늦추지 않는지 보는 테스트 전용이다.
func openCheckAndButtonForm(t *testing.T, label string) *win.Window {
	t.Helper()
	return newTestForm(t, []win.Child{
		{Class: "BUTTON", Text: label, Style: bsAutoCheckbox, X: 20, Y: 24, W: 160, H: 30},
		{Class: "BUTTON", Text: "저장", X: 20, Y: 80, W: 120, H: 40},
	})
}

type rig struct {
	st     *store.Store
	ua     *uia.Service
	events chan recorder.Event
}

func newRig(t *testing.T) *rig {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ua, err := uia.Start()
	if err != nil {
		st.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ua.Close()
		st.Close()
	})
	return &rig{st: st, ua: ua, events: make(chan recorder.Event, 64)}
}

func (g *rig) start(t *testing.T, opt recorder.Options) *recorder.Recorder {
	t.Helper()
	opt.AllowInjected = true
	opt.OnEvent = func(e recorder.Event) { g.events <- e }
	r, err := recorder.Start(g.st, g.ua, opt)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// next는 typ 이벤트가 올 때까지 기다린다. 사이에 온 경고는 로그로 남긴다.
func (g *rig) next(t *testing.T, typ string) recorder.Event {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case e := <-g.events:
			if e.Type == typ {
				return e
			}
			if e.Type == "warning" {
				t.Logf("경고: %s", e.Message)
			}
		case <-timeout:
			t.Fatalf("%s 이벤트가 오지 않았다", typ)
		}
	}
}

// noStep은 d 동안 스텝이 생기지 않는지 본다.
func (g *rig) noStep(t *testing.T, d time.Duration) {
	t.Helper()
	timeout := time.After(d)
	for {
		select {
		case e := <-g.events:
			if e.Type == "step" {
				t.Fatalf("스텝이 생기면 안 된다: %+v", e)
			}
		case <-timeout:
			return
		}
	}
}

func click(t *testing.T, x, y int32) {
	t.Helper()
	if err := win.Click(x, y); err != nil {
		t.Fatal(err)
	}
}

// clickHeld는 (x, y)에서 왼쪽 버튼을 누른 채 hold만큼 있다가 뗀다. BS_AUTOCHECKBOX는 뗄 때(WM_LBUTTONUP)
// 상태가 바뀌므로, "누른 순간"(훅이 찍는 시각)과 "상태가 실제로 바뀐 순간"이 다르다는 것을 실제로 만들어
// C1(누른 뒤 상태를 읽어야 한다)을 의미 있게 검증한다.
func clickHeld(t *testing.T, x, y int32, hold time.Duration) {
	t.Helper()
	if err := win.MouseDown(x, y); err != nil {
		t.Fatal(err)
	}
	time.Sleep(hold)
	if err := win.MouseUp(x, y); err != nil {
		t.Fatal(err)
	}
}

func TestRecorder_ClickButtonMakesStep(t *testing.T) {
	g := newRig(t)
	openForm(t, "이름", 0)
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop() // 중간에 실패해도 훅을 뗀다. Stop은 여러 번 불러도 된다
	click(t, saveX, saveY)
	e := g.next(t, "step")
	if e.Title != "'저장' 버튼을 클릭합니다." || e.Steps != 1 || e.ImageSHA == "" {
		t.Errorf("스텝 이벤트 = %+v", e)
	}
	// M1-carry-6: 클릭(Click.At)부터 GrabRect가 돌아올 때까지 걸린 시간. 500ms 목표 예산의 일부다.
	t.Logf("클릭 → 화면 찍기 지연: %dms (전체 저장 지연: %dms)", e.GrabMs, e.LatencyMs)
	gid := r.Stop()
	steps, err := g.st.ListSteps(gid)
	if err != nil || len(steps) != 1 {
		t.Fatalf("스텝 %d개, 오류 %v", len(steps), err)
	}
	s := steps[0]
	if s.Kind != "click" || s.Title != e.Title || s.ImageSHA != e.ImageSHA {
		t.Errorf("저장된 스텝 = %+v", s)
	}

	// 활성 창 모드: 이미지는 창 크기이고, 클릭 표시의 가운데는 이미지 좌표 (80, 100)이다.
	path, _ := g.st.ImageFile(s.ImageSHA)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != 420 || cfg.Height != 200 {
		t.Errorf("이미지 %dx%d(오류 %v), 420x200이어야 한다", cfg.Width, cfg.Height, err)
	}
	var marks []struct {
		Type       string
		X, Y, W, H int
	}
	if err := json.Unmarshal([]byte(s.AnnotationsJSON), &marks); err != nil || len(marks) != 1 ||
		marks[0].Type != "marker" || marks[0].X+marks[0].W/2 != 80 || marks[0].Y+marks[0].H/2 != 100 {
		t.Errorf("클릭 표시 = %s", s.AnnotationsJSON)
	}
	var info struct {
		Point       struct{ X, Y int }
		WindowTitle string `json:"window_title"`
		Element     struct {
			Name        string
			ControlType int32 `json:"control_type"`
		}
	}
	if err := json.Unmarshal([]byte(s.CaptureJSON), &info); err != nil || info.Point.X != 80 || info.Point.Y != 100 ||
		info.WindowTitle != "HowtoNote 테스트" || info.Element.Name != "저장" || info.Element.ControlType != uia.ButtonControlType {
		t.Errorf("capture_json = %s", s.CaptureJSON)
	}
}

func TestRecorder_IgnoresOwnWindowAndOutsideRegion(t *testing.T) {
	g := newRig(t)
	openForm(t, "이름", 0)
	own, err := win.OpenWindow(800, 300, 200, 100, 0x00EEEEEE, nil, nil) // 앱 툴바 역할
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	other, err := win.OpenWindow(800, 450, 200, 100, 0x00DDDDDD, nil, nil) // 영역 밖 다른 앱 역할
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	time.Sleep(300 * time.Millisecond)

	region := image.Rect(ox, oy, ox+420, oy+200)
	r := g.start(t, recorder.Options{Mode: recorder.ModeRegion, Region: region, IgnoreHWND: own.HWND})
	defer r.Stop()
	click(t, 900, 350) // 우리 창
	click(t, 900, 500) // 영역 밖
	g.noStep(t, 700*time.Millisecond)
	click(t, saveX, saveY)
	if e := g.next(t, "step"); e.Title != "'저장' 버튼을 클릭합니다." {
		t.Errorf("스텝 = %+v", e)
	}
}

// TestRecorder_CheckBoxTitleReflectsStateAfterClick은 C1 규칙을 본다: 체크박스 제목은 누르기 전이 아니라
// 누른 뒤의 실제 상태를 따라야 한다. clickHeld로 누르고 150ms 뒤에 떼서(BS_AUTOCHECKBOX는 뗄 때 토글된다)
// "누른 순간"(At)과 "실제로 바뀐 순간"이 다르게 만든 뒤, 첫 클릭(해제→선택)과 두 번째 클릭(선택→해제) 모두
// 제목이 그 결과를 맞히는지 본다.
func TestRecorder_CheckBoxTitleReflectsStateAfterClick(t *testing.T) {
	g := newRig(t)
	openCheckboxForm(t, "동의")
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop()

	clickHeld(t, checkX, checkY, 150*time.Millisecond)
	e := g.next(t, "step")
	if e.Title != "'동의'를 선택합니다." {
		t.Errorf("체크 후 제목 = %q, want 선택합니다", e.Title)
	}

	clickHeld(t, checkX, checkY, 150*time.Millisecond)
	e2 := g.next(t, "step")
	if e2.Title != "'동의'를 해제합니다." {
		t.Errorf("해제 후 제목 = %q, want 해제합니다", e2.Title)
	}
}

// TestRecorder_CheckBoxRightClickUsesGenericTitle은 고침 3번 규칙을 본다: 오른쪽 클릭은 체크박스를 토글하지
// 않으므로 기다리지도, 선택/해제 문구를 쓰지도 않는다 - 일반 클릭 틀에 "마우스 오른쪽 버튼으로"가 붙는다.
func TestRecorder_CheckBoxRightClickUsesGenericTitle(t *testing.T) {
	g := newRig(t)
	openCheckboxForm(t, "동의")
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop()

	if err := win.RightClick(checkX, checkY); err != nil {
		t.Fatal(err)
	}
	e := g.next(t, "step")
	if e.Title != "'동의'를 마우스 오른쪽 버튼으로 클릭합니다." {
		t.Errorf("오른쪽 클릭 제목 = %q, want 선택/해제가 아닌 일반 클릭 틀", e.Title)
	}
}

// TestRecorder_CheckBoxDoesNotDelayNextCapture는 고침 1번(C1 대기를 stage 1에서 stage 2로 옮김)을 본다:
// 체크박스를 클릭한 100ms 뒤 다른 컨트롤을 클릭해도, 두 클릭 모두 자기 클릭 시각(Click.At) 기준으로 금방
// 찍혀야 한다 - 체크박스의 300ms 상태 재확인 대기가 stage 1(촬영)에 남아 있다면 두 번째 클릭의 GrabMs가
// 커진다.
func TestRecorder_CheckBoxDoesNotDelayNextCapture(t *testing.T) {
	g := newRig(t)
	openCheckAndButtonForm(t, "동의")
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop()

	click(t, checkX, checkY) // 체크박스
	time.Sleep(100 * time.Millisecond)
	click(t, saveX, saveY) // 버튼 - 체크박스의 상태 재확인 대기 중에도 바로 찍혀야 한다

	e1 := g.next(t, "step")
	e2 := g.next(t, "step")
	t.Logf("체크박스 GrabMs=%d, 버튼 GrabMs=%d", e1.GrabMs, e2.GrabMs)
	const maxGrab = 100 // ms: stage 1은 체크박스의 300ms 상태 재확인과 무관하게 빨라야 한다
	if e1.GrabMs > maxGrab {
		t.Errorf("체크박스 클릭 GrabMs = %d, %dms 안이어야 한다", e1.GrabMs, maxGrab)
	}
	if e2.GrabMs > maxGrab {
		t.Errorf("버튼 클릭 GrabMs = %d, %dms 안이어야 한다(체크박스 대기에 묶이면 안 된다)", e2.GrabMs, maxGrab)
	}
}

const (
	vkTab     = 0x09
	vkReturn  = 0x0D
	vkSpace   = 0x20
	vkShift   = 0x10
	vkControl = 0x11
	vkF9      = 0x78
	vkF10     = 0x79
	vkF11     = 0x7A
)

func press(t *testing.T, vks ...uint16) {
	t.Helper()
	if err := win.PressKeys(vks...); err != nil {
		t.Fatal(err)
	}
}

func typeText(t *testing.T, s string) {
	t.Helper()
	if err := win.TypeText(s); err != nil {
		t.Fatal(err)
	}
}

// 클릭 → 한글 입력 → Tab → 버튼 클릭이 스텝 3개가 되는지 본다(스펙 10절 캡처 통합 테스트).
func TestRecorder_TypingSessionEndToEnd(t *testing.T) {
	g := newRig(t)
	openForm(t, "이름", 0)
	r := g.start(t, recorder.Options{Mode: recorder.ModeScreen})
	defer r.Stop() // 중간에 실패해도 훅을 뗀다. Stop은 여러 번 불러도 된다
	click(t, editX, editY)
	g.next(t, "step")
	typeText(t, "홍길동")
	time.Sleep(400 * time.Millisecond) // 사람이 다 치고 잠깐 멈춘 것처럼(미리 읽기 refreshDelay 250ms 뒤)
	press(t, vkTab)
	g.next(t, "step")
	click(t, saveX, saveY)
	e := g.next(t, "step")
	t.Logf("클릭부터 저장까지 %dms(화면 찍기 %dms)", e.LatencyMs, e.GrabMs) // 통과해도 실측값을 남긴다(결과 문서용)
	if e.LatencyMs > 500 {
		t.Errorf("클릭부터 저장까지 %dms, 500ms 이내여야 한다(스펙 4.1절)", e.LatencyMs)
	}
	gid := r.Stop()

	steps, err := g.st.ListSteps(gid)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ kind, title string }{
		{"click", "'이름' 입력란을 클릭합니다."},
		{"type", "'이름' 입력란에 '홍길동'을 입력합니다."},
		{"click", "'저장' 버튼을 클릭합니다."},
	}
	if len(steps) != len(want) {
		t.Fatalf("스텝 %d개: %+v", len(steps), steps)
	}
	for i, w := range want {
		if steps[i].Kind != w.kind || steps[i].Title != w.title {
			t.Errorf("스텝 %d = %s %q, want %s %q", i+1, steps[i].Kind, steps[i].Title, w.kind, w.title)
		}
	}
	// 입력 스텝의 표시는 입력란 가운데에 있다(이미지 좌표 = 화면 좌표 - 모니터 왼쪽 위).
	mon, _, err := capture.MonitorAt(editX, editY)
	if err != nil {
		t.Fatal(err)
	}
	var marks []struct{ X, Y, W, H int }
	if err := json.Unmarshal([]byte(steps[1].AnnotationsJSON), &marks); err != nil || len(marks) != 1 ||
		marks[0].X+marks[0].W/2 != editX-mon.Min.X || marks[0].Y+marks[0].H/2 != editY-mon.Min.Y {
		t.Errorf("입력 스텝 표시 = %s", steps[1].AnnotationsJSON)
	}
}

// TestRecorder_ClickEndsTypingSessionSharesShot은 컨트롤러 규칙 C3을 본다: 클릭이 입력 세션을 끝내면 클릭 순간 찍은
// 화면 하나를 입력 스텝과 클릭 스텝이 같이 쓰고, 입력 스텝이 먼저 저장된다.
// 같은 SHA는 "한 번 찍음"과 "똑같은 화면을 두 번 찍음"을 가르지 못한다. 한 번 찍는 것 자체는 onClick 코드가 보장하고, 여기서는 순서와 같은 이미지를 본다.
func TestRecorder_ClickEndsTypingSessionSharesShot(t *testing.T) {
	g := newRig(t)
	openForm(t, "이름", 0)
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop()
	click(t, editX, editY)
	g.next(t, "step")
	typeText(t, "홍길동")
	time.Sleep(400 * time.Millisecond)
	click(t, saveX, saveY) // Tab 없이 다른 곳 클릭으로 세션을 끝낸다
	typ := g.next(t, "step")
	clk := g.next(t, "step")
	t.Logf("클릭부터 클릭 스텝 저장까지 %dms(입력 스텝 저장 포함)", clk.LatencyMs)
	if typ.Title != "'이름' 입력란에 '홍길동'을 입력합니다." || clk.Title != "'저장' 버튼을 클릭합니다." {
		t.Errorf("스텝 순서·제목 = %q, %q", typ.Title, clk.Title)
	}
	if typ.ImageSHA != clk.ImageSHA {
		t.Errorf("입력 스텝 이미지 %s, 클릭 스텝 이미지 %s: 클릭 순간 화면 하나를 같이 써야 한다", typ.ImageSHA, clk.ImageSHA)
	}
}

func TestRecorder_PasswordIsMasked(t *testing.T) {
	g := newRig(t)
	openForm(t, "비밀번호", win.ES_PASSWORD)
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop() // 중간에 실패해도 훅을 뗀다(안 그러면 뒤 테스트가 모두 "이미 입력 훅이 실행 중"으로 실패한다)
	click(t, editX, editY)
	g.next(t, "step")
	typeText(t, "secret1")
	time.Sleep(400 * time.Millisecond)
	press(t, vkTab)
	if e := g.next(t, "step"); e.Title != "'비밀번호' 입력란에 '●●●●'를 입력합니다." {
		t.Errorf("입력 스텝 = %+v", e)
	}
	gid := r.Stop()
	steps, _ := g.st.ListSteps(gid)
	for _, s := range steps {
		if strings.Contains(s.Title+s.CaptureJSON+s.AnnotationsJSON, "secret1") {
			t.Errorf("비밀번호 값이 저장됐다: %+v", s)
		}
	}
}

func TestRecorder_HotkeysPauseManualStop(t *testing.T) {
	g := newRig(t)
	openForm(t, "이름", 0)
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop()
	select { // 등록 실패 경고는 녹화 시작 직후에 온다
	case e := <-g.events:
		if e.Type == "warning" {
			t.Skipf("단축키를 다른 프로그램이 쓰고 있다: %s", e.Message)
		}
		t.Fatalf("예상하지 못한 이벤트: %+v", e)
	case <-time.After(300 * time.Millisecond):
	}
	click(t, saveX, saveY) // 테스트 창을 앞으로. 수동 캡처 제목이 이 창 제목이 된다
	g.next(t, "step")
	press(t, vkControl, vkShift, vkF9)
	g.next(t, "paused")
	click(t, saveX, saveY)
	g.noStep(t, 700*time.Millisecond)
	press(t, vkControl, vkShift, vkF9)
	g.next(t, "resumed")
	press(t, vkControl, vkShift, vkF10)
	if e := g.next(t, "step"); e.Title != "HowtoNote 테스트 화면을 확인합니다." {
		t.Errorf("수동 캡처 = %+v", e)
	}
	press(t, vkControl, vkShift, vkF11)
	stopped := g.next(t, "stopped")
	if gid := r.Stop(); gid != stopped.GuideID || stopped.Steps != 2 {
		t.Errorf("끝낸 결과 = %+v, Stop() = %d", stopped, gid)
	}
	steps, _ := g.st.ListSteps(stopped.GuideID)
	if len(steps) != 2 || steps[0].Kind != "click" || steps[1].Kind != "manual" {
		t.Errorf("스텝 = %+v", steps)
	}
}

func TestRecorder_PauseResumeMethods(t *testing.T) {
	g := newRig(t)
	openForm(t, "이름", 0)
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop()
	r.Pause()
	g.next(t, "paused")
	click(t, saveX, saveY)
	g.noStep(t, 700*time.Millisecond)
	r.Resume()
	g.next(t, "resumed")
	click(t, saveX, saveY)
	g.next(t, "step")
}

// TestRecorder_ClickRightAfterTypingReadsFinalValue는 I1을 본다: 마지막 쉼(미리 읽기) 뒤에 친 글자도 값에 들어가야 한다.
// 클릭하면 포커스가 버튼으로 옮겨 가므로, 끝날 때는 입력란 자리의 요소를 다시 읽어야 한다.
func TestRecorder_ClickRightAfterTypingReadsFinalValue(t *testing.T) {
	g := newRig(t)
	openForm(t, "이름", 0)
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop()
	click(t, editX, editY)
	g.next(t, "step")
	typeText(t, "홍길")
	time.Sleep(400 * time.Millisecond) // 여기서 "홍길"이 미리 읽힌다
	typeText(t, "동")
	click(t, saveX, saveY) // 쉬지 않고 바로
	if e := g.next(t, "step"); e.Title != "'이름' 입력란에 '홍길동'을 입력합니다." {
		t.Errorf("입력 스텝 = %q, 마지막 글자까지 읽어야 한다", e.Title)
	}
}

// TestRecorder_ReadOnlyFieldOpensNoSession은 I2를 본다: 읽기 전용 입력란에서 친 키(글자·Space)는 입력 세션이 아니다.
func TestRecorder_ReadOnlyFieldOpensNoSession(t *testing.T) {
	g := newRig(t)
	openTwoFieldForm(t, "메모", win.ES_READONLY)
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop()
	click(t, edit2X, edit2Y)
	g.next(t, "step")
	typeText(t, "abc")
	press(t, vkSpace)
	time.Sleep(400 * time.Millisecond)
	press(t, vkTab)
	g.noStep(t, 700*time.Millisecond)
}

// TestRecorder_FocusMoveWithoutClickSplitsSessions는 I3을 본다: 클릭·Tab·Enter 없이 포커스가 다른 입력란으로 옮겨 가도
// (자동 넘김, Ctrl+L 등) 두 입력란이 각자의 입력 스텝이 된다.
func TestRecorder_FocusMoveWithoutClickSplitsSessions(t *testing.T) {
	g := newRig(t)
	openTwoFieldForm(t, "주소", 0)
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	defer r.Stop()
	click(t, editX, editY)
	g.next(t, "step")
	typeText(t, "홍길동")
	time.Sleep(100 * time.Millisecond) // 첫 키로 세션이 '이름'에 열릴 틈. 미리 읽기(250ms)보다는 짧게
	if err := win.FocusAt(edit2X, edit2Y); err != nil {
		t.Fatal(err)
	}
	typeText(t, "서울")
	time.Sleep(400 * time.Millisecond)
	press(t, vkTab)
	for _, want := range []string{"'이름' 입력란에 '홍길동'을 입력합니다.", "'주소' 입력란에 '서울'을 입력합니다."} {
		if e := g.next(t, "step"); e.Title != want {
			t.Errorf("입력 스텝 = %q, want %q", e.Title, want)
		}
	}
}

// hongGildong은 2벌식 자판에서 '홍길동'을 치는 가상 키다(ㅎㅗㅇ ㄱㅣㄹ ㄷㅗㅇ = g h d r l f e h d).
var hongGildong = []uint16{'G', 'H', 'D', 'R', 'L', 'F', 'E', 'H', 'D'}

// TestRecorder_KoreanIMETyping은 R22를 본다: 다른 테스트는 VK_PACKET으로 글자를 넣어 IME를 거치지 않는다. 여기서는
// 테스트 창을 한국어 IME 한글 모드로 켜고 실제 가상 키를 쳐서 IME가 '홍길동'을 조합하게 한다. 마지막 글자 '동'은
// 끝내는 키(Enter·Tab)나 클릭을 앱이 처리할 때에야 확정되는데, 훅은 그보다 먼저 불리므로 값 읽기가 '홍길'에서 멈추면 안 된다.
// 마지막 키 뒤 쉬지 않고 바로 끝낸다(가장 빠른 사람). 처음 만들 때 고침 없이 통과했다: 끝낼 때 '동'은 아직 조합 중이지만
// Win32 EDIT는 조합 중인 글자를 값에 바로 넣는다(ImmGetCompositionString '동', UIA 값 '홍길동'으로 확인). 그래서 값을
// 늦게 읽는 고침은 넣지 않았고 이 테스트는 회귀 방지용이다. TSF 앱(Edge, Win11 메모장)은 다를 수 있고 주입 입력이기도
// 해서 사람 손 확인(M1_RESULTS 6번)은 그대로 남는다.
func TestRecorder_KoreanIMETyping(t *testing.T) {
	for _, end := range []struct {
		name string
		do   func(t *testing.T)
	}{
		{"Enter", func(t *testing.T) { press(t, vkReturn) }},
		{"Tab", func(t *testing.T) { press(t, vkTab) }},
		{"다른 곳 클릭", func(t *testing.T) { click(t, saveX, saveY) }},
	} {
		t.Run(end.name, func(t *testing.T) {
			g := newRig(t)
			w := openForm(t, "이름", 0)
			r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
			defer r.Stop()
			click(t, editX, editY) // 테스트 창을 앞으로 가져오고 입력란에 포커스를 둔다
			g.next(t, "step")
			var prev uint32
			var err error
			w.Do(func() { prev, err = win.HangulOn() })
			if errors.Is(err, win.ErrNoKoreanIME) {
				t.Skipf("한국어 IME(2벌식)가 없어 건너뛴다: %v", err)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer w.Do(func() { win.SetConversion(prev) }) // 사용자 입력기의 한/영 상태를 되돌린다
			for _, vk := range hongGildong {
				press(t, vk)
				time.Sleep(20 * time.Millisecond)
			}
			end.do(t)
			if e := g.next(t, "step"); e.Title != "'이름' 입력란에 '홍길동'을 입력합니다." {
				t.Errorf("입력 스텝 = %q, 마지막 음절까지 읽어야 한다", e.Title)
			}
		})
	}
}

// 같은 자리를 빠르게 두 번 누르면(더블클릭) 스텝 하나가 되고 제목이 '더블클릭'이 된다. 이어지는 세 번째 클릭은 새 스텝이다.
func TestRecorder_DoubleClickMergesIntoOneStep(t *testing.T) {
	g := newRig(t)
	openForm(t, "이름", 0)
	r := g.start(t, recorder.Options{Mode: recorder.ModeWindow})
	click(t, saveX, saveY)
	click(t, saveX, saveY)
	first := g.next(t, "step")
	merged := g.next(t, "step")
	if merged.StepID != first.StepID || merged.Steps != 1 || merged.ImageSHA != first.ImageSHA ||
		merged.Title != "'저장' 버튼을 더블클릭합니다." {
		t.Errorf("합친 스텝 = %+v (첫 스텝 %+v)", merged, first)
	}
	click(t, saveX, saveY)
	if e := g.next(t, "step"); e.Steps != 2 || e.Title != "'저장' 버튼을 클릭합니다." {
		t.Errorf("세 번째 클릭 = %+v", e)
	}
	gid := r.Stop()
	steps, err := g.st.ListSteps(gid)
	if err != nil || len(steps) != 2 || steps[0].Title != "'저장' 버튼을 더블클릭합니다." {
		t.Errorf("저장된 스텝 = %+v, %v", steps, err)
	}
}
