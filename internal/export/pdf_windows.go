// PDF 렌더러(스펙 7.2절): 인쇄용 HTML과 그림·Pretendard 글꼴을 임시 폴더에 쓰고, 숨긴 WebView2(printer)로 인쇄한다.
// 머리글·바닥글은 CDP 템플릿이다. 템플릿은 페이지의 웹 글꼴을 못 써서 맑은 고딕을 직접 지정한다(안 그러면 한글이 Type3).
package export

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"howtonote/internal/printer"
)

// printTimeout은 인쇄 한 번의 제한 시간이다. 스텝 수백 개짜리 문서도 들어가게 넉넉히 둔다.
const printTimeout = 3 * time.Minute

// hfTemplate은 머리글·바닥글 조각을 CDP 템플릿 HTML로 바꾼다. 비면 빈 문자열(그 자리를 쓰지 않는다).
func hfTemplate(ps []Piece) string {
	if len(ps) == 0 {
		return ""
	}
	var b strings.Builder
	// width:100%를 빼면 Chromium 머리글 칸 안에서 글 폭으로 줄어 왼쪽에 붙는다. 이 모양이 가운데에 오는 것을
	// 실제 PDF로 쟀다(2026-10-22 샘플 매뉴얼: 595pt 쪽에서 머리글 x 263~332).
	b.WriteString(`<div style="font-family:'Malgun Gothic';font-size:8pt;color:#5f6368;width:100%;text-align:center;margin:0 15mm">`)
	for _, p := range ps {
		switch p.Field {
		case PageField:
			b.WriteString(`<span class="pageNumber"></span>`)
		case TotalField:
			b.WriteString(`<span class="totalPages"></span>`)
		default:
			b.WriteString(html.EscapeString(p.Text))
		}
	}
	b.WriteString(`</div>`)
	return b.String()
}

// cleanStalePrintDirs는 한 시간 넘은 howtonote-print-* 임시 폴더를 지운다. 바깥 제한 시간(printer.PrintDir의
// timeout+5초) 뒤에도 WebView2가 그림·HTML 파일을 쥐고 있으면 아래 defer os.RemoveAll이 실패해 합성 그림
// 스크린샷이 남을 수 있다(컨트롤러 재정 M3-C18). 그 실패를 바로 되돌릴 수는 없으니, 다음 PDF() 호출이 지나간
// 폴더를 쓸어가게 해서 재시도를 대신한다. 폴더 하나가 지워지지 않아도(아직 열려 있는 핸들 등) 다음 번에 또 시도하면
// 되므로 개별 오류는 무시한다.
func cleanStalePrintDirs() {
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "howtonote-print-*"))
	cutoff := time.Now().Add(-time.Hour)
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil || fi.ModTime().After(cutoff) {
			continue
		}
		os.RemoveAll(m)
	}
}

// PDF는 문서 모델을 PDF 바이트로 만든다.
func PDF(d Doc, o Options) ([]byte, error) {
	o = o.Normalize()
	cleanStalePrintDirs()
	dir, err := os.MkdirTemp("", "howtonote-print-")
	if err != nil {
		return nil, err
	}
	// 실패해도(WebView2가 아직 파일을 쥔 채 시간 초과) 위 cleanStalePrintDirs가 다음 호출에서 정리한다.
	defer os.RemoveAll(dir)
	page, err := printHTML(d, o)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{"index.html": page, "Pretendard-Regular.ttf": fontRegular, "Pretendard-Bold.ttf": fontBold}
	for _, s := range d.Steps {
		if s.Image == nil {
			continue
		}
		b, err := os.ReadFile(s.Image.Path)
		if err != nil {
			return nil, fmt.Errorf("스텝 그림 읽기: %w", err)
		}
		files[fmt.Sprintf("img/%d.png", s.ID)] = b
	}
	if err := os.MkdirAll(filepath.Join(dir, "img"), 0o755); err != nil {
		return nil, err
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), b, 0o644); err != nil {
			return nil, err
		}
	}
	return printer.PrintDir(dir, "index.html", printer.Options{
		HeaderHTML: hfTemplate(Pieces(o.Header, d.Title, d.Date)),
		FooterHTML: hfTemplate(Pieces(o.Footer, d.Title, d.Date)),
		Landscape:  o.Landscape,
	}, printTimeout)
}
