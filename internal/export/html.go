// HTML 렌더러(스펙 7.2절): html/template로 단일 파일 HTML을 만든다. PDF도 같은 템플릿을 인쇄용 설정으로 쓴다.
// 단일 파일은 그림을 base64로 넣고, 인쇄용은 그림·글꼴을 같은 폴더의 파일로 가리킨다(인쇄 WebView2의 가상 호스트).
package export

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"os"
)

//go:embed html.tmpl
var htmlSource string

//go:embed fonts/Pretendard-Regular.ttf
var fontRegular []byte

//go:embed fonts/Pretendard-Bold.ttf
var fontBold []byte

// Font는 내보내기에 넣어 둔 Pretendard TTF다. 앱 화면도 같은 바이트를 /fonts/로 받아 써서, 실행 파일에 글꼴을 두 번 넣지 않는다.
func Font(bold bool) []byte {
	if bold {
		return fontBold
	}
	return fontRegular
}

var htmlTmpl = template.Must(template.New("html.tmpl").Parse(htmlSource))

// htmlPage는 템플릿에 넘기는 값이다. Images는 스텝 순서대로 그림 주소(data: 또는 상대 경로)다.
type htmlPage struct {
	Doc            Doc
	O              Options
	Print          bool // 인쇄용(PDF): @page 규칙, 쪽 나눔
	Font           template.CSS
	FontFaces      template.CSS // @font-face 규칙. 비면 시스템 글꼴만 쓴다
	Images         map[int64]template.URL
	ImageWidth     int
	PageSize       template.CSS
	ImageMaxHeight template.CSS // 인쇄용 그림 높이 한도(mm). Print가 아니면 쓰지 않는다
}

// fontStack은 본문 글꼴 목록이다. 고른 글꼴 → (PDF·글꼴 포함이면) Pretendard → 맑은 고딕 순으로 찾는다.
// 맑은 고딕을 꼭 넣는 이유: Pretendard에 없는 글자가 TTC 글꼴(굴림 등)로 가면 PDF에 Type3로 들어간다.
func fontStack(o Options, pretendard bool) template.CSS {
	s := ""
	if o.Font != "" {
		s += fmt.Sprintf("%q, ", o.Font)
	}
	if pretendard {
		s += `"Pretendard", `
	}
	return template.CSS(s + `"Malgun Gothic", "맑은 고딕", sans-serif`)
}

func fontFaces(regular, bold string) template.CSS {
	return template.CSS(fmt.Sprintf(`@font-face{font-family:"Pretendard";font-weight:400;src:url(%q) format("truetype")}`+
		`@font-face{font-family:"Pretendard";font-weight:700;src:url(%q) format("truetype")}`, regular, bold))
}

func pageSize(o Options) template.CSS {
	s := "A4"
	if o.Paper == "Letter" {
		s = "letter"
	}
	if o.Landscape {
		return template.CSS(s + " landscape")
	}
	return template.CSS(s + " portrait")
}

// printMarginMM은 @page 규칙(html.tmpl)의 위아래 여백과 같다.
const printMarginMM = 20.0

// imageMaxHeightMM은 인쇄용 그림 높이 한도다: 0.8 × 본문 높이(쪽 높이에서 위아래 여백을 뺀 값), mm 단위.
// 세로로 긴 그림이 쪽을 넘어가 Chromium이 둘로 쪼개지 않게 한다(검수 Important 1).
func imageMaxHeightMM(o Options) template.CSS {
	h, w := 297.0, 210.0 // A4
	if o.Paper == "Letter" {
		h, w = 279.4, 215.9 // 11 × 8.5인치
	}
	if o.Landscape {
		h, w = w, h
	}
	cap := (h - 2*printMarginMM) * 0.8
	return template.CSS(fmt.Sprintf("%.2fmm", cap))
}

// HTML은 단일 파일 HTML이다. 그림은 base64로 넣고, 글꼴은 EmbedFont일 때만 Pretendard를 넣는다(기본은 시스템 글꼴).
func HTML(d Doc, o Options) ([]byte, error) {
	o = o.Normalize()
	p := htmlPage{Doc: d, O: o, Font: fontStack(o, o.EmbedFont), Images: map[int64]template.URL{}, ImageWidth: o.ImageWidth, PageSize: pageSize(o)}
	if o.EmbedFont {
		p.FontFaces = fontFaces("data:font/ttf;base64,"+base64.StdEncoding.EncodeToString(fontRegular),
			"data:font/ttf;base64,"+base64.StdEncoding.EncodeToString(fontBold))
	}
	for _, s := range d.Steps {
		if s.Image == nil {
			continue
		}
		b, err := os.ReadFile(s.Image.Path)
		if err != nil {
			return nil, fmt.Errorf("스텝 그림 읽기: %w", err)
		}
		p.Images[s.ID] = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(b))
	}
	return execHTML(p)
}

// printHTML은 PDF용 HTML이다. 그림은 img/<스텝ID>.png, 글꼴은 Pretendard 파일을 가리킨다(같은 폴더에 둔다).
func printHTML(d Doc, o Options) ([]byte, error) {
	p := htmlPage{Doc: d, O: o, Print: true, Font: fontStack(o, true), Images: map[int64]template.URL{}, ImageWidth: o.ImageWidth, PageSize: pageSize(o),
		ImageMaxHeight: imageMaxHeightMM(o), FontFaces: fontFaces("Pretendard-Regular.ttf", "Pretendard-Bold.ttf")}
	for _, s := range d.Steps {
		if s.Image != nil {
			p.Images[s.ID] = template.URL(fmt.Sprintf("img/%d.png", s.ID))
		}
	}
	return execHTML(p)
}

func execHTML(p htmlPage) ([]byte, error) {
	var buf bytes.Buffer
	if err := htmlTmpl.Execute(&buf, p); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
