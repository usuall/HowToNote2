package uia

import "testing"

func TestShouldReadValue(t *testing.T) {
	cases := []struct {
		name       string
		isPassword interface{}
		want       bool
	}{
		{"읽기 실패(nil)", nil, false},
		{"비밀번호 맞음(true)", true, false},
		{"비밀번호 아님(false)", false, true},
		{"기대 못한 타입", "true", false},
	}
	for _, c := range cases {
		if got := shouldReadValue(c.isPassword); got != c.want {
			t.Errorf("%s: shouldReadValue(%#v) = %v, want %v", c.name, c.isPassword, got, c.want)
		}
	}
}

func TestReadOnly(t *testing.T) {
	const document = 50030 // UIA_DocumentControlTypeId
	cases := []struct {
		name        string
		raw         interface{}
		hasValue    interface{} // IsValuePatternAvailable 읽기 결과
		controlType int32
		want        bool
	}{
		{"읽기 전용(true)", true, true, EditControlType, true},
		{"쓸 수 있음(false)", false, true, EditControlType, false},
		{"못 읽음 + Edit: 쓸 수 있다고 본다", nil, true, EditControlType, false},
		{"못 읽음 + Document: 읽기 전용으로 본다", nil, true, document, true},
		{"기대 못한 타입 + Document", "false", true, document, true},
		// Value 패턴이 없으면 IsReadOnly는 UIA 기본값 TRUE일 뿐이다(R22, 스펙 4.2절 4): 값은 못 읽어도 입력은 된다
		{"Value 패턴 없음 + Edit(기본값 true)", true, false, EditControlType, false},
		{"Value 패턴 없음 + Document(기본값 true)", true, false, document, false},
		{"패턴 여부 못 읽음 + 읽기 전용", true, nil, EditControlType, true},
		{"패턴 여부 못 읽음 + 둘 다 못 읽음 + Document", nil, nil, document, true},
	}
	for _, c := range cases {
		if got := readOnly(c.raw, c.hasValue, c.controlType); got != c.want {
			t.Errorf("%s: readOnly(%#v, %#v, %d) = %v, want %v", c.name, c.raw, c.hasValue, c.controlType, got, c.want)
		}
	}
}
