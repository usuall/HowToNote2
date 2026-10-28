// 작성한 HWPX를 python-hwpx 스키마·패키지 검증기로 확인한다. python이나 python-hwpx가 없으면 건너뛴다.
package hwpx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWrite_PassesPythonHwpxValidation(t *testing.T) {
	py, err := exec.LookPath("python")
	if err != nil {
		t.Skip("python 없음")
	}
	if exec.Command(py, "-c", "import hwpx").Run() != nil {
		t.Skip("python-hwpx 미설치: python -m pip install python-hwpx")
	}
	a4 := sampleDoc(t)
	wide := sampleDoc(t)
	wide.Letter, wide.Landscape, wide.Font = true, true, "맑은 고딕"
	for name, d := range map[string]Doc{"a4": a4, "letter-landscape": wide} {
		b, err := Write(d)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(t.TempDir(), name+".hwpx")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(py, "../../tools/validate_hwpx.py", p).CombinedOutput()
		if err != nil {
			t.Fatalf("%s 검증 실패: %v\n%s", name, err, out)
		}
		t.Logf("%s: %s", name, out)
	}
}
