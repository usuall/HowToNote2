// 문서 모양(내보내기 프리셋, 스펙 7.3절) 옵션, 머리글·바닥글 자리표시자, 파일 이름 정리(스펙 7.4절).
package export

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Options는 export_presets.options_json에 저장하는 문서 모양이다. 빠진 칸은 DefaultOptions 값을 쓴다.
type Options struct {
	Paper       string `json:"paper"`       // "A4" 또는 "Letter"
	Landscape   bool   `json:"landscape"`   // 가로 방향
	Cover       bool   `json:"cover"`       // 표지
	TOC         bool   `json:"toc"`         // 목차
	StepPerPage bool   `json:"stepPerPage"` // 1스텝 1쪽(최상위 스텝마다 새 쪽)
	ImageWidth  int    `json:"imageWidth"`  // 그림 최대 폭: 본문 폭의 %(10~100)
	Header      string `json:"header"`      // 머리글. 자리표시자 {{매뉴얼제목}}(옛 이름 {{가이드제목}}) {{날짜}} {{쪽}} {{전체쪽}}
	Footer      string `json:"footer"`      // 바닥글
	Font        string `json:"font"`        // 본문 글꼴. 비면 형식 기본(PDF Pretendard, HWPX 함초롬돋움, DOCX·HTML 맑은 고딕)
	EmbedFont   bool   `json:"embedFont"`   // HTML에 Pretendard 글꼴을 넣는다
}

func DefaultOptions() Options {
	return Options{Paper: "A4", Cover: true, TOC: true, ImageWidth: 100, Header: "{{매뉴얼제목}}", Footer: "{{쪽}} / {{전체쪽}}"}
}

// Normalize는 저장소나 프론트에서 온 값을 쓸 수 있는 범위로 맞춘다.
func (o Options) Normalize() Options {
	if o.Paper != "Letter" {
		o.Paper = "A4"
	}
	o.ImageWidth = min(max(o.ImageWidth, 10), 100)
	if !Fonts[o.Font] {
		o.Font = "" // 고를 수 있는 글꼴만 받는다. 글꼴 이름은 CSS·XML에 그대로 들어간다
	}
	return o
}

// Fonts는 문서 모양에서 고를 수 있는 본문 글꼴이다. 빈 문자열은 형식 기본이다. 프론트 글꼴 고르기와 같다.
var Fonts = map[string]bool{"": true, "맑은 고딕": true, "함초롬돋움": true}

// Formats는 내보낼 수 있는 형식과 파일 확장자다. 프론트 형식 카드와 같은 순서다.
var Formats = map[string]string{"pdf": ".pdf", "hwpx": ".hwpx", "docx": ".docx", "html": ".html"}

// Field는 머리글·바닥글 안의 쪽 번호 자리다.
type Field int

const (
	NoField Field = iota
	PageField
	TotalField
)

// Piece는 머리글·바닥글을 자리표시자로 나눈 조각이다. Field가 있으면 Text는 비어 있다.
type Piece struct {
	Text  string
	Field Field
}

// {{가이드제목}}은 화면 용어를 "매뉴얼"로 바꾸기 전의 이름이다. 이미 저장된 문서 모양에 남아 있을 수 있어 계속 받는다.
var rePlaceholder = regexp.MustCompile(`\{\{(매뉴얼제목|가이드제목|날짜|쪽|전체쪽)\}\}`)

// Pieces는 머리글·바닥글 문구의 제목·날짜를 채우고 쪽 번호 자리로 나눈다. 모르는 {{…}}는 글자 그대로 둔다.
func Pieces(tmpl, title, date string) []Piece {
	var out []Piece
	text := func(s string) {
		if s == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].Field == NoField {
			out[n-1].Text += s
			return
		}
		out = append(out, Piece{Text: s})
	}
	last := 0
	for _, m := range rePlaceholder.FindAllStringSubmatchIndex(tmpl, -1) {
		text(tmpl[last:m[0]])
		switch tmpl[m[2]:m[3]] {
		case "매뉴얼제목", "가이드제목":
			text(title)
		case "날짜":
			text(date)
		case "쪽":
			out = append(out, Piece{Field: PageField})
		case "전체쪽":
			out = append(out, Piece{Field: TotalField})
		}
		last = m[1]
	}
	text(tmpl[last:])
	return out
}

var reservedNames = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}

// FileName은 매뉴얼 제목으로 저장할 파일 이름을 만든다. 한글은 그대로 두고 Windows 금지 문자 <>:"/\|?*와
// 제어 문자, 끝의 점·공백만 지운다(스펙 7.4절). 장치 이름(CON 등)은 Windows가 만들 수 없어 뒤에 _를 붙인다.
// Windows는 "con.txt"도 CON 장치로 보므로 첫 점 앞부분만 보고 판정한다(컨트롤러 재정 M3-C6).
func FileName(title, ext string) string {
	s := strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return -1
		}
		return r
	}, title)
	s = strings.TrimRight(s, ". ")
	for utf8.RuneCountInString(s) > 100 {
		_, size := utf8.DecodeLastRuneInString(s)
		s = strings.TrimRight(s[:len(s)-size], ". ")
	}
	if s == "" {
		s = "매뉴얼"
	}
	// "_"는 장치 이름 바로 뒤에 붙인다("con.txt" → "con_.txt"). 끝에만 붙이면 앞부분이 여전히 "con"이라
	// Windows가 장치로 본다(검수 재정: M3-C6 수정).
	if parts := strings.SplitN(s, ".", 2); reservedNames[strings.ToUpper(parts[0])] {
		parts[0] += "_"
		s = strings.Join(parts, ".")
	}
	return s + ext
}
