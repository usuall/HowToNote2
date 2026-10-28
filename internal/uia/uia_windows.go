// UI Automation 클라이언트. 화면 좌표나 포커스의 요소 정보를 읽는다.
// COM 호출은 MTA로 초기화한 전용 OS 스레드 하나에서 순서대로 실행한다.
package uia

import (
	"errors"
	"fmt"
	"image"
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"

	"howtonote/internal/win"
)

var (
	ErrTimeout = errors.New("uia: 시간 초과")
	ErrClosed  = errors.New("uia: 서비스가 이미 닫힘")
)

type Element struct {
	Name          string
	LocalizedType string
	AutomationID  string
	Value         string
	ControlType   int32
	ProcessID     int32
	IsPassword    bool
	ReadOnly      bool            // 값을 바꿀 수 없다(Value 패턴의 IsReadOnly). 녹화기는 이런 요소에 입력 세션을 열지 않는다
	HasValue      bool            // Value 패턴이 있다(IsValuePatternAvailable). 없으면 Value는 늘 빈 값이다
	Rect          image.Rectangle // 화면 경계(BoundingRectangle, 물리 픽셀). 입력 스텝의 표시 위치와 같은 요소 판별에 쓴다
	ToggleState   int32           // CheckBox: 0 해제, 1 선택, 2 불확정. 토글을 지원하지 않는 요소(버튼 등)는
	// 이 OS에서 기본값 2(불확정)를 돌려준다 - UIA가 비지원 속성에 내려주는 기본값이며 "해제" 의미가 아니다.
}

const (
	ButtonControlType   = 50000
	CheckBoxControlType = 50002
	EditControlType     = 50004
)

var (
	// CLSID_CUIAutomation(레거시)로 만든 객체는 IUIAutomation만 구현하고 QueryInterface로도
	// IUIAutomation2로 못 올라간다(E_NOINTERFACE, 이 환경에서 직접 확인함). CUIAutomation8은
	// IUIAutomation을 그대로 구현하면서 IUIAutomation2 이상도 지원해 Start()에서 쓴다.
	clsidCUIAutomation8 = ole.NewGUID("{E22AD333-B25F-460C-83D0-0581107395C9}")
	iidIUIAutomation    = ole.NewGUID("{30CBE57D-D9D0-452A-AB13-7AC5AC4825EE}")
	iidIUIAutomation2   = ole.NewGUID("{34723AFF-0C9D-49D0-9896-7AB52DF8CD8A}")
)

const (
	slotQueryInterface   = 0  // IUnknown::QueryInterface
	slotRelease          = 2  // IUnknown::Release
	slotElementFromPoint = 7  // IUIAutomation::ElementFromPoint
	slotGetFocused       = 8  // IUIAutomation::GetFocusedElement
	slotGetPropertyValue = 10 // IUIAutomationElement::GetCurrentPropertyValue

	// IUIAutomation2는 IUIAutomation(슬롯 0~57)을 이어받아 슬롯 58부터 제 메서드를 더한다.
	// mingw-w64 uiautomationclient.h의 IUIAutomation2Vtbl 선언 순서를 세어 확인했다:
	// get_AutoSetFocus=58, put_AutoSetFocus=59, get_ConnectionTimeout=60, put_ConnectionTimeout=61,
	// get_TransactionTimeout=62, put_TransactionTimeout=63.
	slotGetConnectionTimeout  = 60
	slotPutConnectionTimeout  = 61
	slotGetTransactionTimeout = 62
	slotPutTransactionTimeout = 63

	propProcessID     = 30002
	propControlType   = 30003
	propLocalizedType = 30004
	propName          = 30005
	propAutomationID  = 30011
	propIsPassword    = 30019
	propValue         = 30045
	propReadOnly      = 30046 // UIA_ValueIsReadOnlyPropertyId
	propHasValue      = 30043 // UIA_IsValuePatternAvailablePropertyId
	propBoundingRect  = 30001
	propToggleState   = 30086
)

// uia2TimeoutMS는 IUIAutomation2 ConnectionTimeout·TransactionTimeout에 맞출 호출자 예산(ms)이다.
// do()의 호출자 timeout과 비슷한 수준으로 묶어, 대상 앱 하나가 멈춰도 전용 스레드가 OS 기본값(약 20초)만큼
// 통째로 막히지 않게 한다.
const uia2TimeoutMS = 300

type request struct {
	focused bool
	x, y    int32
	reply   chan result
}

type result struct {
	el  Element
	err error
}

// Service는 UIA 전용 스레드를 소유한다.
type Service struct {
	reqs chan request
	quit chan struct{}

	closeOnce sync.Once

	uia2Available   bool
	uia2ConnTimeout int32
	uia2TxnTimeout  int32
}

