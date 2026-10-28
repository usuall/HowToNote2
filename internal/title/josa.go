// 조사 고르기(을/를, 이/가, 은/는). 마지막 글자의 받침으로 고르고, 숫자·영문은 읽는 소리로 고른다(스펙 4.3절).
package title

import (
	"strings"
	"unicode"
)

// Josa는 word 뒤에 붙일 조사를 고른다. pair는 "을/를", "이/가", "은/는"처럼 받침 있을 때/없을 때 순서다.
// 받침을 판단할 수 없으면(기호, 한자 등) "을(를)"처럼 둘 다 적는다.
func Josa(word, pair string) string {
	with, without, _ := strings.Cut(pair, "/")
	switch batchim(word) {
	case yes:
		return with
	case no:
		return without
	}
	return with + "(" + without + ")"
}

type answer int

const (
	unknown answer = iota
	yes
	no
)

func yesIf(b bool) answer {
	if b {
		return yes
	}
	return no
}

// batchim은 word를 소리 내어 읽을 때 끝에 받침이 있는지 본다. 끝의 따옴표·괄호·문장부호는 건너뛴다.
func batchim(word string) answer {
	rs := []rune(strings.TrimRightFunc(word, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '●'
	}))
	if len(rs) == 0 {
		return unknown
	}
	last := rs[len(rs)-1]
	switch {
	case last == '●': // 비밀번호 가림표(●●●●, 스펙 4.2절)는 읽는 소리가 없어 받침 없는 쪽("를")으로 정해 둔다
		return no
	case last >= 0xAC00 && last <= 0xD7A3: // 한글 음절: 종성 번호가 0이 아니면 받침
		return yesIf((last-0xAC00)%28 != 0)
	case last >= '0' && last <= '9':
		// 영·일·삼·육·칠·팔은 받침이 있다. 0으로 끝나는 수도 십·백·천·만·억으로 읽어 받침이 있다.
		// ponytail: 조(10^12)처럼 받침 없는 큰 단위는 무시한다
		return yesIf(strings.ContainsRune("013678", last))
	case last < 0x80 && unicode.IsLetter(last):
		return latin(rs)
	}
	return unknown
}

// latin은 끝의 영문 단어를 읽는 소리로 받침을 정한다.
// ponytail: 짧은 규칙표라 외래어 표기 예외(File=파일, Cancel=캔슬)는 틀린다. 틀리는 예가 쌓이면 단어표를 붙인다
func latin(rs []rune) answer {
	i := len(rs)
	for i > 0 && rs[i-1] < 0x80 && unicode.IsLetter(rs[i-1]) {
		i--
	}
	w := string(rs[i:])
	if strings.ToUpper(w) == w { // 약어(PDF, URL)나 대문자 한 글자는 알파벳 이름으로 읽는다: 엘·엠·엔·알만 받침
		return yesIf(strings.ContainsRune("LMNR", rs[len(rs)-1]))
	}
	w = strings.ToLower(w)
	n := len(w)
	switch w[n-1] {
	case 'l', 'm', 'n':
		return yes
	case 'g': // -ng만 받침(String=스트링, Blog=블로그)
		return yesIf(n >= 2 && w[n-2] == 'n')
	case 'b', 'k', 'p', 't': // 모음 바로 뒤 파열음만 받침(Web=웹, Chat=챗, Desk=데스크)
		return yesIf(n >= 2 && strings.ContainsRune("aeiou", rune(w[n-2])))
	}
	return no
}
