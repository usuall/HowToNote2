// DOCX 렌더러 테스트: 필수 부품과 XML 형식, 이름 있는 스타일·목차 필드·updateFields·EMU 크기·안내 상자·PAGE 필드, 골든 파일, 경로 노출.
package export

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"howtonote/internal/hwpx"
)

func docxOut(t *testing.T, o Options) map[string][]byte {
	t.Helper()
	b, err := DOCX(sampleDoc(t), o, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return unzipAll(t, b)
}

// wellFormed는 XML을 끝까지 읽어 형식 오류가 없는지 본다.
func wellFormed(b []byte) error {
	d := xml.NewDecoder(bytes.NewReader(b))
	for {
		if _, err := d.Token(); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
	}
}

func TestDOCX_PartsAreWellFormed(t *testing.T) {
	parts := docxOut(t, goldenOptions())
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/_rels/document.xml.rels", "word/document.xml", "word/styles.xml",
		"word/settings.xml", "word/numbering.xml", "word/header1.xml", "word/footer1.xml", "docProps/core.xml", "docProps/app.xml"} {
		b, ok := parts[name]
		if !ok {
			t.Errorf("%s 없음", name)
			continue
		}
		if err := wellFormed(b); err != nil {
			t.Errorf("%s XML 오류: %v", name, err)
		}
	}
	if len(parts["word/media/image1.png"]) == 0 || len(parts["word/media/image2.png"]) == 0 {
		t.Error("그림 2개가 word/media에 없다")
	}
}

func TestDOCX_Rules(t *testing.T) {
	parts := docxOut(t, goldenOptions())
	doc, styles, settings := string(parts["word/document.xml"]), string(parts["word/styles.xml"]), string(parts["word/settings.xml"])
	for _, want := range []string{
		`<w:instrText xml:space="preserve"> TOC \o "2-3" \h \z \u </w:instrText>`, // Word 목차 필드
		`<w:pStyle w:val="StepTitle"/><w:pageBreakBefore/>`,                       // 1스텝 1쪽(둘째 최상위 스텝)
		`<wp:extent cx="6120130" cy="3442573"/>`,                                  // 1600px(15240000 EMU)는 A4 본문 폭 9638 twip(= 6120130 EMU)으로 줄인다
		`<w:shd w:val="clear" w:color="auto" w:fill="FEF7E0"/>`,                   // 주의 안내 상자 바탕
		`<w:hyperlink r:id="rId101">`,                                             // rId100은 앞선 첫 그림
		`<w:numId w:val="2"/>`,                                                    // 번호 목록(시작 3)
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document.xml에 %q 없음", want)
		}
	}
	for _, want := range []string{`<w:name w:val="heading 1"/>`, `<w:name w:val="스텝 제목"/>`, `<w:name w:val="본문"/>`, `<w:name w:val="안내 상자"/>`, `w:eastAsia="맑은 고딕"`} {
		if !strings.Contains(styles, want) {
			t.Errorf("styles.xml에 %q 없음", want)
		}
	}
	if !strings.Contains(settings, `<w:updateFields w:val="true"/>`) {
		t.Error("열 때 목차를 갱신하는 updateFields가 없다")
	}
	if !strings.Contains(string(parts["word/footer1.xml"]), `<w:fldSimple w:instr=" PAGE ">`) || !strings.Contains(string(parts["word/footer1.xml"]), `w:instr=" NUMPAGES "`) {
		t.Error("바닥글에 쪽 번호 필드가 없다")
	}
	if !strings.Contains(string(parts["word/numbering.xml"]), `<w:num w:numId="2"><w:abstractNumId w:val="1"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="3"/>`) {
		t.Error("번호 목록 시작 번호가 numbering.xml에 없다")
	}
	if strings.Contains(doc, "file:") || strings.Contains(string(parts["word/_rels/document.xml.rels"]), "file:") {
		t.Error("로컬 링크가 들어갔다")
	}
}

func TestDOCX_LandscapeLetterAndFont(t *testing.T) {
	o := DefaultOptions()
	o.Paper, o.Landscape, o.Font = "Letter", true, "함초롬돋움"
	parts := docxOut(t, o)
	if !strings.Contains(string(parts["word/document.xml"]), `<w:pgSz w:w="15840" w:h="12240" w:orient="landscape"/>`) {
		t.Error("Letter 가로 용지가 아니다")
	}
	if !strings.Contains(string(parts["word/styles.xml"]), `w:eastAsia="함초롬돋움"`) {
		t.Error("고른 글꼴이 기본 글꼴이 아니다")
	}
}

// 세로로 긴 그림(1000×3000)은 폭을 줄인 뒤에도 쪽 높이를 넘을 수 있다. 0.8 × 본문 높이로 다시 줄이고
// 가로세로 비율은 그대로 지킨다(검수 Important 1).
func TestDOCX_ImageHeightCapped(t *testing.T) {
	dir := t.TempDir()
	d := Doc{Title: "키 테스트", Steps: []Step{{ID: 1, Number: "1", Title: "크게",
		Image: &Image{Path: writePNG(t, dir, "tall.png", 1000, 3000), Width: 1000, Height: 3000}}}}
	b, err := DOCX(d, DefaultOptions(), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(unzipAll(t, b)["word/document.xml"])
	m := regexp.MustCompile(`<wp:extent cx="(\d+)" cy="(\d+)"/>`).FindStringSubmatch(doc)
	if m == nil {
		t.Fatal("wp:extent 없음")
	}
	cx, _ := strconv.Atoi(m[1])
	cy, _ := strconv.Atoi(m[2])
	pw, ph := docxPage(DefaultOptions())
	_ = pw
	capCy := (ph - 2*marginTBTwips) * emuPerTwip * 4 / 5
	if cy > capCy {
		t.Errorf("그림 높이 %d EMU, 한도 %d EMU를 넘었다", cy, capCy)
	}
	want, got := 1000.0/3000.0, float64(cx)/float64(cy)
	if diff := (got - want) / want; diff > 0.01 || diff < -0.01 {
		t.Errorf("가로세로 비율이 바뀌었다: %v -> %v", want, got)
	}
}

// DOCX·HWPX 안내 상자 색은 같은 값이어야 한다(검수 Minor 4). 둘을 하나로 합치지는 않는다(HTML은 html.tmpl의 CSS 글자 그대로).
func TestCalloutColors_MatchHWPX(t *testing.T) {
	for _, k := range []string{"info", "tip", "warning", "danger"} {
		face, line := calloutColors(k)
		want := hwpx.BoxColors[k]
		if face != want[0] || line != want[1] {
			t.Errorf("calloutColors(%q) = %q, %q인데 hwpx.BoxColors는 %v다", k, face, line, want)
		}
	}
}

func TestDOCX_Golden(t *testing.T) {
	parts := docxOut(t, goldenOptions())
	for _, name := range []string{"[Content_Types].xml", "word/document.xml", "word/_rels/document.xml.rels", "word/numbering.xml", "word/styles.xml", "word/header1.xml", "word/footer1.xml", "docProps/core.xml"} {
		golden(t, "docx/"+strings.NewReplacer("[", "", "]", "", "/", "_").Replace(name), parts[name])
	}
}

func TestDOCX_NoLocalPaths(t *testing.T) {
	d := sampleDoc(t)
	b, err := DOCX(d, goldenOptions(), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range unzipAll(t, b) {
		if l := leaks(data, forbidden(d)); l != nil {
			t.Errorf("%s에 %q", name, l)
		}
	}
}