func Start() (*Service, error) {
	s := &Service{reqs: make(chan request), quit: make(chan struct{})}
	ready := make(chan error, 1)
	go s.loop(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) ElementAt(x, y int32, timeout time.Duration) (Element, error) {
	return s.do(request{x: x, y: y}, timeout)
}

func (s *Service) Focused(timeout time.Duration) (Element, error) {
	return s.do(request{focused: true}, timeout)
}

// uia2Timeouts는 Start 때 IUIAutomation2로 되읽은 ConnectionTimeout·TransactionTimeout(ms)을 돌려준다.
// ok=false면 이 OS에 IUIAutomation2가 없어 설정을 건너뛴 것이다(그래도 서비스 자체는 쓸 수 있다).
// 운영 코드의 공개 API가 아니다 - M1-carry-1 되읽기 확인용으로 export_test.go가 테스트에만 노출한다.
func (s *Service) uia2Timeouts() (connMS, txnMS int32, ok bool) {
	return s.uia2ConnTimeout, s.uia2TxnTimeout, s.uia2Available
}

// Close는 여러 번 불러도 안전하다. 닫은 뒤의 요청은 ErrTimeout이 아니라 ErrClosed를 바로 돌려준다.
func (s *Service) Close() {
	s.closeOnce.Do(func() { close(s.quit) })
}

// do는 요청을 전용 스레드에 넘긴다. 앞 요청이 대상 앱 때문에 멈춰 있으면 시간 초과로 돌아온다.
// quit이 이미 닫혀 있으면(Close 호출 뒤) 시간 초과를 기다리지 않고 바로 ErrClosed를 돌려준다.
func (s *Service) do(r request, timeout time.Duration) (Element, error) {
	r.reply = make(chan result, 1)
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case s.reqs <- r:
	case <-s.quit:
		return Element{}, ErrClosed
	case <-t.C:
		return Element{}, ErrTimeout
	}
	select {
	case res := <-r.reply:
		return res.el, res.err
	case <-s.quit:
		return Element{}, ErrClosed
	case <-t.C:
		return Element{}, ErrTimeout
	}
}

func (s *Service) loop(ready chan<- error) {
	runtime.LockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		ready <- fmt.Errorf("CoInitializeEx: %w", err)
		return
	}
	defer ole.CoUninitialize()
	unk, err := ole.CreateInstance(clsidCUIAutomation8, iidIUIAutomation)
	if err != nil {
		ready <- fmt.Errorf("CUIAutomation8 생성: %w", err)
		return
	}
	defer unk.Release()
	auto := unsafe.Pointer(unk)
	s.setupUIA2(auto)
	ready <- nil
	for {
		select {
		case <-s.quit:
			return
		case r := <-s.reqs:
			el, err := query(auto, r)
			r.reply <- result{el, err}
		}
	}
}

// setupUIA2는 IUIAutomation을 IUIAutomation2로 올려 연결·트랜잭션 시간 제한을 uia2TimeoutMS로 줄인다.
// 안 하면 대상 앱 하나가 멈췄을 때 OS 기본값(약 20초) 동안 이 전용 스레드가 통째로 막힌다.
// 구버전 OS는 IUIAutomation2가 없을 수 있어 QueryInterface가 실패하면 조용히 건너뛴다.
func (s *Service) setupUIA2(auto unsafe.Pointer) {
	out := new(unsafe.Pointer)
	if win.Failed(win.ComCall(auto, slotQueryInterface, uintptr(unsafe.Pointer(iidIUIAutomation2)), uintptr(unsafe.Pointer(out)))) {
		return
	}
	auto2 := *out
	if auto2 == nil {
		return
	}
	defer win.ComCall(auto2, slotRelease)
	win.ComCall(auto2, slotPutConnectionTimeout, uintptr(uia2TimeoutMS))
	win.ComCall(auto2, slotPutTransactionTimeout, uintptr(uia2TimeoutMS))

	conn, txn := new(int32), new(int32)
	okConn := !win.Failed(win.ComCall(auto2, slotGetConnectionTimeout, uintptr(unsafe.Pointer(conn))))
	okTxn := !win.Failed(win.ComCall(auto2, slotGetTransactionTimeout, uintptr(unsafe.Pointer(txn))))
	if okConn && okTxn {
		s.uia2Available = true
		s.uia2ConnTimeout = *conn
		s.uia2TxnTimeout = *txn
	}
}

