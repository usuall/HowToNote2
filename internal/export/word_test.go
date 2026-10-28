//go:build word

// Word로 DOCX를 실제로 열어 확인한다(Microsoft 365가 깔린 개발 PC 전용): 목차를 갱신해 스텝 제목이 들어가는지, PDF로 저장되는지.
// 실행 전후로 WINWORD.EXE PID를 기록해 비교한다 — 이 테스트가 시작한 숨긴 인스턴스가 Quit 뒤에도 남아 있으면
// 그 PID만 강제로 끝낸다. 실행 전부터 떠 있던(사용자) WINWORD.EXE는 절대 건드리지 않는다(데스크톱 안전 재정).
// 실행: go test -tags word ./internal/export/ -run Word -v
package export

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const wordScript = `
$ErrorActionPreference = 'Stop'
$w = New-Object -ComObject Word.Application
try {
  $w.Visible = $false
  $w.DisplayAlerts = 0
  $d = $w.Documents.Open($env:IN_DOCX, $false, $true)
  foreach ($t in $d.TablesOfContents) { $t.Update() }
  $pages = $d.ComputeStatistics(2)
  $toc = ''
  if ($d.TablesOfContents.Count -gt 0) { $toc = $d.TablesOfContents.Item(1).Range.Text }
  $d.SaveAs2($env:OUT_PDF, 17)
  $d.Close($false)
  [Console]::OutputEncoding = [Text.Encoding]::UTF8
  Write-Output "PAGES=$pages"
  Write-Output "TOC=$toc"
} finally { $w.Quit() }
`

// winwordPIDs는 지금 떠 있는 WINWORD.EXE PID 목록이다. 사용자 인스턴스를 건드리지 않으려고 전후로 비교한다.
func winwordPIDs(t *testing.T) map[string]bool {
	t.Helper()
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq WINWORD.EXE", "/FO", "CSV", "/NH").Output()
	if err != nil {
		t.Logf("tasklist 확인 못함: %v", err)
		return map[string]bool{}
	}
	pids := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, ",")
		if len(f) < 2 {
			continue
		}
		pid := strings.Trim(f[1], `" `)
		if _, err := strconv.Atoi(pid); err == nil {
			pids[pid] = true
		}
	}
	return pids
}

func TestDOCX_OpensInWord(t *testing.T) {
	before := winwordPIDs(t)

	b, err := DOCX(sampleDoc(t), goldenOptions(), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in, out := filepath.Join(dir, "guide.docx"), filepath.Join(dir, "guide.pdf")
	if err := os.WriteFile(in, b, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", wordScript)
	cmd.Env = append(os.Environ(), "IN_DOCX="+in, "OUT_PDF="+out)
	res, err := cmd.CombinedOutput()
	t.Logf("%s", res)

	// Quit은 스크립트 안에서 이미 불렀다. 그래도 남아 있으면(끝나는 데 시간이 걸림) 이 테스트가 새로
	// 띄운 PID만 끝낸다 — 실행 전부터 있던 PID(사용자 Word)는 절대 건드리지 않는다.
	for pid := range winwordPIDs(t) {
		if before[pid] {
			continue
		}
		t.Logf("남은 WINWORD.EXE(PID %s)를 끝낸다(이 테스트가 띄운 것)", pid)
		if out, kerr := exec.Command("taskkill", "/PID", pid, "/F").CombinedOutput(); kerr != nil {
			t.Logf("taskkill 실패: %v %s", kerr, out)
		}
	}

	if err != nil {
		t.Fatalf("Word가 열거나 저장하지 못했다: %v", err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() < 1000 {
		t.Fatalf("Word가 PDF를 만들지 못했다: %v", err)
	}
	if !strings.Contains(string(res), "TOC=1. '파일' 메뉴를 선택합니다.") || !strings.Contains(string(res), "1.1 '저장' 버튼을 클릭합니다.") {
		t.Error("목차에 스텝 제목이 없다")
	}
	_ = os.MkdirAll("../../out", 0o755)
	_ = os.WriteFile("../../out/export-m3.docx", b, 0o644)
}
