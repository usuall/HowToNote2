//go:build integration

// PDF 렌더러 통합 테스트(스펙 10절): 숨긴 WebView2로 고정 문서를 인쇄해 쪽 수, 글꼴 형식(Type3 없음, Pretendard·맑은 고딕 TrueType),
// 태그 PDF, 결과물 안 로컬 경로·가상 호스트 이름이 없는지(압축 푼 스트림까지) 본다.
package export

import (
	"bytes"
	"compress/zlib"
	"io"
	"os"
	"regexp"
	"testing"
)

// inflated는 PDF의 Flate 스트림을 모두 풀어 이어 붙인다. 링크·제목 같은 글이 압축 안에 있어도 검사하려는 것이다.
func inflated(pdf []byte) []byte {
	var out bytes.Buffer
	re := regexp.MustCompile(`(?s)stream\r?\n(.*?)endstream`)
	for _, m := range re.FindAllSubmatch(pdf, -1) {
		r, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue // 압축이 아닌 스트림(그림 등)
		}
		b, _ := io.ReadAll(r)
		out.Write(b)
	}
	return out.Bytes()
}

func TestPDF_PagesFontsAndNoLeaks(t *testing.T) {
	d := sampleDoc(t)
	pdf, err := PDF(d, goldenOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("PDF가 아니다")
	}
	// 표지 1 + 목차 1 + 스텝 1(그림·설명이 길어 1쪽 이상) + 스텝 2(1스텝 1쪽) ≥ 4쪽
	if n := len(regexp.MustCompile(`/Type\s*/Page\b`).FindAll(pdf, -1)); n < 4 {
		t.Errorf("쪽 수 = %d, 4쪽 이상이어야 한다", n)
	}
	if regexp.MustCompile(`/Subtype\s*/Type3`).Match(pdf) {
		t.Error("Type3 글꼴이 들어갔다(스펙 7.2절: 한글은 TrueType)")
	}
	for _, want := range []string{"/FontFile2", "Pretendard-Regular", "Pretendard-Bold", "MalgunGothic", "/StructTreeRoot"} {
		if !bytes.Contains(pdf, []byte(want)) {
			t.Errorf("PDF에 %q 없음", want)
		}
	}
	bad := append(forbidden(d), "howtonote.example", "index.html", "howtonote-print-")
	for name, b := range map[string][]byte{"PDF": pdf, "PDF 스트림": inflated(pdf)} {
		if l := leaks(b, bad); l != nil {
			t.Errorf("%s에 %q", name, l)
		}
	}
	_ = os.MkdirAll("../../out", 0o755)
	_ = os.WriteFile("../../out/export-m3.pdf", pdf, 0o644) // 머리글·바닥글·표지·목차는 눈으로 확인한다
}
