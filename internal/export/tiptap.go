// Tiptap JSON(설명·안내 상자 본문)을 문서 모델 블록으로 한 번만 바꾼다(스펙 7.1절).
// 노드·마크 이름은 편집기 허용 서식(frontend/src/editor/rich.ts ALLOWED_NODES, ALLOWED_MARKS)과 같다.
package export

import (
	"encoding/json"
	"strings"
)

type node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs"`
	Content []node         `json:"content"`
	Text    string         `json:"text"`
	Marks   []struct {
		Type  string         `json:"type"`
		Attrs map[string]any `json:"attrs"`
	} `json:"marks"`
}

// Blocks는 Tiptap 문서 JSON을 블록으로 바꾼다. 비었거나 깨졌으면 nil이다(편집기도 깨진 값은 빈 문서로 본다).
func Blocks(js string) []Block {
	var doc node
	if strings.TrimSpace(js) == "" || json.Unmarshal([]byte(js), &doc) != nil || doc.Type != "doc" {
		return nil
	}
	return blocksOf(doc.Content)
}

func blocksOf(nodes []node) []Block {
	var out []Block
	for _, n := range nodes {
		switch n.Type {
		case "paragraph":
			out = append(out, Block{Kind: KindPara, Runs: runsOf(n.Content)})
		case "heading":
			lv := intAttr(n.Attrs, "level", 3)
			if lv != 4 {
				lv = 3 // 편집기는 3·4만 쓴다
			}
			out = append(out, Block{Kind: KindHeading, Level: lv, Runs: runsOf(n.Content)})
		case "bulletList", "orderedList":
			b := Block{Kind: KindList, Ordered: n.Type == "orderedList", Start: intAttr(n.Attrs, "start", 1)}
			for _, it := range n.Content {
				b.Items = append(b.Items, blocksOf(it.Content))
			}
			out = append(out, b)
		case "table":
			b := Block{Kind: KindTable}
			for _, row := range n.Content {
				var cells []Cell
				for _, c := range row.Content {
					cells = append(cells, Cell{Header: c.Type == "tableHeader", Blocks: blocksOf(c.Content)})
				}
				b.Rows = append(b.Rows, cells)
			}
			out = append(out, b)
		default:
			// 스키마에 없는 새 노드(예: 나중에 생길 blockquote)가 와도 버리지 않고 안의 내용을 꺼낸다(M3-C13).
			// 글자만 있으면 문단으로 두고, 글자 안 링크도 safeHref로 다시 검사한다(아래 runsOf).
			if inner := blocksOf(n.Content); len(inner) > 0 {
				out = append(out, inner...)
			} else if rs := runsOf([]node{n}); len(rs) > 0 {
				out = append(out, Block{Kind: KindPara, Runs: rs})
			}
		}
	}
	return out
}

func runsOf(nodes []node) []Run {
	var out []Run
	for _, n := range nodes {
		switch n.Type {
		case "text":
			r := Run{Text: n.Text}
			for _, m := range n.Marks {
				switch m.Type {
				case "bold":
					r.Bold = true
				case "italic":
					r.Italic = true
				case "underline":
					r.Underline = true
				case "link":
					r.Href = safeHref(m.Attrs["href"])
				}
			}
			out = append(out, r)
		case "hardBreak":
			out = append(out, Run{Break: true})
		default:
			out = append(out, runsOf(n.Content)...)
		}
	}
	return out
}

// safeHref는 웹·메일 주소만 남긴다. file:·javascript: 같은 주소는 글자만 남기고 링크를 뺀다(스펙 7.4절, 9절).
// 편집기(rich.ts isAllowedUri)와 같은 http·https·mailto만 허용한다.
func safeHref(v any) string {
	s, _ := v.(string)
	l := strings.ToLower(strings.TrimSpace(s))
	if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "mailto:") {
		return strings.TrimSpace(s)
	}
	return ""
}

func intAttr(a map[string]any, key string, def int) int {
	if f, ok := a[key].(float64); ok && f >= 1 {
		return int(f)
	}
	return def
}

// Text는 블록의 글자만 이어 붙인다(미리 보기 글, 표 칸을 문단으로 펼칠 때).
func Text(rs []Run) string {
	var b strings.Builder
	for _, r := range rs {
		if r.Break {
			b.WriteString(" ")
		}
		b.WriteString(r.Text)
	}
	return b.String()
}
