// HWPX header.xml 고치기: 글꼴 이름, 이 작성기가 쓰는 글자 모양·문단 모양·테두리 채우기를 템플릿 목록 끝에 더한다.
// 번호는 템플릿 목록 개수 뒤에 이어 붙인 고정값이고, 템플릿을 다시 만들면 TestTemplate_Counts가 알려 준다.
package hwpx

import (
	"fmt"
	"regexp"
	"strings"
)

// 템플릿(template/blank.hwpx)의 목록 개수. 우리 항목 번호는 여기서 시작한다.
const (
	tplCharCount = 7
	tplParaCount = 20
	tplFillCount = 3
)

// 글자 모양 번호. 본문 글자는 charBody + (굵게 1 | 기울임 2 | 밑줄 4)다.
const (
	charBody     = tplCharCount      // 7~14: 본문 10pt 조합 8가지
	charLink     = tplCharCount + 8  // 파란 밑줄
	charDocTitle = tplCharCount + 9  // 20pt 굵게
	charStep     = tplCharCount + 10 // 14pt 굵게
	charSub      = tplCharCount + 11 // 12pt 굵게
	charH3       = tplCharCount + 12 // 12pt 굵게
	charH4       = tplCharCount + 13 // 11pt 굵게
	charSmall    = tplCharCount + 14 // 9pt
)

// 문단 모양 번호.
const (
	paraBody   = 0                // 템플릿 바탕글(양쪽 정렬, 줄 간격 160%)
	paraCenter = tplParaCount     // 가운데
	paraIndent = tplParaCount + 1 // +0~2: 목록 들여쓰기 1~3단계
	paraStep   = tplParaCount + 4 // 스텝 제목: 앞 간격, 다음 문단과 같은 쪽
)

// 테두리 채우기 번호. 3은 템플릿의 실선 테두리(표).
const (
	fillTable  = 3
	fillHeader = tplFillCount + 1 // 표 머리 칸: 옅은 회색
	fillBox    = tplFillCount + 2 // +0~3: 안내 상자 info, tip, warning, danger
)

// BoxColors는 안내 상자 종류별 (바탕, 테두리) 색이다. HTML·DOCX(calloutColors)와 같은 값을 쓴다.
// 내보내기(internal/export)가 드리프트 검사 테스트에서 읽으려고 내보낸다(검수 Minor 4).
var BoxColors = map[string][2]string{
	"info":    {"#E8F0FE", "#1A73E8"},
	"tip":     {"#E6F4EA", "#188038"},
	"warning": {"#FEF7E0", "#F9AB00"},
	"danger":  {"#FCE8E6", "#D93025"},
}

var boxKinds = []string{"info", "tip", "warning", "danger"}

func boxFill(kind string) int {
	for i, k := range boxKinds {
		if k == kind {
			return fillBox + i
		}
	}
	return fillBox
}

// langs는 일곱 언어 칸에 같은 값을 넣은 요소다(<hh:ratio hangul="100" … user="100"/>).
func langs(name, v string) string {
	return fmt.Sprintf(`<hh:%s hangul="%[2]s" latin="%[2]s" hanja="%[2]s" japanese="%[2]s" other="%[2]s" symbol="%[2]s" user="%[2]s"/>`, name, v)
}

