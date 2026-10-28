//go:build integration

// 숨긴 WebView2 인쇄 통합 테스트: 한글 3쪽 PDF(쪽 수, Type3 없음, TrueType 내장, 태그 PDF), 원격 요청 차단,
// 없는 페이지 오류, 시간 초과 뒤 다시 인쇄, 다른 WebView2(Wails 본창 흉내)와 같은 프로세스에서 인쇄, 인쇄 뒤 브라우저 프로세스 정리.
package printer

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"

	"howtonote/internal/win"
)

// 셋째 쪽의 원격 그림은 막혀야 한다(스펙 9절). 실제로 나가지 않으므로 네트워크가 없어도 된다.
const page = `<!doctype html><html lang="ko"><head><meta charset="utf-8"><style>
@font-face { font-family: "Pretendard"; font-weight: 400; src: url("Pretendard-Regular.ttf") format("truetype") }
@font-face { font-family: "Pretendard"; font-weight: 700; src: url("Pretendard-Bold.ttf") format("truetype") }
@page { size: A4; margin: 20mm 15mm }
body { font-family: "Pretendard", "Malgun Gothic"; font-size: 14pt }
.pb { break-after: page }
</style></head><body>
<h1 class="pb">첫 쪽 — 한글 출력 확인</h1>
<p class="pb">둘째 쪽: <b>'저장'</b> 버튼을 클릭합니다.</p>
<p>셋째 쪽 <img src="https://example.com/x.png" alt=""></p>
</body></html>`

// hf는 머리글·바닥글 템플릿이다. 템플릿은 페이지의 웹 글꼴을 못 써서 맑은 고딕을 직접 준다(안 주면 한글이 Type3).
const hf = `<div style="font-family:'Malgun Gothic';font-size:9px;width:100%;text-align:center">`

// writePage는 인쇄할 폴더를 만든다. 글꼴은 export 패키지의 Pretendard 파일을 쓴다.
func writePage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"Pretendard-Regular.ttf", "Pretendard-Bold.ttf"} {
		b, err := os.ReadFile(filepath.Join("..", "export", "fonts", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func checkPDF(t *testing.T, pdf []byte) {
	t.Helper()
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("PDF가 아니다")
	}
	if n := len(regexp.MustCompile(`/Type\s*/Page\b`).FindAll(pdf, -1)); n != 3 {
		t.Errorf("쪽 수 = %d, 3이어야 한다", n)
	}
	if regexp.MustCompile(`/Subtype\s*/Type3`).Match(pdf) {
		t.Error("Type3 글꼴이 들어갔다")
	}
	if !bytes.Contains(pdf, []byte("/FontFile2")) || !bytes.Contains(pdf, []byte("Pretendard-Regular")) {
		t.Error("Pretendard TrueType 내장(/FontFile2)이 없다")
	}
	if !bytes.Contains(pdf, []byte("/StructTreeRoot")) {
		t.Error("태그 PDF(/StructTreeRoot)가 아니다")
	}
}

// printProcs는 인쇄용 사용자 데이터 폴더를 쓰는 msedgewebview2 프로세스 수다.
func printProcs(t *testing.T) string {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		`(Get-CimInstance Win32_Process -Filter "Name='msedgewebview2.exe'" | Where-Object { $_.CommandLine -like '*print-webview*' }).Count`).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestPrintDir_KoreanPDF(t *testing.T) {
	dir := writePage(t)
	before := blockedTotal.Load()
	pdf, err := PrintDir(dir, "index.html", Options{
		HeaderHTML: hf + `머리글</div>`,
		FooterHTML: hf + `<span class="pageNumber"></span> / <span class="totalPages"></span></div>`,
	}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	checkPDF(t, pdf)
	if !bytes.Contains(pdf, []byte("MalgunGothic")) {
		t.Error("머리글·바닥글 글꼴(맑은 고딕)이 없다")
	}
	if blockedTotal.Load() == before {
		t.Error("가상 호스트 밖 요청(https://example.com)을 막지 않았다")
	}
	_ = os.MkdirAll("../../out", 0o755)
	_ = os.WriteFile("../../out/print-m3.pdf", pdf, 0o644) // 머리글·바닥글 위치는 눈으로 확인한다
}

func TestPrintDir_MissingPageFails(t *testing.T) {
	if _, err := PrintDir(t.TempDir(), "none.html", Options{}, 30*time.Second); err == nil {
		t.Fatal("없는 페이지인데 오류가 없다")
	}
}

func TestPrintDir_TimeoutReturnsAndUnlocks(t *testing.T) {
	dir := writePage(t)
	start := time.Now()
	if _, err := PrintDir(dir, "index.html", Options{}, time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("1ms 인쇄 = %v, ErrTimeout이어야 한다", err)
	}
	if d := time.Since(start); d > 6*time.Second {
		t.Errorf("시간 초과를 알리는 데 %v 걸렸다", d)
	}
	pdf, err := PrintDir(dir, "index.html", Options{}, 30*time.Second)
	if err != nil {
		t.Fatalf("시간 초과 뒤 인쇄: %v", err)
	}
	checkPDF(t, pdf)
}

// Wails 본창처럼 같은 프로세스에 다른 사용자 데이터 폴더의 WebView2가 떠 있어도 인쇄된다(M0에서 확인 못 한 조건).
func TestPrintDir_AlongsideAnotherWebView2(t *testing.T) {
	ready := make(chan bool, 1)
	stop := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
		hwnd, err := win.CreateHiddenWindow(800, 600)
		if err != nil {
			ready <- false
			return
		}
		c := edge.NewChromium()
		// go-webview2 핸들러는 c를 통해서만 살아 있어, 메시지를 돌리는 동안 c가 GC되면 콜백이 해제된 메모리를 읽는다(Wails는 c를 앱 끝까지 쥔다).
		defer runtime.KeepAlive(c)
		// 브라우저 프로세스가 늦게 끝나 t.TempDir 정리가 실패하므로 고정 폴더를 쓴다.
		c.DataPath = filepath.Join(os.TempDir(), "howtonote-test-other-webview")
		c.SetErrorCallback(func(error) {})
		c.Embed(hwnd)
		c.NavigateToString("<p>앱 창 흉내</p>")
		ready <- true
		for {
			select {
			case <-stop:
				return
			default:
				win.Pump()
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()
	if !<-ready {
		t.Fatal("다른 WebView2를 띄우지 못했다")
	}
	defer close(stop)
	pdf, err := PrintDir(writePage(t), "index.html", Options{}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	checkPDF(t, pdf)
}

// 인쇄가 끝나면 컨트롤러를 닫고 COM 참조를 놓아 인쇄용 브라우저 프로세스가 사라진다(M0에서는 5개가 남았다).
func TestPrintDir_ReleasesBrowserProcesses(t *testing.T) {
	dir := writePage(t)
	for i := 0; i < 2; i++ {
		if _, err := PrintDir(dir, "index.html", Options{}, 30*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for printProcs(t) != "0" {
		if time.Now().After(deadline) {
			t.Fatalf("인쇄 뒤 10초가 지나도 인쇄용 브라우저 프로세스 %s개가 남았다", printProcs(t))
		}
		time.Sleep(500 * time.Millisecond)
	}
}
