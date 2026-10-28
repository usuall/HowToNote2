// HWPX(한/글 OWPML) 문서를 템플릿 기반으로 만든다. 템플릿의 header.xml(글꼴·모양·스타일)에 우리 모양을 더하고,
// section0.xml은 첫 문단(쪽 설정 secPr)만 남기고 본문을 새로 쓴다(스펙 7.2절).
package hwpx

import (
	"archive/zip"
	"bytes"
	"cmp"
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

//go:embed template/blank.hwpx
var blankTemplate []byte

type Image struct {
	PNG      []byte
	WidthPx  int
	HeightPx int
}

// Style은 문단의 글자 모양이다. Body만 Run의 굵게·기울임·밑줄·링크를 따른다.
type Style int

const (
	Body Style = iota
	DocTitle
	StepTitle
	SubTitle
	Heading3
	Heading4
	Small
)

// Field는 머리말·꼬리말에 넣는 자동 번호다.
type Field int

const (
	NoField Field = iota
	PageNum
	TotalPages
)

// Run은 같은 모양의 글자 묶음이다. Break면 줄바꿈 하나, Field면 쪽 번호 하나이고 Text는 쓰지 않는다.
type Run struct {
	Text                    string
	Bold, Italic, Underline bool
	Href                    string
	Break                   bool
	Field                   Field
}

type Para struct {
	Style  Style
	Runs   []Run
	Indent int  // 목록 들여쓰기 단계(0~3)
	Center bool // 가운데 맞춤(표지)
}

type Cell struct {
	Header bool
	Paras  []Para
}

// Box는 안내 상자다. 1×1 표에 종류별 바탕색을 칠해 그린다(스펙 7.2절).
type Box struct {
	Kind  string // info, tip, warning, danger
	Paras []Para
}

// Block은 본문 한 덩어리다. Para, Image, Table, Box 중 하나만 채운다.
type Block struct {
	PageBreak bool // 이 덩어리 앞에서 쪽을 나눈다
	Para      *Para
	Image     *Image
	Table     [][]Cell
	Box       *Box
}

type Doc struct {
	Title          string
	Now            time.Time // 메타데이터 날짜. 테스트에서 고정하려고 밖에서 받는다
	Letter         bool      // false면 A4
	Landscape      bool
	Font           string // 비면 함초롬돋움
	ImagePct       int    // 그림 최대 폭(본문 폭의 %). 0이면 100
	Header, Footer []Run  // 비면 넣지 않는다
	Blocks         []Block
}

const (
	hwpUnitPerPx = 75   // 1인치 = 7200 HWPUNIT, 96dpi 기준 1px = 75
	marginLR     = 8504 // 템플릿 좌우 여백(30mm)
	marginTop    = 5668 // 템플릿 위 여백(20mm). buildSection은 이 값을 고치지 않는다(템플릿 그대로)
	marginBottom = 4252 // 템플릿 아래 여백(15mm)
	defaultFont  = "함초롬돋움"
)

// paper는 용지의 세로 방향 (폭, 높이)다. 가로 방향이어도 한/글은 세로 크기를 그대로 두고 landscape만 바꾼다.
func paper(letter bool) (int, int) {
	if letter {
		return 61200, 79200 // 8.5 × 11인치
	}
	return 59528, 84186 // A4 210 × 297mm
}

// TextWidth는 본문 폭(HWPUNIT)이다.
func (d Doc) TextWidth() int {
	w, h := paper(d.Letter)
	if d.Landscape {
		w = h
	}
	return w - 2*marginLR
}

// TextHeight는 본문 높이(HWPUNIT)다. TextWidth와 같은 식으로, 가로 방향이면 세로 쪽에 원래 폭을 쓴다.
func (d Doc) TextHeight() int {
	w, h := paper(d.Letter)
	if d.Landscape {
		h = w
	}
	return h - marginTop - marginBottom
}

func Write(d Doc) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(blankTemplate), int64(len(blankTemplate)))
	if err != nil {
		return nil, fmt.Errorf("템플릿 열기: %w", err)
	}
	var images []*Image
	for _, b := range d.Blocks {
		if b.Image != nil {
			images = append(images, b.Image)
		}
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// mimetype은 ZIP 첫 항목이고 무압축이어야 한/글이 HWPX로 인식한다.
	if err := writeEntry(zw, "mimetype", []byte("application/hwp+zip"), zip.Store); err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name == "mimetype" || strings.HasPrefix(f.Name, "BinData/") {
			continue
		}
		data, err := readEntry(f)
		if err != nil {
			return nil, err
		}
		switch f.Name {
		case "Contents/section0.xml":
			data, err = buildSection(data, d)
		case "Contents/header.xml":
			data, err = patchHeader(data, cmp.Or(d.Font, defaultFont))
		case "Contents/content.hpf":
			data = patchPackage(data, d, len(images))
		case "Preview/PrvText.txt":
			data = previewText(d)
		}
		if err != nil {
			return nil, err
		}
		if err := writeEntry(zw, f.Name, data, zip.Deflate); err != nil {
			return nil, err
		}
	}
	for i, img := range images {
		if err := writeEntry(zw, fmt.Sprintf("BinData/image%d.png", i+1), img.PNG, zip.Deflate); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func writeEntry(zw *zip.Writer, name string, data []byte, method uint16) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

var rePagePr = regexp.MustCompile(`<hp:pagePr landscape="[A-Z]+" width="\d+" height="\d+"`)

// buildSection은 템플릿 section0.xml의 첫 문단(secPr이 든 문단)만 남기고 본문을 새로 쓴다.
// 첫 문단에는 용지 크기·방향을 고치고 머리말·꼬리말 컨트롤을 단 너비 컨트롤(colPr) 뒤에 넣는다.
func buildSection(tpl []byte, d Doc) ([]byte, error) {
	s := string(tpl)
	sec := strings.Index(s, "<hp:secPr")
	if sec < 0 {
		return nil, errors.New("템플릿 section0.xml에 secPr이 없다")
	}
	colEnd := strings.Index(s[sec:], "</hp:ctrl>")
	end := strings.Index(s[sec:], "</hp:p>")
	if colEnd < 0 || end < 0 || colEnd > end {
		return nil, errors.New("템플릿 section0.xml의 첫 문단 모양이 예상과 다르다")
	}
	colEnd += sec + len("</hp:ctrl>")
	end += sec + len("</hp:p>")

	// 0.8 × 본문 높이: 세로로 긴 그림도 제목과 한 쪽에 들어가게 한다(검수 Important 1)
	w := &section{b: new(strings.Builder), id: 1000, textWidth: d.TextWidth(), imgPct: d.ImagePct, maxImgH: d.TextHeight() * 4 / 5}
	if w.imgPct <= 0 || w.imgPct > 100 {
		w.imgPct = 100
	}
	pw, ph := paper(d.Letter)
	orient := "WIDELY"
	if d.Landscape {
		orient = "NARROWLY"
	}
	head := rePagePr.ReplaceAllString(s[:colEnd], fmt.Sprintf(`<hp:pagePr landscape="%s" width="%d" height="%d"`, orient, pw, ph))
	w.b.WriteString(head)
	if len(d.Header) > 0 {
		w.b.WriteString(w.story("header", "TOP", d.Header))
	}
	if len(d.Footer) > 0 {
		w.b.WriteString(w.story("footer", "BOTTOM", d.Footer))
	}
	w.b.WriteString(s[colEnd:end])
	for _, blk := range d.Blocks {
		w.brk = w.brk || blk.PageBreak // 아무것도 쓰지 않은 앞 덩어리(빈 표)의 쪽 나눔을 잇는다
		switch {
		case blk.Image != nil:
			w.pic(blk.Image)
		case blk.Box != nil:
			fill := boxFill(blk.Box.Kind)
			w.table([][]Cell{{{Paras: blk.Box.Paras}}}, func(Cell) int { return fill })
		case blk.Table != nil:
			w.table(blk.Table, func(c Cell) int {
				if c.Header {
					return fillHeader
				}
				return fillTable
			})
		case blk.Para != nil:
			w.para(*blk.Para)
		}
	}
	w.b.WriteString("</hs:sec>")
	return []byte(w.b.String()), nil
}

var (
	reTitle  = regexp.MustCompile(`<opf:title\s*/>|<opf:title>[^<]*</opf:title>`)
	reAuthor = regexp.MustCompile(`(<opf:meta name="(?:creator|lastsaveby)" content="text">)[^<]*(</opf:meta>)`)
	reStamp  = regexp.MustCompile(`(<opf:meta name="(?:CreatedDate|ModifiedDate)" content="text">)[^<]*(</opf:meta>)`)
	reDate   = regexp.MustCompile(`(<opf:meta name="date" content="text">)[^<]*(</opf:meta>)`)
)

// patchPackage는 content.hpf의 메타데이터를 이 문서 것으로 바꾸고 그림을 manifest에 등록한다.
// 템플릿의 작성자·날짜가 결과물에 남지 않게 하는 것이 목적이다.
func patchPackage(hpf []byte, d Doc, nImages int) []byte {
	s := string(hpf)
	s = reTitle.ReplaceAllLiteralString(s, "<opf:title>"+esc(d.Title)+"</opf:title>")
	s = reAuthor.ReplaceAllString(s, "${1}HowtoNote${2}")
	s = reStamp.ReplaceAllString(s, "${1}"+d.Now.UTC().Format("2006-01-02T15:04:05Z")+"${2}")
	s = reDate.ReplaceAllString(s, "${1}"+d.Now.Format("2006-01-02 15:04:05")+"${2}")
	var items strings.Builder
	for i := 1; i <= nImages; i++ {
		fmt.Fprintf(&items, `<opf:item id="image%d" href="BinData/image%d.png" media-type="image/png" isEmbeded="1"/>`, i, i)
	}
	return []byte(strings.Replace(s, "</opf:manifest>", items.String()+"</opf:manifest>", 1))
}

// previewText는 탐색기·한/글 미리 보기용 글(Preview/PrvText.txt, UTF-8)이다. 제목과 문단 글을 1KB 안에서 담는다.
// 템플릿 것을 그대로 두면 다른 문서의 글이 미리 보기에 남을 수 있어 늘 새로 만든다.
func previewText(d Doc) []byte {
	const limit = 1024
	var b strings.Builder
	add := func(line string) bool {
		if b.Len()+len(line)+2 > limit {
			return false
		}
		b.WriteString(line)
		b.WriteString("\r\n")
		return true
	}
	if !add(d.Title) {
		return []byte(truncate(d.Title, limit))
	}
	for _, blk := range d.Blocks {
		if blk.Para == nil || blk.Para.Style == DocTitle { // 제목은 맨 위에 이미 썼다
			continue
		}
		var line strings.Builder
		for _, r := range blk.Para.Runs {
			if r.Break {
				line.WriteString(" ")
			}
			line.WriteString(r.Text)
		}
		if line.Len() > 0 && !add(line.String()) {
			break
		}
	}
	return []byte(b.String())
}

// truncate는 s를 n바이트 안에서 글자 경계로 자른다.
func truncate(s string, n int) string {
	for len(s) > n {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
