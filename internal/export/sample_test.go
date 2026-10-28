// 렌더러 테스트 공용 고정 문서: 최상위 2개·하위 1개 스텝, 그림 2개, 앞·뒤 안내 상자, 허용 서식 전부, 로컬 링크.
// 그림 파일은 t.TempDir(C:\Users\…\AppData\Local\Temp 아래)에 두어 경로 노출 검사가 실제 경로로 돈다.
package export

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

// writePNG는 w×h 그림 파일을 dir에 만든다.
func writePNG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 180, A: 255})
		}
	}
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return p
}

const sampleDesc = `{"type":"doc","content":[
{"type":"paragraph","content":[{"type":"text","text":"보통 "},{"type":"text","marks":[{"type":"bold"}],"text":"굵게"},{"type":"text","text":" "},{"type":"text","marks":[{"type":"italic"}],"text":"기울임"},{"type":"text","text":" "},{"type":"text","marks":[{"type":"underline"}],"text":"밑줄"},{"type":"hardBreak"},{"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com/a?b=1&c=2"}}],"text":"링크 <A&B>"}]},
{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"소제목 셋"}]},
{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"글머리 하나"}]},{"type":"orderedList","attrs":{"start":3},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"안쪽 번호 셋"}]}]}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"글머리 둘"}]}]}]},
{"type":"heading","attrs":{"level":4},"content":[{"type":"text","text":"소제목 넷"}]},
{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","attrs":{"colspan":1,"rowspan":1},"content":[{"type":"paragraph","content":[{"type":"text","text":"항목"}]}]},{"type":"tableHeader","attrs":{"colspan":1,"rowspan":1},"content":[{"type":"paragraph","content":[{"type":"text","text":"값"}]}]}]},{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colspan":1,"rowspan":1},"content":[{"type":"paragraph","content":[{"type":"text","text":"이름"}]}]},{"type":"tableCell","attrs":{"colspan":1,"rowspan":1},"content":[{"type":"paragraph"}]}]}]},
{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":"file:///C:/Users/x/secret.txt"}}],"text":"로컬 링크는 글자만"}]}
]}`

// sampleDoc은 렌더러 테스트용 고정 문서다.
func sampleDoc(t *testing.T) Doc {
	t.Helper()
	dir := t.TempDir()
	before, after := Callouts(`[{"type":"warning","title":"","body":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"저장 전에 확인하세요."}]}]},"position":"before"},{"type":"tip","title":"도움말 <제목>","body":null,"position":"after"}]`)
	return Doc{
		Title: "보고서 <저장> & 제출",
		Date:  "2026. 10. 21.",
		Steps: []Step{
			{ID: 1, Number: "1", Title: "'파일' 메뉴를 선택합니다.", Image: &Image{Path: writePNG(t, dir, "a.png", 1600, 900), Width: 1600, Height: 900}, Blocks: Blocks(sampleDesc)},
			{ID: 2, Number: "1.1", Sub: true, Title: "'저장' 버튼을 클릭합니다.", Before: before, After: after, Image: &Image{Path: writePNG(t, dir, "b.png", 320, 200), Width: 320, Height: 200}},
			{ID: 3, Number: "2", Title: ""},
		},
	}
}

// unzipAll은 ZIP 항목 이름별 내용이다.
func unzipAll(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = data
	}
	return out
}