func query(auto unsafe.Pointer, r request) (Element, error) {
	out := new(unsafe.Pointer)
	var hr uintptr
	if r.focused {
		hr = win.ComCall(auto, slotGetFocused, uintptr(unsafe.Pointer(out)))
	} else {
		hr = win.ComCall(auto, slotElementFromPoint, win.PointArg(r.x, r.y), uintptr(unsafe.Pointer(out)))
	}
	if win.Failed(hr) {
		return Element{}, fmt.Errorf("uia: HRESULT 0x%08X", uint32(hr))
	}
	el := *out
	if el == nil {
		return Element{}, errors.New("uia: 요소 없음")
	}
	defer win.ComCall(el, slotRelease)
	isPasswordRaw := prop(el, propIsPassword)
	isPassword, _ := isPasswordRaw.(bool)
	value := "" // 비밀번호 필드는 Value를 읽지 않는다: 일부 제공자가 평문을 그대로 돌려준다
	if shouldReadValue(isPasswordRaw) {
		value = propString(el, propValue)
	}
	controlType := propInt(el, propControlType)
	hasValue := prop(el, propHasValue)
	hasValueBool, _ := hasValue.(bool)
	return Element{
		Name:          propString(el, propName),
		LocalizedType: propString(el, propLocalizedType),
		AutomationID:  propString(el, propAutomationID),
		Value:         value,
		ControlType:   controlType,
		ProcessID:     propInt(el, propProcessID),
		IsPassword:    isPassword,
		ReadOnly:      readOnly(prop(el, propReadOnly), hasValue, controlType),
		HasValue:      hasValueBool,
		Rect:          propRect(el),
		ToggleState:   propInt(el, propToggleState),
	}, nil
}

func prop(el unsafe.Pointer, id int) interface{} {
	v := new(ole.VARIANT)
	if win.Failed(win.ComCall(el, slotGetPropertyValue, uintptr(id), uintptr(unsafe.Pointer(v)))) {
		return nil
	}
	defer v.Clear()
	return v.Value()
}

func propString(el unsafe.Pointer, id int) string { s, _ := prop(el, id).(string); return s }
func propInt(el unsafe.Pointer, id int) int32     { i, _ := prop(el, id).(int32); return i }

// shouldReadValue는 IsPassword 속성 읽기가 "확실히 비밀번호 아님"(bool false)으로 돌아왔을 때만 true다.
// 읽기 실패(타임아웃 등으로 prop이 nil을 돌려줌)나 기대 못한 타입이면 비밀번호일 수도 있다고 보고 false를
// 돌려준다(안전 쪽으로 닫힘 - propBool처럼 실패를 false로 뭉개면 타임아웃 뒤 Value를 읽어 평문이 샐 수 있다).
func shouldReadValue(isPassword interface{}) bool {
	b, ok := isPassword.(bool)
	return ok && !b
}

// readOnly는 IsReadOnly 읽기 결과를 해석한다. Value 패턴이 확실히 없으면(hasValue false) 쓸 수 있다고 본다: 그때
// IsReadOnly는 UIA가 내려주는 기본값 TRUE일 뿐이라, 믿으면 Value 패턴 없는 입력란(메모장 본문 등)에서 입력 스텝이
// 아예 안 생긴다(R22). 값은 못 읽으니 제목은 "텍스트를 입력합니다"가 된다(스펙 4.2절 4).
// IsReadOnly를 못 읽으면(nil·기대 못한 타입) Edit는 쓸 수 있다고, 그 밖(Document 등)은 읽기 전용으로 본다:
// Edit는 거의 늘 입력란이지만 Document는 읽기만 하는 문서 보기인 경우가 많다.
func readOnly(raw, hasValue interface{}, controlType int32) bool {
	if b, ok := hasValue.(bool); ok && !b {
		return false
	}
	if b, ok := raw.(bool); ok {
		return b
	}
	return controlType != EditControlType
}

// propRect는 BoundingRectangle(double 4개 배열: 왼쪽, 위, 너비, 높이)을 사각형으로 바꾼다. 못 읽으면 빈 사각형이다.
func propRect(el unsafe.Pointer) image.Rectangle {
	v := new(ole.VARIANT)
	if win.Failed(win.ComCall(el, slotGetPropertyValue, propBoundingRect, uintptr(unsafe.Pointer(v)))) {
		return image.Rectangle{}
	}
	defer v.Clear()
	arr := v.ToArray()
	if arr == nil {
		return image.Rectangle{}
	}
	vals := arr.ToValueArray()
	if len(vals) != 4 {
		return image.Rectangle{}
	}
	var d [4]float64
	for i, x := range vals {
		d[i], _ = x.(float64)
	}
	// 왼쪽·위를 먼저 반올림하고 오른쪽·아래를 (왼쪽/위 + 너비/높이)에서 반올림해야, 각 값을 따로
	// 잘라낼 때 생기는 ±1픽셀 오차 없이 너비가 그대로 보존된다.
	left, top := math.Round(d[0]), math.Round(d[1])
	right, bottom := math.Round(d[0]+d[2]), math.Round(d[1]+d[3])
	return image.Rect(int(left), int(top), int(right), int(bottom))
}
