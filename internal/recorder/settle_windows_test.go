// 체크박스 "누른 뒤" 상태를 다시 읽지 못했을 때의 제목(C1 대체 경로) 테스트.
package recorder

import (
	"testing"
	"time"

	"howtonote/internal/title"
	"howtonote/internal/uia"
)

// 다시 읽기를 못 하면 선택/해제를 추측하지 않고 일반 클릭 제목을 쓴다. 첫 읽기는 화면을 찍은 뒤라 이미 누른 뒤
// 상태일 때가 많아, 뒤집으면 틀린 제목이 된다. 표시 지점이 없으면(point nil) UIA를 부르지 않고 바로 이 경로로 간다.
func TestSettleCheckbox_FallbackUsesGenericClickTitle(t *testing.T) {
	s := shot{at: time.Now().Add(-time.Second), el: uia.Element{ControlType: uia.CheckBoxControlType, ToggleState: 1},
		in: title.Input{Action: title.Click, ControlType: uia.CheckBoxControlType, Name: "동의"}}
	new(Recorder).settleCheckbox(&s)
	if got := title.Make(s.in, title.Hamnida); got != "'동의'를 클릭합니다." {
		t.Errorf("제목 = %q, 일반 클릭 제목이어야 한다", got)
	}
}
