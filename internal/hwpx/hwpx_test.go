// HWPX 작성기 단위 테스트: ZIP 구조, 글꼴·모양 추가, 쪽 설정, 머리말·꼬리말 쪽 번호, 줄바꿈, 링크, 표·안내 상자, 그림, 메타데이터, 미리 보기 글.
package hwpx

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func samplePNG(t *testing.T, w, h int) *Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &Image{PNG: buf.Bytes(), WidthPx: w, HeightPx: h}
}

func body(rs ...Run) Block { return Block{Para: &Para{Runs: rs}} }

// sampleDoc은 작성기가 쓰는 모든 덩어리를 한 번씩 담는다.
func sampleDoc(t *testing.T) Doc {
	return Doc{
		Title:  "테스트 <가이드>",
		Now:    time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC),
		Header: []Run{{Text: "테스트 <가이드>"}},
		Footer: []Run{{Field: PageNum}, {Text: " / "}, {Field: TotalPages}},
		Blocks: []Block{
			{Para: &Para{Style: DocTitle, Center: true, Runs: []Run{{Text: "테스트 <가이드>"}}}},
			{PageBreak: true, Para: &Para{Style: StepTitle, Runs: []Run{{Text: "1. '저장' 버튼을 클릭합니다."}}}},
			{Image: samplePNG(t, 200, 100)},
			body(Run{Text: "보통 "}, Run{Text: "굵게", Bold: true}, Run{Text: "기울임", Italic: true}, Run{Text: "밑줄", Underline: true},
				Run{Text: "첫 줄"}, Run{Break: true}, Run{Text: "둘째 줄"}, Run{Text: "링크", Href: "https://example.com/a?b=1&c=2#x"}),
			{Para: &Para{Style: Heading3, Runs: []Run{{Text: "소제목"}}}},
			{Para: &Para{Indent: 1, Runs: []Run{{Text: "• 항목"}}}},
			{Table: [][]Cell{{{Header: true, Paras: []Para{{Runs: []Run{{Text: "이름"}}}}}, {Header: true}}, {{Paras: []Para{{Runs: []Run{{Text: "값"}}}}}}}},
			{Box: &Box{Kind: "warning", Paras: []Para{{Runs: []Run{{Text: "주의: <A&B> 확인", Bold: true}}}}}},
			{Para: &Para{Style: SubTitle, Runs: []Run{{Text: "1.1 하위 스텝"}}}},
		},
	}
}

type entry struct {
	name   string
	method uint16
	data   []byte
}

func unzip(t *testing.T, b []byte) []entry {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var out []entry
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		out = append(out, entry{f.Name, f.Method, data})
	}
	return out
}

func find(es []entry, name string) string {
	for _, e := range es {
		if e.name == name {
			return string(e.data)
		}
	}
	return ""
}

func write(t *testing.T, d Doc) []entry {
	t.Helper()
	b, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	return unzip(t, b)
}

func TestWrite_MimetypeFirstAndStored(t *testing.T) {
	es := write(t, sampleDoc(t))
	if es[0].name != "mimetype" || es[0].method != zip.Store || string(es[0].data) != "application/hwp+zip" {
		t.Fatalf("첫 항목 = %q(method %d) %q", es[0].name, es[0].method, es[0].data)
	}
}

