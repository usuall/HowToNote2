// HWPX 렌더러(스펙 7.2절): 문서 모델을 internal/hwpx 작성기의 문단·표·그림·안내 상자로 옮긴다.
// 목록은 한/글 번호 문단 대신 "• ", "1. " 글자와 들여쓰기로 쓴다(어느 한/글 버전에서도 같은 모양).
package export

import (
	"fmt"
	"os"
	"strings"
	"time"

	"howtonote/internal/hwpx"
)

// hwpxRuns는 글자 묶음을 옮긴다. 글자 안 \n은 줄마다 나눠 줄바꿈(Break)으로 쓴다:
// 작성기는 \n을 &#xA;로 쓰는데 한/글은 이를 줄바꿈으로 보이지 않을 수 있다(Task 7 검수).
func hwpxRuns(rs []Run) []hwpx.Run {
	out := make([]hwpx.Run, 0, len(rs))
	for _, r := range rs {
		hr := hwpx.Run{Bold: r.Bold, Italic: r.Italic, Underline: r.Underline, Href: r.Href, Break: r.Break}
		lines := strings.Split(strings.ReplaceAll(r.Text, "\r", ""), "\n")
		for i, line := range lines {
			if i > 0 {
				out = append(out, hwpx.Run{Bold: r.Bold, Italic: r.Italic, Underline: r.Underline, Break: true})
			}
			if line != "" || len(lines) == 1 {
				hr.Text = line
				out = append(out, hr)
			}
		}
	}
	return out
}

// hwpxText는 꾸밈 없는 글자(제목·목차 줄 등)를 글자 묶음으로 옮긴다. 줄바꿈 처리는 hwpxRuns를 탄다.
func hwpxText(s string) []hwpx.Run { return hwpxRuns([]Run{{Text: s}}) }

func hwpxPieces(ps []Piece) []hwpx.Run {
	var out []hwpx.Run
	for _, p := range ps {
		switch p.Field {
		case PageField:
			out = append(out, hwpx.Run{Field: hwpx.PageNum})
		case TotalField:
			out = append(out, hwpx.Run{Field: hwpx.TotalPages})
		default:
			out = append(out, hwpxText(p.Text)...)
		}
	}
	return out
}

func para(st hwpx.Style, rs []hwpx.Run) hwpx.Block {
	return hwpx.Block{Para: &hwpx.Para{Style: st, Runs: rs}}
}

// hwpxBlocks는 설명 블록을 바꾼다. depth는 목록 들여쓰기 단계다. inBox면 표를 문단으로 펼친다(안내 상자·표 칸은 문단만 담는다).
func hwpxBlocks(bs []Block, depth int, inBox bool) []hwpx.Block {
	var out []hwpx.Block
	for _, b := range bs {
		switch b.Kind {
		case KindPara:
			out = append(out, hwpx.Block{Para: &hwpx.Para{Runs: hwpxRuns(b.Runs), Indent: depth}})
		case KindHeading:
			st := hwpx.Heading3
			if b.Level == 4 {
				st = hwpx.Heading4
			}
			out = append(out, para(st, hwpxRuns(b.Runs)))
		case KindList:
			for i, item := range b.Items {
				mark := "• "
				if b.Ordered {
					mark = fmt.Sprintf("%d. ", b.Start+i)
				}
				inner := hwpxBlocks(item, depth+1, inBox)
				if len(inner) > 0 && inner[0].Para != nil && inner[0].Para.Style == hwpx.Body {
					p := *inner[0].Para
					p.Runs = append([]hwpx.Run{{Text: mark}}, p.Runs...)
					inner[0].Para = &p
				} else {
					inner = append([]hwpx.Block{{Para: &hwpx.Para{Runs: []hwpx.Run{{Text: mark}}, Indent: depth + 1}}}, inner...)
				}
				out = append(out, inner...)
			}
		case KindTable:
			if inBox {
				for _, row := range b.Rows {
					var cells []string
					for _, c := range row {
						cells = append(cells, blocksText(c.Blocks))
					}
					out = append(out, hwpx.Block{Para: &hwpx.Para{Runs: hwpxText(strings.Join(cells, " | ")), Indent: depth}})
				}
				continue
			}
			var rows [][]hwpx.Cell
			for _, row := range b.Rows {
				var cells []hwpx.Cell
				for _, c := range row {
					cells = append(cells, hwpx.Cell{Header: c.Header, Paras: parasOf(hwpxBlocks(c.Blocks, 0, true))})
				}
				rows = append(rows, cells)
			}
			out = append(out, hwpx.Block{Table: rows})
		}
	}
	return out
}

