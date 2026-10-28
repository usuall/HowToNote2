// HWPX 렌더러 테스트: 목록·안내 상자·목차·쪽 나눔 옮기기, 글자 안 줄바꿈, python-hwpx 검증, 골든 파일, 경로 노출.
package export

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func hwpxOut(t *testing.T, d Doc, o Options) map[string][]byte {
	t.Helper()
	b, err := HWPX(d, o, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return unzipAll(t, b)
}

func TestHWPX_Content(t *testing.T) {
	sec := string(hwpxOut(t, sampleDoc(t), goldenOptions())["Contents/section0.xml"])
	for _, want := range []string{
		`<hp:t>목차</hp:t>`, `<hp:t>1.1 &#39;저장&#39; 버튼을 클릭합니다.</hp:t>`,
		`<hp:t>• </hp:t></hp:run><hp:run charPrIDRef="7"><hp:t>글머리 하나</hp:t>`,   // 글머리 기호는 글자로
		`<hp:t>3. </hp:t></hp:run><hp:run charPrIDRef="7"><hp:t>안쪽 번호 셋</hp:t>`, // 번호 목록 시작 3
		`<hp:t>주의</hp:t>`, `<hp:t>도움말 &lt;제목&gt;</hp:t>`, // 안내 상자 제목
		`<hp:t>소제목 셋</hp:t>`, `<hp:t>로컬 링크는 글자만</hp:t>`,
		`binaryItemIDRef="image2"`,
	} {
		if !strings.Contains(sec, want) {
			t.Errorf("section0.xml에 %q 없음", want)
		}
	}
	// 표지 뒤, 목차 뒤, 둘째 최상위 스텝 앞(1스텝 1쪽)에서 쪽을 나눈다. 하위 스텝 앞에서는 나누지 않는다.
	if n := strings.Count(sec, `pageBreak="1"`); n != 3 {
		t.Errorf("쪽 나눔 %d개, 3개여야 한다", n)
	}
	if strings.Contains(sec, "file:") {
		t.Error("로컬 링크가 들어갔다")
	}
}

// 안내 상자 안 표는 한/글 표 대신 "칸 | 칸" 문단으로 펼친다(D7, 검수 Must 2).
func TestHWPX_CalloutTableFlattened(t *testing.T) {
	cell := Cell{Blocks: []Block{{Kind: KindPara, Runs: []Run{{Text: "칸"}}}}}
	table := Block{Kind: KindTable, Rows: [][]Cell{{cell, cell}}}
	d := sampleDoc(t)
	d.Steps[0].Before = []Callout{{Kind: "info", Blocks: []Block{table}}}
	sec := string(hwpxOut(t, d, goldenOptions())["Contents/section0.xml"])
	if !strings.Contains(sec, "칸 | 칸") {
		t.Error(`안내 상자 안 표가 "칸 | 칸"으로 펼쳐지지 않았다`)
	}
}

// 한/글은 글자 안 \n(&#xA;)을 줄바꿈으로 보지 않을 수 있어, 렌더러가 줄마다 나눠 lineBreak로 쓴다(Task 7 검수).
func TestHWPX_NewlineInTextBecomesLineBreak(t *testing.T) {
	d := sampleDoc(t)
	d.Steps[0].Title = "첫 줄\r\n둘째 줄"
	d.Steps[0].Blocks = []Block{{Kind: KindPara, Runs: []Run{{Text: "가\n나", Bold: true}}}}
	sec := string(hwpxOut(t, d, goldenOptions())["Contents/section0.xml"])
	if strings.Contains(sec, "&#xA;") || strings.Contains(sec, "&#xD;") {
		t.Error("줄바꿈 문자가 글자로 들어갔다")
	}
	for _, want := range []string{
		`<hp:t>1. 첫 줄</hp:t></hp:run><hp:run charPrIDRef="17"><hp:t><hp:lineBreak/></hp:t></hp:run><hp:run charPrIDRef="17"><hp:t>둘째 줄</hp:t>`,
		`<hp:t>가</hp:t></hp:run><hp:run charPrIDRef="8"><hp:t><hp:lineBreak/></hp:t></hp:run><hp:run charPrIDRef="8"><hp:t>나</hp:t>`, // 굵게가 이어진다
	} {
		if !strings.Contains(sec, want) {
			t.Errorf("section0.xml에 %q 없음", want)
		}
	}
}

func TestHWPX_PassesPythonHwpxValidation(t *testing.T) {
	b, err := HWPX(sampleDoc(t), goldenOptions(), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll("../../out", 0o755)
	_ = os.WriteFile("../../out/export-m3.hwpx", b, 0o644) // 한/글·한컴오피스 뷰어로 열어 보는 수동 확인용
	py, err := exec.LookPath("python")
	if err != nil {
		t.Skip("python 없음")
	}
	if exec.Command(py, "-c", "import hwpx").Run() != nil {
		t.Skip("python-hwpx 미설치: python -m pip install python-hwpx")
	}
	p := filepath.Join(t.TempDir(), "guide.hwpx")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(py, "../../tools/validate_hwpx.py", p).CombinedOutput()
	if err != nil {
		t.Fatalf("스키마 검증 실패: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}

func TestHWPX_Golden(t *testing.T) {
	parts := hwpxOut(t, sampleDoc(t), goldenOptions())
	for _, name := range []string{"Contents/section0.xml", "Contents/content.hpf", "Preview/PrvText.txt"} {
		golden(t, "hwpx/"+strings.ReplaceAll(name, "/", "_"), parts[name])
	}
}

func TestHWPX_NoLocalPaths(t *testing.T) {
	d := sampleDoc(t)
	for name, data := range hwpxOut(t, d, goldenOptions()) {
		if l := leaks(data, forbidden(d)); l != nil {
			t.Errorf("%s에 %q", name, l)
		}
	}
}
