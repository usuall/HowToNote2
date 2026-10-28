// 골든 파일·경로 노출 검사 공용 도구(스펙 10절)와 HTML 테스트. 의도한 변경이면
// go test ./internal/export -run Golden -update로 기준을 다시 쓰고, git diff로 바뀐 곳을 검토한 뒤 함께 커밋한다.
package export

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "testdata/golden 기준 파일을 지금 출력으로 다시 쓴다")

var (
	// html/template은 속성 안 "+"를 "&#43;"로 쓴다(표준 라이브러리 attrEscaper). base64 글자 집합에 "+"가 있어
	// 그대로 두면 정규식이 첫 "+" 앞에서 멈추고 긴 base64 꼬리가 골든에 그대로 남는다(검수 재정 M3-C8).
	reBase64 = regexp.MustCompile(`base64,(?:[A-Za-z0-9+/=]|&#43;)+`)
	reTags   = regexp.MustCompile(`>\s*<`)
)

// normalize는 비교를 읽기 쉽게 만든다: base64 그림·글꼴은 줄이고, 태그마다 줄을 바꾼다.
func normalize(b []byte) []byte {
	b = reBase64.ReplaceAll(b, []byte("base64,…"))
	return reTags.ReplaceAll(b, []byte(">\n<"))
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	got = normalize(got)
	p := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("기준 파일이 없다: %v (처음이면 -update로 만든다)", err)
	}
	if bytes.Equal(got, want) {
		return
	}
	gl, wl := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	for i := 0; i < max(len(gl), len(wl)); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("%s %d번째 줄이 기준과 다르다:\n지금: %s\n기준: %s\n의도한 변경이면 -update로 기준을 다시 쓴다", name, i+1, g, w)
			return
		}
	}
}

func goldenOptions() Options {
	o := DefaultOptions()
	o.StepPerPage = true
	return o
}

// forbidden은 결과물에 있으면 안 되는 문자열이다(스펙 7.4절). 사용자 이름은 3자 이상일 때만 본다(짧으면 우연히 겹친다).
func forbidden(d Doc) []string {
	out := []string{`C:\Users`, `C:/Users`, "AppData", "file:"}
	if u := os.Getenv("USERNAME"); len(u) >= 3 {
		out = append(out, u)
	}
	for _, s := range d.Steps {
		if s.Image != nil {
			out = append(out, filepath.Dir(s.Image.Path), filepath.ToSlash(filepath.Dir(s.Image.Path)))
		}
	}
	return out
}

// leaks는 b에 들어 있는 금지 문자열을 돌려준다. 대소문자는 가리지 않는다.
// base64 덩어리(그림·글꼴)는 빼고 본다: 수 MB의 base64에 사용자 이름 글자가 우연히 이어 나올 수 있다.
func leaks(b []byte, bad []string) []string {
	var found []string
	low := bytes.ToLower(reBase64.ReplaceAll(b, nil))
	for _, s := range bad {
		if bytes.Contains(low, []byte(strings.ToLower(s))) {
			found = append(found, s)
		}
	}
	return found
}

func TestHTML_Golden(t *testing.T) {
	b, err := HTML(sampleDoc(t), goldenOptions())
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "guide.html", b)
	p, err := printHTML(sampleDoc(t), goldenOptions().Normalize())
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "print.html", p)
}

func TestHTML_SingleFileRules(t *testing.T) {
	b, err := HTML(sampleDoc(t), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`<html lang="ko">`, `default-src 'none'`, // 네트워크 없음(스펙 9절): 문서가 밖에서 아무것도 가져오지 않는다
		`<title>보고서 &lt;저장&gt; &amp; 제출</title>`,
		`font-family:"Malgun Gothic", "맑은 고딕", sans-serif`, // 기본은 시스템 글꼴(스펙 7.2절)
		`<img src="data:image/png;base64,`, `<a href="https://example.com/a?b=1&amp;c=2">링크 &lt;A&amp;B&gt;</a>`,
		`<aside class="callout warning"><p class="callout-title">주의</p>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("HTML에 %q 없음", want)
		}
	}
	if strings.Contains(s, "@font-face") || strings.Contains(s, "@page") {
		t.Error("기본 HTML에는 글꼴을 넣지 않고 인쇄 규칙도 없어야 한다")
	}
	o := DefaultOptions()
	o.EmbedFont = true
	b, _ = HTML(sampleDoc(t), o)
	if !strings.Contains(string(b), `src:url("data:font/ttf;base64,`) || !strings.Contains(string(b), `font-family:"Pretendard"`) {
		t.Error("글꼴 넣기를 켜면 Pretendard가 base64로 들어가야 한다")
	}
}

// 세로로 긴 그림이 쪽을 넘지 않게, 인쇄용 CSS는 0.8 × 본문 높이(쪽 높이 - 위아래 여백 40mm)를 그림 높이 한도로 둔다(검수 Important 1).
// Go가 아는 건 이 한도뿐이다(실제 줄어든 모양은 브라우저/Chromium이 max-width·max-height·height:auto로 계산한다).
func TestHTML_PrintImageMaxHeight(t *testing.T) {
	for _, c := range []struct {
		paper     string
		landscape bool
		want      float64 // mm
	}{
		{"A4", false, (297.0 - 40) * 0.8},
		{"A4", true, (210.0 - 40) * 0.8},
		{"Letter", false, (279.4 - 40) * 0.8},
		{"Letter", true, (215.9 - 40) * 0.8},
	} {
		o := DefaultOptions()
		o.Paper, o.Landscape = c.paper, c.landscape
		p, err := printHTML(sampleDoc(t), o.Normalize())
		if err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("figure img{max-height:%.2fmm}", c.want)
		if !strings.Contains(string(p), want) {
			t.Errorf("%s landscape=%v: %q 없음", c.paper, c.landscape, want)
		}
	}
}

func TestHTML_NoLocalPaths(t *testing.T) {
	d := sampleDoc(t)
	for _, embed := range []bool{false, true} {
		o := goldenOptions()
		o.EmbedFont = embed
		b, err := HTML(d, o)
		if err != nil {
			t.Fatal(err)
		}
		if l := leaks(b, forbidden(d)); l != nil {
			t.Errorf("HTML(글꼴 넣기 %v)에 %q", embed, l)
		}
	}
}