func TestWrite_SectionContent(t *testing.T) {
	sec := find(write(t, sampleDoc(t)), "Contents/section0.xml")
	for _, want := range []string{
		`<hp:pagePr landscape="WIDELY" width="59528" height="84186"`, // A4 세로
		`<hp:t>1. &#39;저장&#39; 버튼을 클릭합니다.</hp:t>`,
		`pageBreak="1"`,
		`<hp:t>첫 줄</hp:t></hp:run><hp:run charPrIDRef="7"><hp:t><hp:lineBreak/></hp:t>`, // 줄바꿈은 &#xA;가 아니라 lineBreak
		`<hp:run charPrIDRef="8"><hp:t>굵게</hp:t>`,                                       // 굵게 = 본문 7 + 1
		`<hp:run charPrIDRef="9"><hp:t>기울임</hp:t>`,
		`<hp:run charPrIDRef="11"><hp:t>밑줄</hp:t>`,
		`type="HYPERLINK" name="https://example.com/a?b=1&amp;c=2#x"`,
		`<hp:stringParam name="Command">https\://example.com/a\?b=1&amp;c=2\#x;1;0;0;</hp:stringParam>`,
		`<hp:run charPrIDRef="15"><hp:t>링크</hp:t>`, // 링크 글자 모양
		`<hp:t>주의: &lt;A&amp;B&gt; 확인</hp:t>`,
		`borderFillIDRef="7"`, // warning 안내 상자 색(fillBox + 2)
		`borderFillIDRef="4"`, // 표 머리 칸
		`<hp:autoNum num="1" numType="PAGE">`,
		`<hp:autoNum num="1" numType="TOTAL_PAGE">`,
		`<hp:header id=`, `<hp:footer id=`,
		"</hs:sec>",
	} {
		if !strings.Contains(sec, want) {
			t.Errorf("section0.xml에 %q 없음", want)
		}
	}
	if strings.Contains(sec, "&#xA;") {
		t.Error("줄바꿈이 &#xA;로 들어갔다")
	}
	if strings.Count(sec, "<hp:secPr") != 1 || strings.Index(sec, "<hp:header") < strings.Index(sec, "</hp:secPr>") {
		t.Error("머리말은 secPr 문단의 컨트롤로 secPr 뒤에 한 번만 있어야 한다")
	}
	if strings.Count(sec, "<hp:tbl ") != 2 {
		t.Errorf("표는 표 하나와 안내 상자 하나여야 한다: %d개", strings.Count(sec, "<hp:tbl "))
	}
}

func TestWrite_PaperAndOrientation(t *testing.T) {
	d := sampleDoc(t)
	d.Letter, d.Landscape, d.ImagePct = true, true, 50
	sec := find(write(t, d), "Contents/section0.xml")
	if !strings.Contains(sec, `<hp:pagePr landscape="NARROWLY" width="61200" height="79200"`) {
		t.Error("Letter 가로가 아니다(한/글은 세로 크기를 두고 landscape만 바꾼다)")
	}
	// 가로 Letter 본문 폭 = 79200 - 2*8504 = 62192, 50%면 31096. 그림 200px = 15000이라 그대로다.
	if !strings.Contains(sec, `<hp:sz width="15000" height="7500"`) {
		t.Error("그림 크기가 틀렸다")
	}
}

func TestFitSize(t *testing.T) {
	cases := []struct{ w, h, tw, pct, maxH, ww, wh int }{
		{200, 100, 42520, 100, 0, 15000, 7500},       // 1px = 75 HWPUNIT
		{1000, 500, 42520, 100, 0, 42520, 21260},     // 본문 폭 넘으면 비율 유지 축소
		{1000, 500, 42520, 50, 0, 21260, 10630},      // 그림 최대 폭 50%
		{1000, 3000, 42520, 100, 10000, 3333, 10000}, // 세로로 긴 그림은 높이 한도로 다시 줄인다(검수 Important 1)
	}
	for _, c := range cases {
		if w, h := fitSize(c.w, c.h, c.tw, c.pct, c.maxH); w != c.ww || h != c.wh {
			t.Errorf("fitSize(%d,%d,%d,%d,%d) = %d,%d, want %d,%d", c.w, c.h, c.tw, c.pct, c.maxH, w, h, c.ww, c.wh)
		}
	}
}

func TestWrite_HeaderFontsAndStyles(t *testing.T) {
	hdr := find(write(t, sampleDoc(t)), "Contents/header.xml")
	for _, want := range []string{
		`face="함초롬돋움" type="TTF" isEmbedded="0"><hh:substFont face="맑은 고딕"`,
		`<hh:charProperties itemCnt="22">`, `<hh:paraProperties itemCnt="25">`, `<hh:borderFills itemCnt="8">`,
		`<hh:charPr id="8" height="1000" textColor="#000000"`, `<hh:bold/>`, `<hh:italic/>`, `<hh:underline type="BOTTOM"`,
		`faceColor="#FEF7E0"`,
	} {
		if !strings.Contains(hdr, want) {
			t.Errorf("header.xml에 %q 없음", want)
		}
	}
	if strings.Contains(hdr, "함초롬바탕") {
		t.Error("본문 글꼴이 함초롬바탕으로 남았다")
	}
	d := sampleDoc(t)
	d.Font = "맑은 고딕"
	hdr = find(write(t, d), "Contents/header.xml")
	if !strings.Contains(hdr, `face="맑은 고딕" type="TTF" isEmbedded="0"><hh:typeInfo`) || strings.Contains(hdr, "함초롬") {
		t.Error("글꼴을 맑은 고딕으로 고르면 대체 글꼴 없이 맑은 고딕이어야 한다")
	}
}