func charPr(id, height int, color string, bold, italic, underline bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<hh:charPr id="%d" height="%d" textColor="%s" shadeColor="none" useFontSpace="0" useKerning="0" symMark="NONE" borderFillIDRef="2">`, id, height, color)
	b.WriteString(langs("fontRef", "0") + langs("ratio", "100") + langs("spacing", "0") + langs("relSz", "100") + langs("offset", "0"))
	if italic {
		b.WriteString(`<hh:italic/>`)
	}
	if bold {
		b.WriteString(`<hh:bold/>`)
	}
	ul := "NONE"
	if underline {
		ul = "BOTTOM"
	}
	fmt.Fprintf(&b, `<hh:underline type="%s" shape="SOLID" color="%s"/><hh:strikeout shape="NONE" color="#000000"/><hh:outline type="NONE"/><hh:shadow type="NONE" color="#C0C0C0" offsetX="10" offsetY="10"/></hh:charPr>`, ul, color)
	return b.String()
}

func paraPr(id int, align string, left, prev int, keepNext bool) string {
	keep := 0
	if keepNext {
		keep = 1
	}
	margin := fmt.Sprintf(`<hh:margin><hc:intent value="0" unit="HWPUNIT"/><hc:left value="%d" unit="HWPUNIT"/><hc:right value="0" unit="HWPUNIT"/><hc:prev value="%d" unit="HWPUNIT"/><hc:next value="0" unit="HWPUNIT"/></hh:margin><hh:lineSpacing type="PERCENT" value="160" unit="HWPUNIT"/>`, left, prev)
	return fmt.Sprintf(`<hh:paraPr id="%d" tabPrIDRef="0" condense="0" fontLineHeight="0" snapToGrid="1" suppressLineNumbers="0" checked="0" textDir="LTR">`+
		`<hh:align horizontal="%s" vertical="BASELINE"/><hh:heading type="NONE" idRef="0" level="0"/>`+
		`<hh:breakSetting breakLatinWord="KEEP_WORD" breakNonLatinWord="BREAK_WORD" widowOrphan="0" keepWithNext="%d" keepLines="0" pageBreakBefore="0" lineWrap="BREAK"/>`+
		`<hh:autoSpacing eAsianEng="0" eAsianNum="0"/>`+
		`<hp:switch><hp:case hp:required-namespace="http://www.hancom.co.kr/hwpml/2016/HwpUnitChar">%s</hp:case><hp:default>%s</hp:default></hp:switch>`+
		`<hh:border borderFillIDRef="2" offsetLeft="0" offsetRight="0" offsetTop="0" offsetBottom="0" connect="0" ignoreMargin="0"/></hh:paraPr>`,
		id, align, keep, margin, margin)
}

func borderFill(id int, face, line string) string {
	side := func(name string) string {
		return fmt.Sprintf(`<hh:%s type="SOLID" width="0.12 mm" color="%s"/>`, name, line)
	}
	return fmt.Sprintf(`<hh:borderFill id="%d" threeD="0" shadow="0" centerLine="NONE" breakCellSeparateLine="0">`+
		`<hh:slash type="NONE" Crooked="0" isCounter="0"/><hh:backSlash type="NONE" Crooked="0" isCounter="0"/>%s%s%s%s`+
		`<hh:diagonal type="SOLID" width="0.1 mm" color="#000000"/><hc:fillBrush><hc:winBrush faceColor="%s" hatchColor="#999999" alpha="0"/></hc:fillBrush></hh:borderFill>`,
		id, side("leftBorder"), side("rightBorder"), side("topBorder"), side("bottomBorder"), face)
}

// appendItems는 <hh:{list} itemCnt="n">…</hh:{list}> 끝에 items를 넣고 itemCnt를 want로 바꾼다.
func appendItems(s, list string, have, want int, items string) (string, error) {
	open := fmt.Sprintf(`<hh:%s itemCnt="%d">`, list, have)
	if !strings.Contains(s, open) {
		return "", fmt.Errorf("템플릿 header.xml에 %s가 없다. tools/make_hwpx_template.py로 다시 만들었다면 tpl*Count를 고친다", open)
	}
	s = strings.Replace(s, open, fmt.Sprintf(`<hh:%s itemCnt="%d">`, list, want), 1)
	return strings.Replace(s, "</hh:"+list+">", items+"</hh:"+list+">", 1), nil
}

var reFontFace = regexp.MustCompile(`(<hh:font id="\d+" face=")(함초롬돋움|함초롬바탕)(" type="TTF" isEmbedded="0">)`)

// patchHeader는 글꼴을 font로 바꾸고(없는 PC는 맑은 고딕으로 대신 보이게) 우리 모양 항목을 더한다.
func patchHeader(h []byte, font string) ([]byte, error) {
	s := string(h)
	subst := `<hh:substFont face="맑은 고딕" type="TTF" isEmbedded="0" binaryItemIDRef=""/>`
	if font == "맑은 고딕" {
		subst = ""
	}
	s = reFontFace.ReplaceAllString(s, "${1}"+esc(font)+"${3}"+subst)

	var chars strings.Builder
	for i := 0; i < 8; i++ {
		chars.WriteString(charPr(charBody+i, 1000, "#000000", i&1 != 0, i&2 != 0, i&4 != 0))
	}
	chars.WriteString(charPr(charLink, 1000, "#0000FF", false, false, true))
	chars.WriteString(charPr(charDocTitle, 2000, "#000000", true, false, false))
	chars.WriteString(charPr(charStep, 1400, "#000000", true, false, false))
	chars.WriteString(charPr(charSub, 1200, "#000000", true, false, false))
	chars.WriteString(charPr(charH3, 1200, "#000000", true, false, false))
	chars.WriteString(charPr(charH4, 1100, "#000000", true, false, false))
	chars.WriteString(charPr(charSmall, 900, "#000000", false, false, false))

	var paras strings.Builder
	paras.WriteString(paraPr(paraCenter, "CENTER", 0, 0, false))
	for lv := 1; lv <= 3; lv++ {
		paras.WriteString(paraPr(paraIndent+lv-1, "LEFT", 1600*lv, 0, false))
	}
	paras.WriteString(paraPr(paraStep, "LEFT", 0, 1400, true))

	var fills strings.Builder
	fills.WriteString(borderFill(fillHeader, "#F1F3F4", "#000000"))
	for _, k := range boxKinds {
		c := BoxColors[k]
		fills.WriteString(borderFill(boxFill(k), c[0], c[1]))
	}

	var err error
	if s, err = appendItems(s, "charProperties", tplCharCount, charSmall+1, chars.String()); err != nil {
		return nil, err
	}
	if s, err = appendItems(s, "paraProperties", tplParaCount, paraStep+1, paras.String()); err != nil {
		return nil, err
	}
	if s, err = appendItems(s, "borderFills", tplFillCount, fillBox+len(boxKinds)-1, fills.String()); err != nil {
		return nil, err
	}
	return []byte(s), nil
}
