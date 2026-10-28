// 규칙 기반 한국어 스텝 제목(스펙 4.3절): 컨트롤별 템플릿, 이름 정리, 문체(합니다체·하세요체).
package title

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// UIA ControlType ID(UIAutomationClient.h). uia 패키지는 Windows 전용이라 순수 Go인 이 패키지가 따로 갖는다.
const (
	Button    = 50000
	CheckBox  = 50002
	ComboBox  = 50003
	Edit      = 50004
	Hyperlink = 50005
	ListItem  = 50007
	MenuItem  = 50011
	TabItem   = 50019
	Document  = 50030
	Window    = 50032
	Pane      = 50033
)

type Action int

const (
	Click  Action = iota // 마우스 클릭
	Type                 // 입력 세션
	Manual               // 수동 캡처(단축키)
)

// Style은 문체다. settings.json의 title_style 값을 그대로 쓴다. 모르는 값은 합니다체로 본다.
type Style string

const (
	Hamnida Style = "합니다"
	Haseyo  Style = "하세요"
)

// Input은 제목 재료다. 녹화기가 UIA·OCR·창 정보로 채운다.
type Input struct {
	Action      Action
	ControlType int32
	Name        string // UIA 이름(정리 전)
	Value       string // Type: 입력란 값. 비밀번호는 호출 쪽이 ●●●●로 바꿔 넣는다. 빈 값은 "읽지 못함"이다
	Checked     bool   // CheckBox 클릭: 누른 뒤 선택 상태면 true
	Right       bool   // 오른쪽 버튼 클릭
	Double      bool   // 더블클릭: 녹화기가 같은 자리의 두 클릭을 한 스텝으로 합칠 때
	OCRText     string // 이름이 없을 때 클릭 지점 주변 OCR 결과
	WindowTitle string
}

const maxLen = 40 // 이름·값 최대 글자 수(스펙 4.3절)

const trailing = ".…:： " // 이름 끝에서 지울 말줄임·콜론·공백

var (
	reAccessKey = regexp.MustCompile(`\s*\(&?[A-Za-z]\)$`) // 저장(&S), 파일(F)
	reSpaces    = regexp.MustCompile(`\s+`)
	ampersand   = strings.NewReplacer("&&", "&", "&", "") // 접근 키 표시 &는 지우고 &&는 & 하나로
)

// Make는 스텝 제목을 만든다. 템플릿을 모두 합니다체로 만든 뒤, 하세요체면 끝 어미만 바꾼다.
func Make(in Input, style Style) string {
	s := hamnida(in)
	if in.Right {
		s = strings.Replace(s, "클릭합니다.", "마우스 오른쪽 버튼으로 클릭합니다.", 1)
	}
	if in.Double {
		// 메뉴·탭·항목처럼 "선택합니다"로 끝나는 템플릿도 더블클릭은 한 낱말로 맞춘다. "해제합니다"(체크 상자)는 그대로 둔다.
		for _, verb := range []string{"클릭합니다.", "선택합니다."} {
			if strings.HasSuffix(s, verb) {
				s = strings.TrimSuffix(s, verb) + "더블클릭합니다."
				break
			}
		}
	}
	if style == Haseyo {
		s = strings.TrimSuffix(s, "합니다.") + "하세요."
	}
	return s
}

// NeedsText는 UI 이름을 쓸 수 없어 OCR 글자나 창 제목으로 제목을 만들 클릭인지 알려 준다. 녹화기는 이때만 OCR을 돌린다.
// 창(Window)·창틀(Pane)의 이름은 대개 창 제목이라 버튼 이름처럼 쓰면 어색해서 이름 없음으로 본다.
func NeedsText(in Input) bool {
	return in.Action == Click && (Clean(in.Name) == "" || in.ControlType == Window || in.ControlType == Pane)
}

// Clean은 UI 이름을 제목에 넣기 좋게 다듬는다: 공백 정리, 끝의 말줄임·콜론과 접근 키 표기 제거, & 제거, 40자 제한.
func Clean(s string) string {
	s = reSpaces.ReplaceAllString(strings.TrimSpace(s), " ")
	s = strings.TrimRight(s, trailing)
	s = reAccessKey.ReplaceAllString(s, "")
	s = strings.TrimRight(ampersand.Replace(s), trailing)
	return shorten(s)
}

func shorten(s string) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	return string([]rune(s)[:maxLen-1]) + "…"
}

func q(s string) string { return "'" + s + "'" }

func hamnida(in Input) string {
	switch in.Action {
	case Manual:
		if t := Clean(in.WindowTitle); t != "" {
			return t + " 화면을 확인합니다."
		}
		return "화면을 확인합니다."
	case Type:
		// 값은 이름 정리(접근 키 제거 등)를 하지 않는다. 사용자가 친 글자라 공백만 정리해 자른다.
		return typed(Clean(in.Name), shorten(reSpaces.ReplaceAllString(strings.TrimSpace(in.Value), " ")))
	}
	if NeedsText(in) {
		if t := Clean(in.OCRText); t != "" {
			return q(t) + Josa(t, "을/를") + " 클릭합니다."
		}
		if t := Clean(in.WindowTitle); t != "" {
			return t + " 화면에서 클릭합니다."
		}
		return "화면을 클릭합니다."
	}
	name := Clean(in.Name)
	switch in.ControlType {
	case Button:
		return q(name) + " 버튼을 클릭합니다."
	case MenuItem:
		return q(name) + " 메뉴를 선택합니다."
	case TabItem:
		return q(name) + " 탭을 선택합니다."
	case Edit, Document:
		return q(name) + " 입력란을 클릭합니다."
	case CheckBox:
		if in.Checked {
			return q(name) + Josa(name, "을/를") + " 선택합니다."
		}
		return q(name) + Josa(name, "을/를") + " 해제합니다."
	case ComboBox, ListItem:
		return q(name) + " 항목을 선택합니다."
	case Hyperlink:
		return q(name) + " 링크를 클릭합니다."
	}
	return q(name) + Josa(name, "을/를") + " 클릭합니다."
}

// typed는 입력 세션 제목이다. 값을 읽지 못하면(빈 값) "텍스트를 입력합니다"로 쓴다(스펙 4.2절).
func typed(name, value string) string {
	switch {
	case name != "" && value != "":
		return q(name) + " 입력란에 " + q(value) + Josa(value, "을/를") + " 입력합니다."
	case name != "":
		return q(name) + " 입력란에 텍스트를 입력합니다."
	case value != "":
		return q(value) + Josa(value, "을/를") + " 입력합니다."
	}
	return "텍스트를 입력합니다."
}
