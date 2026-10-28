// Tiptap JSON → 블록 변환 테스트: 허용 서식(문단, H3·H4, 굵게·기울임·밑줄·링크, 목록 안 목록, 표, 줄바꿈), 위험한 링크, 깨진 값.
package export

import (
	"reflect"
	"testing"
)

func TestBlocks_AllowedFormatting(t *testing.T) {
	got := Blocks(sampleDesc)
	want := []Block{
		{Kind: KindPara, Runs: []Run{{Text: "보통 "}, {Text: "굵게", Bold: true}, {Text: " "}, {Text: "기울임", Italic: true}, {Text: " "}, {Text: "밑줄", Underline: true}, {Break: true}, {Text: "링크 <A&B>", Href: "https://example.com/a?b=1&c=2"}}},
		{Kind: KindHeading, Level: 3, Runs: []Run{{Text: "소제목 셋"}}},
		{Kind: KindList, Start: 1, Items: [][]Block{
			{{Kind: KindPara, Runs: []Run{{Text: "글머리 하나"}}}, {Kind: KindList, Ordered: true, Start: 3, Items: [][]Block{{{Kind: KindPara, Runs: []Run{{Text: "안쪽 번호 셋"}}}}}}},
			{{Kind: KindPara, Runs: []Run{{Text: "글머리 둘"}}}},
		}},
		{Kind: KindHeading, Level: 4, Runs: []Run{{Text: "소제목 넷"}}},
		{Kind: KindTable, Rows: [][]Cell{
			{{Header: true, Blocks: []Block{{Kind: KindPara, Runs: []Run{{Text: "항목"}}}}}, {Header: true, Blocks: []Block{{Kind: KindPara, Runs: []Run{{Text: "값"}}}}}},
			{{Blocks: []Block{{Kind: KindPara, Runs: []Run{{Text: "이름"}}}}}, {Blocks: []Block{{Kind: KindPara}}}},
		}},
		{Kind: KindPara, Runs: []Run{{Text: "로컬 링크는 글자만"}}}, // file: 링크는 버린다(스펙 7.4절)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Blocks =\n%+v\nwant\n%+v", got, want)
	}
}

func TestBlocks_EmptyBrokenAndUnknown(t *testing.T) {
	for _, js := range []string{"", "  ", "{", `{"type":"paragraph"}`, `[]`} {
		if b := Blocks(js); b != nil {
			t.Errorf("Blocks(%q) = %+v, nil이어야 한다", js, b)
		}
	}
	// 모르는 노드(예: 나중에 생길 blockquote)는 안의 문단을 꺼내고, 모르는 글자 노드는 글자만 남긴다.
	got := Blocks(`{"type":"doc","content":[{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"인용"}]}]},{"type":"mention","content":[{"type":"text","text":"@이름"}]}]}`)
	want := []Block{{Kind: KindPara, Runs: []Run{{Text: "인용"}}}, {Kind: KindPara, Runs: []Run{{Text: "@이름"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Blocks = %+v", got)
	}
}

func TestSafeHref(t *testing.T) {
	for href, want := range map[string]string{
		"https://a.example/x": "https://a.example/x", "HTTP://A.example": "HTTP://A.example", "mailto:a@b.example": "mailto:a@b.example",
		"file:///C:/Users/x": "", "javascript:alert(1)": "", `C:\Users\x`: "", "": "",
	} {
		if got := safeHref(href); got != want {
			t.Errorf("safeHref(%q) = %q, want %q", href, got, want)
		}
	}
}

func TestCallouts_SplitsAndDefaults(t *testing.T) {
	before, after := Callouts(`[{"type":"danger","title":"위험!","body":null,"position":"before"},{"type":"이상한값","title":"","body":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"본문"}]}]},"position":"after"},{"type":"tip","title":"","body":null}]`)
	if len(before) != 1 || before[0].Kind != "danger" || before[0].Heading() != "위험!" {
		t.Errorf("앞 = %+v", before)
	}
	if len(after) != 2 || after[0].Kind != "info" || after[0].Heading() != "알림" || len(after[0].Blocks) != 1 || after[1].Heading() != "도움말" {
		t.Errorf("뒤 = %+v", after)
	}
	if b, a := Callouts("깨짐"); b != nil || a != nil {
		t.Error("깨진 callouts_json은 빈 목록이어야 한다")
	}
}
