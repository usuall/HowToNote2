// HWPX section0.xml 조각: 문단·글자 묶음·그림·표·안내 상자·머리말·꼬리말. 구조는 python-hwpx가 만드는 한/글 출력과 같다.
package hwpx

import (
	"fmt"
	"strings"
)

// section은 section0.xml 본문을 쓰는 동안의 상태다. 문단·개체 id는 문서 안에서 겹치지 않으면 된다.
type section struct {
	b         *strings.Builder
	id        int
	img       int
	textWidth int
	imgPct    int
	maxImgH   int  // 그림 높이 한도(HWPUNIT). 0이면 한도 없음
	brk       bool // 다음에 쓰는 바깥 문단 하나에 쪽 나눔을 단다
}

// open은 본문 문단 시작 태그다. 쪽 나눔은 한 번만 쓰고 끈다(표 안 문단에는 달지 않는다).
func (w *section) open(paraPr int) {
	brk := 0
	if w.brk {
		brk, w.brk = 1, false
	}
	fmt.Fprintf(w.b, `<hp:p id="%d" paraPrIDRef="%d" styleIDRef="0" pageBreak="%d" columnBreak="0" merged="0">`, w.next(), paraPr, brk)
}

func (w *section) next() int {
	w.id++
	return w.id
}

func charOf(st Style, r Run) int {
	switch st {
	case DocTitle:
		return charDocTitle
	case StepTitle:
		return charStep
	case SubTitle:
		return charSub
	case Heading3:
		return charH3
	case Heading4:
		return charH4
	case Small:
		return charSmall
	}
	if r.Href != "" {
		return charLink
	}
	n := 0
	if r.Bold {
		n |= 1
	}
	if r.Italic {
		n |= 2
	}
	if r.Underline {
		n |= 4
	}
	return charBody + n
}

func paraOf(p Para) int {
	switch {
	case p.Center:
		return paraCenter
	case p.Style == StepTitle || p.Style == SubTitle:
		return paraStep
	case p.Indent > 0:
		return paraIndent + min(p.Indent, 3) - 1
	}
	return paraBody
}

const autoNumFmt = `<hp:autoNumFormat type="DIGIT" userChar="" prefixChar="" suffixChar="" supscript="0"/>`

// runs는 글자 묶음을 쓴다. 링크는 한/글 하이퍼링크 필드(fieldBegin·fieldEnd) 사이에 둔다.
func (w *section) runs(st Style, rs []Run) {
	if len(rs) == 0 {
		fmt.Fprintf(w.b, `<hp:run charPrIDRef="%d"><hp:t/></hp:run>`, charOf(st, Run{}))
		return
	}
	for _, r := range rs {
		c := charOf(st, r)
		switch {
		case r.Field == PageNum:
			fmt.Fprintf(w.b, `<hp:run charPrIDRef="%d"><hp:ctrl><hp:autoNum num="1" numType="PAGE">%s</hp:autoNum></hp:ctrl></hp:run>`, c, autoNumFmt)
		case r.Field == TotalPages:
			fmt.Fprintf(w.b, `<hp:run charPrIDRef="%d"><hp:ctrl><hp:autoNum num="1" numType="TOTAL_PAGE">%s</hp:autoNum></hp:ctrl></hp:run>`, c, autoNumFmt)
		case r.Break:
			fmt.Fprintf(w.b, `<hp:run charPrIDRef="%d"><hp:t><hp:lineBreak/></hp:t></hp:run>`, c)
		case r.Href != "":
			id := w.next()
			fmt.Fprintf(w.b, `<hp:run charPrIDRef="%d"><hp:ctrl>%s</hp:ctrl></hp:run>`, charBody, linkBegin(id, r.Href))
			fmt.Fprintf(w.b, `<hp:run charPrIDRef="%d"><hp:t>%s</hp:t></hp:run>`, c, esc(r.Text))
			fmt.Fprintf(w.b, `<hp:run charPrIDRef="%d"><hp:ctrl><hp:fieldEnd beginIDRef="%d" fieldid="%s"/></hp:ctrl></hp:run>`, charBody, id, linkFieldID)
		default:
			fmt.Fprintf(w.b, `<hp:run charPrIDRef="%d"><hp:t>%s</hp:t></hp:run>`, c, esc(r.Text))
		}
	}
}

// linkFieldID는 한/글이 모든 하이퍼링크 필드에 주는 fieldid다(python-hwpx hyperlink_form.py).
const linkFieldID = "627600491"

