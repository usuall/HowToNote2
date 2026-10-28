// DOCX 렌더러(스펙 7.2절): OOXML 부품을 직접 써서 ZIP으로 묶는다. 이름 있는 스타일, Word 목차 필드(열 때 갱신),
// 그림 EMU 크기, 안내 상자는 색칠한 1×1 표, 머리글·바닥글의 PAGE·NUMPAGES 필드.
package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"
)

const (
	emuPerPx      = 9525 // 96dpi 기준 1px
	emuPerTwip    = 635
	marginTwips   = 1134 // 좌우 여백 2cm
	marginTBTwips = 1440 // 위아래 여백 1인치. <w:pgMar>와 같다
)

// docxPage는 용지의 세로 방향 (폭, 높이) twip이다.
func docxPage(o Options) (int, int) {
	if o.Paper == "Letter" {
		return 12240, 15840
	}
	return 11906, 16838
}

type docxWriter struct {
	b      strings.Builder
	o      Options
	rels   []string // word/_rels/document.xml.rels에 더할 관계(그림·링크)
	media  [][]byte
	starts []int // 번호 목록마다 시작 번호. numId는 2부터 차례로 준다
	textW  int   // 본문 폭(twip)
	maxCy  int   // 그림 높이 한도(EMU). 0이면 한도 없음
	pic    int
	first  bool // 아직 본문 첫 스텝 전
}