// blocksText는 블록의 글자를 한 줄로 이어 붙인다(안내 상자 안 표 칸).
func blocksText(bs []Block) string {
	var parts []string
	for _, b := range bs {
		switch b.Kind {
		case KindPara, KindHeading:
			parts = append(parts, Text(b.Runs))
		case KindList:
			for _, it := range b.Items {
				parts = append(parts, blocksText(it))
			}
		case KindTable:
			for _, r := range b.Rows {
				for _, c := range r {
					parts = append(parts, blocksText(c.Blocks))
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

func parasOf(bs []hwpx.Block) []hwpx.Para {
	var out []hwpx.Para
	for _, b := range bs {
		if b.Para != nil {
			out = append(out, *b.Para)
		}
	}
	return out
}

func hwpxBox(c Callout) hwpx.Block {
	paras := []hwpx.Para{{Runs: hwpxRuns([]Run{{Text: c.Heading(), Bold: true}})}}
	paras = append(paras, parasOf(hwpxBlocks(c.Blocks, 0, true))...)
	return hwpx.Block{Box: &hwpx.Box{Kind: c.Kind, Paras: paras}}
}

// HWPX는 문서 모델을 .hwpx 바이트로 만든다.
func HWPX(d Doc, o Options, now time.Time) ([]byte, error) {
	o = o.Normalize()
	hd := hwpx.Doc{
		Title: d.Title, Now: now, Letter: o.Paper == "Letter", Landscape: o.Landscape, Font: o.Font, ImagePct: o.ImageWidth,
		Header: hwpxPieces(Pieces(o.Header, d.Title, d.Date)), Footer: hwpxPieces(Pieces(o.Footer, d.Title, d.Date)),
	}
	brk := false // 다음 덩어리 앞에서 쪽을 나눈다
	add := func(bs ...hwpx.Block) {
		for _, b := range bs {
			if brk {
				b.PageBreak, brk = true, false
			}
			hd.Blocks = append(hd.Blocks, b)
		}
	}
	title := hwpx.Block{Para: &hwpx.Para{Style: hwpx.DocTitle, Center: true, Runs: hwpxText(d.Title)}}
	if o.Cover {
		add(hwpx.Block{Para: &hwpx.Para{}}, title, hwpx.Block{Para: &hwpx.Para{Center: true, Runs: hwpxText(d.Date)}})
		brk = true
	} else {
		add(title)
	}
	if o.TOC && len(d.Steps) > 0 {
		// 한/글 차례 필드는 한/글에서 새로 고쳐야 채워지므로, 쪽 번호 없는 스텝 목록으로 둔다.
		add(para(hwpx.Heading3, hwpxText("목차")))
		for _, s := range d.Steps {
			indent := 0
			if s.Sub {
				indent = 1
			}
			add(hwpx.Block{Para: &hwpx.Para{Runs: hwpxText(s.Heading()), Indent: indent}})
		}
		brk = true
	}
	for i, s := range d.Steps {
		if o.StepPerPage && !s.Sub && i > 0 {
			brk = true
		}
		for _, c := range s.Before {
			add(hwpxBox(c))
		}
		st := hwpx.StepTitle
		if s.Sub {
			st = hwpx.SubTitle
		}
		add(para(st, hwpxText(s.Heading())))
		if s.Image != nil {
			b, err := os.ReadFile(s.Image.Path)
			if err != nil {
				return nil, fmt.Errorf("스텝 그림 읽기: %w", err)
			}
			add(hwpx.Block{Image: &hwpx.Image{PNG: b, WidthPx: s.Image.Width, HeightPx: s.Image.Height}})
		}
		add(hwpxBlocks(s.Blocks, 0, false)...)
		for _, c := range s.After {
			add(hwpxBox(c))
		}
	}
	return hwpx.Write(hd)
}