// linkBegin은 하이퍼링크 필드 시작이다. Command 값은 : ? ; #를 \로 막는다(한/글 형식).
func linkBegin(id int, href string) string {
	cmd := href + ";2;0;0"
	category := "HWPHYPERLINK_TYPE_EMAIL"
	if !strings.HasPrefix(strings.ToLower(href), "mailto:") {
		var e strings.Builder
		for _, r := range href {
			if strings.ContainsRune(":?;#", r) {
				e.WriteByte('\\')
			}
			e.WriteRune(r)
		}
		cmd, category = e.String()+";1;0;0;", "HWPHYPERLINK_TYPE_URL"
	}
	return fmt.Sprintf(`<hp:fieldBegin id="%d" type="HYPERLINK" name="%s" editable="0" dirty="0" zorder="-1" fieldid="%s">`+
		`<hp:parameters cnt="6" name=""><hp:integerParam name="Prop">0</hp:integerParam><hp:stringParam name="Command">%s</hp:stringParam>`+
		`<hp:stringParam name="Path">%s</hp:stringParam><hp:stringParam name="Category">%s</hp:stringParam>`+
		`<hp:stringParam name="TargetType">HWPHYPERLINK_TARGET_BOOKMARK</hp:stringParam><hp:stringParam name="DocOpenType">HWPHYPERLINK_JUMP_CURRENTTAB</hp:stringParam></hp:parameters></hp:fieldBegin>`,
		id, esc(href), linkFieldID, esc(cmd), esc(href), category)
}

func (w *section) para(p Para) {
	w.open(paraOf(p))
	w.runs(p.Style, p.Runs)
	w.b.WriteString(`</hp:p>`)
}

// fitSize는 px 크기를 HWPUNIT으로 바꾸고, 본문 폭의 pct%를 넘거나 maxH(0이면 한도 없음)를 넘으면
// 비율을 유지해 줄인다(검수 Important 1: 세로로 긴 그림이 쪽을 넘지 않게).
func fitSize(wPx, hPx, textWidth, pct, maxH int) (int, int) {
	limit := textWidth * pct / 100
	w, h := wPx*hwpUnitPerPx, hPx*hwpUnitPerPx
	if w > limit {
		h = h * limit / w
		w = limit
	}
	if maxH > 0 && h > maxH {
		w = w * maxH / h
		h = maxH
	}
	return w, h
}

func (w *section) pic(img *Image) {
	w.img++
	wd, ht := fitSize(img.WidthPx, img.HeightPx, w.textWidth, w.imgPct, w.maxImgH)
	obj := w.next()
	w.open(paraCenter)
	fmt.Fprintf(w.b, `<hp:run charPrIDRef="%d">`, charBody)
	fmt.Fprintf(w.b, `<hp:pic textWrap="SQUARE" textFlow="BOTH_SIDES" reverse="0" id="%[1]d" zOrder="0" numberingType="PICTURE" lock="0" dropcapstyle="None" href="" groupLevel="0" instid="%[1]d">`+
		`<hp:offset x="0" y="0"/><hp:orgSz width="%[2]d" height="%[3]d"/><hp:curSz width="%[2]d" height="%[3]d"/><hp:flip horizontal="0" vertical="0"/>`+
		`<hp:rotationInfo angle="0" centerX="%[4]d" centerY="%[5]d" rotateimage="1"/>`+
		`<hp:renderingInfo><hc:transMatrix e1="1" e2="0" e3="0" e4="0" e5="1" e6="0"/><hc:scaMatrix e1="1" e2="0" e3="0" e4="0" e5="1" e6="0"/><hc:rotMatrix e1="1" e2="0" e3="0" e4="0" e5="1" e6="0"/></hp:renderingInfo>`+
		`<hp:imgRect><hc:pt0 x="0" y="0"/><hc:pt1 x="%[2]d" y="0"/><hc:pt2 x="%[2]d" y="%[3]d"/><hc:pt3 x="0" y="%[3]d"/></hp:imgRect>`+
		`<hp:imgClip left="0" right="%[2]d" top="0" bottom="%[3]d"/><hp:inMargin left="0" right="0" top="0" bottom="0"/><hp:imgDim dimwidth="%[2]d" dimheight="%[3]d"/>`+
		`<hc:img binaryItemIDRef="image%[6]d" bright="0" contrast="0" effect="REAL_PIC" alpha="0"/><hp:effects/>`+
		`<hp:sz width="%[2]d" height="%[3]d" widthRelTo="ABSOLUTE" heightRelTo="ABSOLUTE" protect="0"/>`+
		`<hp:pos treatAsChar="1" affectLSpacing="0" flowWithText="1" allowOverlap="0" holdAnchorAndSO="0" vertRelTo="PARA" horzRelTo="COLUMN" vertAlign="TOP" horzAlign="LEFT" vertOffset="0" horzOffset="0"/>`+
		`<hp:outMargin left="0" right="0" top="0" bottom="0"/><hp:shapeComment/></hp:pic></hp:run></hp:p>`,
		obj, wd, ht, wd/2, ht/2, w.img)
}