func xesc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (w *docxWriter) rel(typ, target string, external bool) string {
	id := fmt.Sprintf("rId%d", 100+len(w.rels))
	mode := ""
	if external {
		mode = ` TargetMode="External"`
	}
	w.rels = append(w.rels, fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/%s" Target="%s"%s/>`, id, typ, xesc(target), mode))
	return id
}

func (w *docxWriter) runs(rs []Run, bold bool) {
	for _, r := range rs {
		if r.Break {
			w.b.WriteString(`<w:r><w:br/></w:r>`)
			continue
		}
		var pr strings.Builder
		if r.Href != "" {
			pr.WriteString(`<w:rStyle w:val="Hyperlink"/>`)
		}
		if r.Bold || bold {
			pr.WriteString(`<w:b/>`)
		}
		if r.Italic {
			pr.WriteString(`<w:i/>`)
		}
		if r.Underline {
			pr.WriteString(`<w:u w:val="single"/>`)
		}
		run := fmt.Sprintf(`<w:r><w:rPr>%s</w:rPr><w:t xml:space="preserve">%s</w:t></w:r>`, pr.String(), xesc(r.Text))
		if r.Href != "" {
			run = fmt.Sprintf(`<w:hyperlink r:id="%s">%s</w:hyperlink>`, w.rel("hyperlink", r.Href, true), run)
		}
		w.b.WriteString(run)
	}
}

// para는 문단 하나다. ppr은 <w:pPr> 안에 들어갈 속성이다.
func (w *docxWriter) para(style, ppr string, rs []Run, bold bool) {
	fmt.Fprintf(&w.b, `<w:p><w:pPr><w:pStyle w:val="%s"/>%s</w:pPr>`, style, ppr)
	w.runs(rs, bold)
	w.b.WriteString(`</w:p>`)
}

func (w *docxWriter) blocks(bs []Block, depth int, style string) {
	for _, b := range bs {
		switch b.Kind {
		case KindPara:
			w.para(style, "", b.Runs, false)
		case KindHeading:
			w.para(fmt.Sprintf("DescHeading%d", b.Level), "", b.Runs, false)
		case KindList:
			numID := 1 // 글머리 목록은 하나를 같이 쓴다
			if b.Ordered {
				numID = 2 + len(w.starts)
				w.starts = append(w.starts, b.Start)
			}
			lvl := min(depth, 2)
			for _, item := range b.Items {
				for i, ib := range item {
					if ib.Kind == KindPara && i == 0 {
						w.para("ListParagraph", fmt.Sprintf(`<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, lvl, numID), ib.Runs, false)
						continue
					}
					if ib.Kind == KindPara {
						w.para("ListParagraph", fmt.Sprintf(`<w:ind w:left="%d"/>`, 720*(lvl+1)), ib.Runs, false)
						continue
					}
					w.blocks([]Block{ib}, depth+1, style)
				}
			}
		case KindTable:
			w.table(b.Rows)
		}
	}
}

func (w *docxWriter) table(rows [][]Cell) {
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	if cols == 0 {
		return
	}
	cw := w.textW / cols
	w.b.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/><w:tblW w:w="5000" w:type="pct"/></w:tblPr><w:tblGrid>`)
	for i := 0; i < cols; i++ {
		fmt.Fprintf(&w.b, `<w:gridCol w:w="%d"/>`, cw)
	}
	w.b.WriteString(`</w:tblGrid>`)
	for _, r := range rows {
		w.b.WriteString(`<w:tr>`)
		for i := 0; i < cols; i++ {
			c := Cell{}
			if i < len(r) {
				c = r[i]
			}
			shd := ""
			if c.Header {
				shd = `<w:shd w:val="clear" w:color="auto" w:fill="F1F3F4"/>`
			}
			fmt.Fprintf(&w.b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>%s</w:tcPr>`, cw, shd)
			if len(c.Blocks) == 0 {
				w.b.WriteString(`<w:p/>`) // 칸은 문단이 하나 이상 있어야 한다
			}
			for _, b := range c.Blocks {
				w.para("Body", "", b.Runs, c.Header)
			}
			w.b.WriteString(`</w:tc>`)
		}
		w.b.WriteString(`</w:tr>`)
	}
	w.b.WriteString(`</w:tbl><w:p><w:pPr><w:pStyle w:val="Spacer"/></w:pPr></w:p>`) // 표가 이어 붙지 않게 빈 문단을 둔다
}

// calloutColors는 안내 상자 종류별 (바탕, 테두리) 색이다. 편집기(Callouts.svelte)·HTML(html.tmpl)·HWPX(internal/hwpx)와 같은 값이다.
// HTML은 이 색을 Go 표가 아니라 html.tmpl의 CSS 글자 그대로 넣으므로(html.go 확인됨) 여기가 Go 코드로는 유일한 표다.
func calloutColors(kind string) (string, string) {
	switch kind {
	case "tip":
		return "#E6F4EA", "#188038"
	case "warning":
		return "#FEF7E0", "#F9AB00"
	case "danger":
		return "#FCE8E6", "#D93025"
	}
	return "#E8F0FE", "#1A73E8"
}

// callout은 안내 상자를 색칠한 1×1 표로 쓴다(HWPX와 같은 방식, 색은 편집기와 같다).
func (w *docxWriter) callout(c Callout) {
	face, line := calloutColors(c.Kind)
	fmt.Fprintf(&w.b, `<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/><w:tblBorders>`+
		`<w:top w:val="single" w:sz="4" w:color="%[1]s"/><w:left w:val="single" w:sz="24" w:color="%[1]s"/><w:bottom w:val="single" w:sz="4" w:color="%[1]s"/><w:right w:val="single" w:sz="4" w:color="%[1]s"/>`+
		`</w:tblBorders><w:tblCellMar><w:top w:w="80" w:type="dxa"/><w:left w:w="160" w:type="dxa"/><w:bottom w:w="80" w:type="dxa"/><w:right w:w="160" w:type="dxa"/></w:tblCellMar></w:tblPr>`+
		`<w:tblGrid><w:gridCol w:w="%[2]d"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:tcW w:w="%[2]d" w:type="dxa"/><w:shd w:val="clear" w:color="auto" w:fill="%[3]s"/></w:tcPr>`,
		strings.TrimPrefix(line, "#"), w.textW, strings.TrimPrefix(face, "#"))
	w.para("Callout", "", []Run{{Text: c.Heading()}}, true)
	w.blocks(c.Blocks, 0, "Callout")
	w.b.WriteString(`</w:tc></w:tr></w:tbl><w:p><w:pPr><w:pStyle w:val="Spacer"/></w:pPr></w:p>`)
}

func (w *docxWriter) image(img *Image) error {
	png, err := os.ReadFile(img.Path)
	if err != nil {
		return fmt.Errorf("스텝 그림 읽기: %w", err)
	}
	w.media = append(w.media, png)
	w.pic++
	id := w.rel("image", fmt.Sprintf("media/image%d.png", w.pic), false)
	cx, cy := img.Width*emuPerPx, img.Height*emuPerPx
	if limit := w.textW * emuPerTwip * w.o.ImageWidth / 100; cx > limit {
		cy = int(int64(cy) * int64(limit) / int64(cx))
		cx = limit
	}
	if w.maxCy > 0 && cy > w.maxCy { // 세로로 긴 그림이 쪽 높이를 넘지 않게, 비율은 유지한다(검수 Important 1)
		cx = int(int64(cx) * int64(w.maxCy) / int64(cy))
		cy = w.maxCy
	}
	fmt.Fprintf(&w.b, `<w:p><w:pPr><w:pStyle w:val="Picture"/></w:pPr><w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">`+
		`<wp:extent cx="%[1]d" cy="%[2]d"/><wp:docPr id="%[3]d" name="그림 %[3]d"/>`+
		`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="%[3]d" name="image%[3]d.png"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="%[4]s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%[1]d" cy="%[2]d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic>`+
		`</a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`, cx, cy, w.pic, id)
	return nil
}

const pageBreak = `<w:p><w:pPr><w:pStyle w:val="Spacer"/></w:pPr><w:r><w:br w:type="page"/></w:r></w:p>`

func (w *docxWriter) step(s Step) error {
	brk := ""
	if w.o.StepPerPage && !s.Sub && !w.first {
		if len(s.Before) > 0 {
			w.b.WriteString(pageBreak) // 표 안 문단의 pageBreakBefore는 Word가 무시한다
		} else {
			brk = `<w:pageBreakBefore/>`
		}
	}
	w.first = false
	for _, c := range s.Before {
		w.callout(c)
	}
	style := "StepTitle"
	if s.Sub {
		style = "SubStepTitle"
	}
	w.para(style, brk, []Run{{Text: s.Heading()}}, false)
	if s.Image != nil {
		if err := w.image(s.Image); err != nil {
			return err
		}
	}
	w.blocks(s.Blocks, 0, "Body")
	for _, c := range s.After {
		w.callout(c)
	}
	return nil
}

var docxTmpl = template.Must(template.New("").Funcs(template.FuncMap{"x": xesc, "add": func(a, b int) int { return a + b }}).Parse(docxParts))

type docxData struct {
	Title, Font, Created string
	Rels                 string
	Starts               []int
	Header, Footer       string
	Body                 string
}

// DOCX는 문서 모델을 .docx 바이트로 만든다.
func DOCX(d Doc, o Options, now time.Time) ([]byte, error) {
	o = o.Normalize()
	pw, ph := docxPage(o)
	if o.Landscape {
		pw, ph = ph, pw
	}
	// 0.8 × 본문 높이(위아래 여백을 뺀 쪽 높이): 세로로 긴 그림도 제목과 한 쪽에 들어가게 한다(검수 Important 1)
	maxCy := (ph - 2*marginTBTwips) * emuPerTwip * 4 / 5
	w := &docxWriter{o: o, textW: pw - 2*marginTwips, maxCy: maxCy, first: true}
	if o.Cover {
		w.para("Heading1", `<w:spacing w:before="4000"/>`, []Run{{Text: d.Title}}, false)
		w.para("Subtitle", "", []Run{{Text: d.Date}}, false)
		w.b.WriteString(pageBreak)
	} else {
		w.para("Heading1", "", []Run{{Text: d.Title}}, false)
	}
	if o.TOC && len(d.Steps) > 0 {
		w.para("TOCHeading", "", []Run{{Text: "목차"}}, false)
		// 목차는 Word가 채운다. settings.xml의 updateFields로 열 때 갱신을 묻는다.
		w.b.WriteString(`<w:p><w:r><w:fldChar w:fldCharType="begin" w:dirty="true"/></w:r><w:r><w:instrText xml:space="preserve"> TOC \o "2-3" \h \z \u </w:instrText></w:r>` +
			`<w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>목차를 보려면 Word에서 목차를 업데이트하세요.</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>`)
		w.b.WriteString(pageBreak)
	}
	for _, s := range d.Steps {
		if err := w.step(s); err != nil {
			return nil, err
		}
	}
	orient := ""
	if o.Landscape {
		orient = ` w:orient="landscape"`
	}
	fmt.Fprintf(&w.b, `<w:sectPr><w:headerReference w:type="default" r:id="rIdHeader"/><w:footerReference w:type="default" r:id="rIdFooter"/>`+
		`<w:pgSz w:w="%d" w:h="%d"%s/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr>`,
		pw, ph, orient, marginTBTwips, marginTwips, marginTBTwips, marginTwips)

	font := o.Font
	if font == "" {
		font = "맑은 고딕"
	}
	data := docxData{
		Title: d.Title, Font: font, Created: now.UTC().Format("2006-01-02T15:04:05Z"),
		Rels: strings.Join(w.rels, ""), Starts: w.starts,
		Header: hfXML(Pieces(o.Header, d.Title, d.Date)), Footer: hfXML(Pieces(o.Footer, d.Title, d.Date)),
		Body: w.b.String(),
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/_rels/document.xml.rels", "word/document.xml", "word/styles.xml",
		"word/settings.xml", "word/numbering.xml", "word/header1.xml", "word/footer1.xml", "docProps/core.xml", "docProps/app.xml"} {
		f, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if err := docxTmpl.ExecuteTemplate(f, name, data); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	for i, png := range w.media {
		f, err := zw.Create(fmt.Sprintf("word/media/image%d.png", i+1))
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(png); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// hfXML은 머리글·바닥글 조각을 runs로 쓴다. 쪽 번호는 Word 필드(PAGE, NUMPAGES)다.
func hfXML(ps []Piece) string {
	var b strings.Builder
	for _, p := range ps {
		switch p.Field {
		case PageField:
			b.WriteString(`<w:fldSimple w:instr=" PAGE "><w:r><w:t>1</w:t></w:r></w:fldSimple>`)
		case TotalField:
			b.WriteString(`<w:fldSimple w:instr=" NUMPAGES "><w:r><w:t>1</w:t></w:r></w:fldSimple>`)
		default:
			fmt.Fprintf(&b, `<w:r><w:t xml:space="preserve">%s</w:t></w:r>`, xesc(p.Text))
		}
	}
	return b.String()
}
