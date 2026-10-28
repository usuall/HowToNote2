// 문서 모양 옵션, 머리글·바닥글 자리표시자, 파일 이름 정리(스펙 7.3절, 7.4절) 테스트.
package export

import (
	"reflect"
	"testing"
)

func TestPieces(t *testing.T) {
	got := Pieces("{{가이드제목}} - {{날짜}} | {{쪽}} / {{전체쪽}}쪽 {{모름}}", "보고서", "2026. 10. 21.")
	want := []Piece{{Text: "보고서 - 2026. 10. 21. | "}, {Field: PageField}, {Text: " / "}, {Field: TotalField}, {Text: "쪽 {{모름}}"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Pieces = %+v", got)
	}
	if Pieces("", "a", "b") != nil {
		t.Error("빈 문구는 조각이 없어야 한다")
	}
}

// 새 기본 자리표시자는 {{매뉴얼제목}}이다. 이미 저장된 문서 모양을 위해 옛 이름 {{가이드제목}}도 같은 값으로 바꾼다(UI 설계 6절).
func TestPieces_TitlePlaceholders(t *testing.T) {
	got := Pieces("{{매뉴얼제목}}|{{가이드제목}}", "보고서", "")
	if want := []Piece{{Text: "보고서|보고서"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Pieces = %+v", got)
	}
	if h := DefaultOptions().Header; h != "{{매뉴얼제목}}" {
		t.Errorf("기본 머리글 %q, {{매뉴얼제목}}이어야 한다", h)
	}
}

func TestFileName(t *testing.T) {
	for title, want := range map[string]string{
		"주간 보고 쓰기":            "주간 보고 쓰기.pdf",
		`a<b>c:d"e/f\g|h?i*j`: "abcdefghij.pdf",
		"끝에 점과 공백. . ":        "끝에 점과 공백.pdf",
		"  앞 공백은 둔다":          "  앞 공백은 둔다.pdf",
		"줄\n바꿈\t탭":            "줄바꿈탭.pdf",
		"???":                 "매뉴얼.pdf",
		"con":                 "con_.pdf",
		"LPT1":                "LPT1_.pdf",
		"con.txt":             "con_.txt.pdf",  // 장치 이름 검사는 첫 점 앞부분만 보고, "_"는 그 뒤에 바로 붙인다(M3-C6)
		"LPT1.doc":            "LPT1_.doc.pdf", // 확장자가 있어도 장치 이름 바로 뒤에 "_"를 붙인다
		"aux":                 "aux_.pdf",      // 대소문자를 가리지 않는다
	} {
		if got := FileName(title, ".pdf"); got != want {
			t.Errorf("FileName(%q) = %q, want %q", title, got, want)
		}
	}
	long := ""
	for i := 0; i < 120; i++ {
		long += "가"
	}
	if got := FileName(long, ".hwpx"); got != long[:100*3]+".hwpx" {
		t.Errorf("긴 제목을 100자로 자르지 않았다: %d바이트", len(got))
	}
}

func TestOptions_Normalize(t *testing.T) {
	o := Options{Paper: "B5", ImageWidth: 500, Font: " <style> "}.Normalize()
	if o.Paper != "A4" || o.ImageWidth != 100 || o.Font != "" {
		t.Errorf("Normalize = %+v", o)
	}
	o = Options{Paper: "Letter", ImageWidth: 0, Font: "함초롬돋움"}.Normalize()
	if o.Paper != "Letter" || o.ImageWidth != 10 || o.Font != "함초롬돋움" {
		t.Errorf("Normalize = %+v", o)
	}
}