// table은 rows×cols 표를 쓴다. fill은 칸마다의 테두리 채우기 번호(머리 칸은 회색, 안내 상자는 색).
func (w *section) table(rows [][]Cell, fillOf func(Cell) int) {
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	if cols == 0 {
		return
	}
	cw := w.textWidth / cols
	w.open(paraBody)
	fmt.Fprintf(w.b, `<hp:run charPrIDRef="%d">`, charBody)
	fmt.Fprintf(w.b, `<hp:tbl id="%d" zOrder="0" numberingType="TABLE" textWrap="TOP_AND_BOTTOM" textFlow="BOTH_SIDES" lock="0" dropcapstyle="None" pageBreak="CELL" repeatHeader="0" rowCnt="%d" colCnt="%d" cellSpacing="0" borderFillIDRef="%d" noAdjust="0">`+
		`<hp:sz width="%d" widthRelTo="ABSOLUTE" height="%d" heightRelTo="ABSOLUTE" protect="0"/>`+
		`<hp:pos treatAsChar="1" affectLSpacing="0" flowWithText="1" allowOverlap="0" holdAnchorAndSO="0" vertRelTo="PARA" horzRelTo="COLUMN" vertAlign="TOP" horzAlign="LEFT" vertOffset="0" horzOffset="0"/>`+
		`<hp:outMargin left="0" right="0" top="0" bottom="0"/><hp:inMargin left="510" right="510" top="141" bottom="141"/>`,
		w.next(), len(rows), cols, fillTable, cw*cols, 282*len(rows))
	for ri, r := range rows {
		w.b.WriteString(`<hp:tr>`)
		for ci := 0; ci < cols; ci++ {
			c := Cell{}
			if ci < len(r) {
				c = r[ci]
			}
			hdr := 0
			if c.Header {
				hdr = 1
			}
			fmt.Fprintf(w.b, `<hp:tc name="" header="%d" hasMargin="0" protect="0" editable="0" dirty="0" borderFillIDRef="%d">`+
				`<hp:subList id="" textDirection="HORIZONTAL" lineWrap="BREAK" vertAlign="CENTER" linkListIDRef="0" linkListNextIDRef="0" textWidth="0" textHeight="0" hasTextRef="0" hasNumRef="0">`, hdr, fillOf(c))
			paras := c.Paras
			if len(paras) == 0 {
				paras = []Para{{}}
			}
			for _, p := range paras {
				w.para(p)
			}
			fmt.Fprintf(w.b, `</hp:subList><hp:cellAddr colAddr="%d" rowAddr="%d"/><hp:cellSpan colSpan="1" rowSpan="1"/><hp:cellSz width="%d" height="282"/><hp:cellMargin left="510" right="510" top="141" bottom="141"/></hp:tc>`, ci, ri, cw)
		}
		w.b.WriteString(`</hp:tr>`)
	}
	w.b.WriteString(`</hp:tbl></hp:run></hp:p>`)
}

// story는 머리말·꼬리말 컨트롤이다. 한/글은 secPr 안 사본이 아니라 첫 문단의 hp:ctrl을 읽는다(python-hwpx section_format.py).
func (w *section) story(tag, vAlign string, rs []Run) string {
	saved, out := w.b, new(strings.Builder)
	w.b = out
	fmt.Fprintf(w.b, `<hp:ctrl><hp:%s id="%d" applyPageType="BOTH"><hp:subList id="" textDirection="HORIZONTAL" lineWrap="BREAK" vertAlign="%s" linkListIDRef="0" linkListNextIDRef="0" textWidth="%d" textHeight="4252" hasTextRef="0" hasNumRef="0">`, tag, w.next(), vAlign, w.textWidth)
	w.para(Para{Style: Small, Runs: rs, Center: true})
	fmt.Fprintf(w.b, `</hp:subList></hp:%s></hp:ctrl>`, tag)
	w.b = saved
	return out.String()
}
