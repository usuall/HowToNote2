// 제목 템플릿(스펙 4.3절), 이름 정리, 문체 테이블 테스트.
package title

import (
	"strings"
	"testing"
)

func TestMake(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{"버튼", Input{ControlType: Button, Name: "저장(&S)"}, "'저장' 버튼을 클릭합니다."},
		{"메뉴", Input{ControlType: MenuItem, Name: "다른 이름으로 저장(A)..."}, "'다른 이름으로 저장' 메뉴를 선택합니다."},
		{"탭", Input{ControlType: TabItem, Name: "보기"}, "'보기' 탭을 선택합니다."},
		{"입력란 클릭", Input{ControlType: Edit, Name: "이름:"}, "'이름' 입력란을 클릭합니다."},
		{"체크 선택", Input{ControlType: CheckBox, Name: "자동 저장", Checked: true}, "'자동 저장'을 선택합니다."},
		{"체크 해제", Input{ControlType: CheckBox, Name: "미리 보기"}, "'미리 보기'를 해제합니다."},
		{"콤보", Input{ControlType: ComboBox, Name: "글꼴"}, "'글꼴' 항목을 선택합니다."},
		{"목록 항목", Input{ControlType: ListItem, Name: "문서"}, "'문서' 항목을 선택합니다."},
		{"링크", Input{ControlType: Hyperlink, Name: "도움말"}, "'도움말' 링크를 클릭합니다."},
		{"그 밖의 컨트롤", Input{ControlType: 50020, Name: "PDF"}, "'PDF'를 클릭합니다."},
		{"이름 없음 OCR", Input{ControlType: Button, OCRText: " 확인 "}, "'확인'을 클릭합니다."},
		{"창틀 이름은 이름 없음", Input{ControlType: Pane, Name: "제목 없음 - 메모장", OCRText: "파일"}, "'파일'을 클릭합니다."},
		{"창 제목 대체", Input{WindowTitle: "제목 없음 - 메모장"}, "제목 없음 - 메모장 화면에서 클릭합니다."},
		{"아무것도 없음", Input{}, "화면을 클릭합니다."},
		{"오른쪽 클릭", Input{ControlType: Button, Name: "저장", Right: true}, "'저장' 버튼을 마우스 오른쪽 버튼으로 클릭합니다."},
		{"입력", Input{Action: Type, ControlType: Edit, Name: "이름", Value: "홍길동"}, "'이름' 입력란에 '홍길동'을 입력합니다."},
		{"입력 받침 없음", Input{Action: Type, ControlType: Edit, Name: "이름", Value: "김철수"}, "'이름' 입력란에 '김철수'를 입력합니다."},
		{"입력값 공백 정리", Input{Action: Type, ControlType: Edit, Name: "이름", Value: "홍길동\t"}, "'이름' 입력란에 '홍길동'을 입력합니다."},
		{"비밀번호", Input{Action: Type, ControlType: Edit, Name: "비밀번호", Value: "●●●●"}, "'비밀번호' 입력란에 '●●●●'를 입력합니다."},
		{"값 못 읽음", Input{Action: Type, ControlType: Edit, Name: "주소"}, "'주소' 입력란에 텍스트를 입력합니다."},
		{"이름·값 없음", Input{Action: Type}, "텍스트를 입력합니다."},
		{"수동 캡처", Input{Action: Manual, WindowTitle: "설정"}, "설정 화면을 확인합니다."},
		{"수동 캡처 제목 없음", Input{Action: Manual}, "화면을 확인합니다."},
	}
	for _, c := range cases {
		if got := Make(c.in, Hamnida); got != c.want {
			t.Errorf("%s: Make = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestMake_HaseyoStyle(t *testing.T) {
	cases := []struct {
		in   Input
		want string
	}{
		{Input{ControlType: Button, Name: "저장"}, "'저장' 버튼을 클릭하세요."},
		{Input{Action: Type, ControlType: Edit, Name: "이름", Value: "홍길동"}, "'이름' 입력란에 '홍길동'을 입력하세요."},
		{Input{ControlType: CheckBox, Name: "자동 저장"}, "'자동 저장'을 해제하세요."},
		{Input{Action: Manual, WindowTitle: "설정"}, "설정 화면을 확인하세요."},
	}
	for _, c := range cases {
		if got := Make(c.in, Haseyo); got != c.want {
			t.Errorf("Make(%+v, 하세요) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClean(t *testing.T) {
	cases := []struct{ in, want string }{
		{"저장(&S)", "저장"},
		{"파일(F)", "파일"},
		{"다른 이름으로 저장(&A)...", "다른 이름으로 저장"},
		{"열기…", "열기"},
		{"이름:", "이름"},
		{"&Save", "Save"},
		{"Save && Exit", "Save & Exit"},
		{"  여러   칸\n공백 ", "여러 칸 공백"},
		{"항목(1)", "항목(1)"}, // 숫자 괄호는 접근 키가 아니다
		{strings.Repeat("가", 45), strings.Repeat("가", 39) + "…"},
	}
	for _, c := range cases {
		if got := Clean(c.in); got != c.want {
			t.Errorf("Clean(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNeedsText(t *testing.T) {
	cases := []struct {
		in   Input
		want bool
	}{
		{Input{ControlType: Button, Name: "저장"}, false},
		{Input{ControlType: Button}, true},
		{Input{ControlType: Button, Name: "..."}, true},
		{Input{ControlType: Pane, Name: "제목 없음 - 메모장"}, true},
		{Input{ControlType: Window, Name: "설정"}, true},
		{Input{Action: Type, ControlType: Edit}, false},
	}
	for _, c := range cases {
		if got := NeedsText(c.in); got != c.want {
			t.Errorf("NeedsText(%+v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestMake_Double(t *testing.T) {
	cases := []struct {
		in   Input
		want string
	}{
		{Input{ControlType: Button, Name: "저장", Double: true}, "'저장' 버튼을 더블클릭합니다."},
		{Input{ControlType: ListItem, Name: "문서", Double: true}, "'문서' 항목을 더블클릭합니다."},
		{Input{ControlType: Pane, OCRText: "보고서.hwpx", Double: true}, "'보고서.hwpx'를 더블클릭합니다."},
		{Input{WindowTitle: "파일 탐색기", Double: true}, "파일 탐색기 화면에서 더블클릭합니다."},
		{Input{ControlType: CheckBox, Name: "자동 저장", Double: true}, "'자동 저장'을 해제합니다."},
	}
	for _, c := range cases {
		if got := Make(c.in, Hamnida); got != c.want {
			t.Errorf("Make(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := Make(Input{ControlType: Button, Name: "저장", Double: true}, Haseyo); got != "'저장' 버튼을 더블클릭하세요." {
		t.Errorf("하세요체 더블클릭 = %q", got)
	}
}