func TestWrite_RegistersImages(t *testing.T) {
	d := sampleDoc(t)
	es := write(t, d)
	if find(es, "BinData/image1.png") != string(d.Blocks[2].Image.PNG) {
		t.Error("BinData/image1.png 내용이 원본과 다르다")
	}
	if !strings.Contains(find(es, "Contents/content.hpf"), `<opf:item id="image1" href="BinData/image1.png" media-type="image/png" isEmbeded="1"/>`) {
		t.Error("content.hpf에 그림 항목이 없다")
	}
	if !strings.Contains(find(es, "Contents/section0.xml"), `binaryItemIDRef="image1"`) {
		t.Error("section0.xml의 그림 참조가 없다")
	}
}

func TestWrite_ReplacesFixtureMetadataAndPreview(t *testing.T) {
	es := write(t, sampleDoc(t))
	hpf := find(es, "Contents/content.hpf")
	if strings.Contains(hpf, "synthetic-fixture-author") {
		t.Error("템플릿 작성자 메타데이터가 남았다")
	}
	for _, want := range []string{"<opf:title>테스트 &lt;가이드&gt;</opf:title>", ">HowtoNote</opf:meta>", ">2026-10-20T09:30:00Z</opf:meta>"} {
		if !strings.Contains(hpf, want) {
			t.Errorf("content.hpf에 %q 없음", want)
		}
	}
	prv := find(es, "Preview/PrvText.txt")
	if !strings.HasPrefix(prv, "테스트 <가이드>\r\n") || !strings.Contains(prv, "1. '저장' 버튼을 클릭합니다.\r\n") {
		t.Errorf("미리 보기 글 = %q", prv)
	}
}

func TestPreviewText_FitsIn1KB(t *testing.T) {
	p := previewText(Doc{Title: strings.Repeat("가", 400)}) // 1200바이트 제목
	if len(p) > 1024 || !strings.HasPrefix(string(p), "가가") || !utf8.Valid(p) {
		t.Errorf("미리 보기 글 %d바이트", len(p))
	}
}

func TestTemplate_Counts(t *testing.T) {
	hdr := find(unzip(t, blankTemplate), "Contents/header.xml")
	for _, want := range []string{`<hh:charProperties itemCnt="7">`, `<hh:paraProperties itemCnt="20">`, `<hh:borderFills itemCnt="3">`, `<hh:borderFill id="3"`} {
		if !strings.Contains(hdr, want) {
			t.Fatalf("템플릿 header.xml에 %q가 없다. tools/make_hwpx_template.py로 다시 만들었다면 header.go의 tpl*Count를 맞춘다", want)
		}
	}
}

// 미리 보기 글은 제목을 맨 위에 한 번만 쓴다. 본문의 문서 제목 문단은 건너뛴다(M3-C15(d)).
func TestPreviewText_TitleOnce(t *testing.T) {
	prv := find(write(t, sampleDoc(t)), "Preview/PrvText.txt")
	if n := strings.Count(prv, "테스트 <가이드>"); n != 1 {
		t.Errorf("미리 보기 글에 제목이 %d번: %q", n, prv)
	}
}

// 아무것도 쓰지 않는 덩어리(빈 표)에 단 쪽 나눔은 다음 덩어리로 넘어가야 한다(Task 7 검수).
func TestWrite_PageBreakSurvivesEmptyBlock(t *testing.T) {
	d := Doc{Blocks: []Block{body(Run{Text: "앞"}), {PageBreak: true, Table: [][]Cell{{}}}, body(Run{Text: "뒤"})}}
	sec := find(write(t, d), "Contents/section0.xml")
	if !strings.Contains(sec, `pageBreak="1" columnBreak="0" merged="0"><hp:run charPrIDRef="7"><hp:t>뒤</hp:t>`) {
		t.Error("빈 표의 쪽 나눔이 사라졌다")
	}
}
