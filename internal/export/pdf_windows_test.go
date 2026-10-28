// PDF 머리글·바닥글 템플릿과 임시 폴더 정리 테스트(인쇄 없이): 자리표시자가 CDP 쪽 번호 칸이 되고, 글자는
// 이스케이프되고, 글꼴은 맑은 고딕이다. 한 시간 넘은 howtonote-print-* 폴더만 지워진다(M3-C18).
package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHFTemplate(t *testing.T) {
	got := hfTemplate(Pieces("{{가이드제목}} <b> {{쪽}}/{{전체쪽}}", "보고서 & 제출", "2026. 10. 21."))
	for _, want := range []string{
		`font-family:'Malgun Gothic'`, // 템플릿은 페이지 웹 글꼴을 못 쓴다(D5)
		`보고서 &amp; 제출 &lt;b&gt; <span class="pageNumber"></span>/<span class="totalPages"></span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("hfTemplate = %s\n%q 없음", got, want)
		}
	}
	if hfTemplate(nil) != "" {
		t.Error("빈 머리글은 빈 템플릿이어야 한다(printer가 빈 요소로 바꾼다)")
	}
}

// TestCleanStalePrintDirs는 한 시간 넘은 howtonote-print-* 폴더만 지우고, 최근 폴더와 다른 이름의 폴더는 그대로
// 두는지 본다(M3-C18).
func TestCleanStalePrintDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)

	stale := filepath.Join(tmp, "howtonote-print-stale")
	fresh := filepath.Join(tmp, "howtonote-print-fresh")
	other := filepath.Join(tmp, "other-dir")
	for _, d := range []string{stale, fresh, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	cleanStalePrintDirs()

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("한 시간 넘은 폴더가 남아 있다: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("방금 만든 howtonote-print-* 폴더가 지워졌다: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("다른 이름의 폴더가 지워졌다: %v", err)
	}
}
