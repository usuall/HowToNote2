// 내보내기 바인딩 테스트: 합성 그림 목록·올리기, 그림 없이 내보내기 거부, HTML·DOCX·HWPX 파일 쓰기, 문서 모양 저장·지우기,
// 사용자에게 보이는 오류에 경로가 없는지(재정 R31), 안 쓰는 그림 정리(재정 M3-C3), '폴더 열기' 거부.
// 고정 가이드(fixture, app_edit_test.go)는 같은 그림·클릭 표시의 스텝 3개라 합성 그림은 하나다.
package main

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"howtonote/internal/export"
	"howtonote/internal/recorder"
	"howtonote/internal/store"
)

// renderAll은 프론트 ensureRendered 대신 원본 그림을 그대로 합성 그림으로 올린다.
func renderAll(t *testing.T, a *App, gid int64) {
	t.Helper()
	jobs, err := a.ExportPlan(gid)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if err := a.PutRendered(j.Key, base64.StdEncoding.EncodeToString(encode(t, "png", 8, 6))); err != nil {
			t.Fatal(err)
		}
	}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// choose는 ChooseExportFile이 막 이 경로를 돌려준 것처럼 등록한다(검수 Minor 7: ExportGuide는 고른 경로만 받는다).
func choose(a *App, path string) string {
	a.chosenExport = path
	return path
}

func TestApp_ExportPlanAndPutRendered(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	jobs, err := a.ExportPlan(gid)
	if err != nil || len(jobs) != 1 || jobs[0].Cached || len(jobs[0].Key) != 64 || jobs[0].AnnotationsJSON != marker {
		t.Fatalf("ExportPlan = %+v, %v", jobs, err)
	}
	for _, bad := range []struct{ key, data string }{
		{jobs[0].Key, "!!"}, // base64가 아님
		{jobs[0].Key, base64.StdEncoding.EncodeToString(encode(t, "jpeg", 8, 6))}, // PNG가 아님
		{"../../x", base64.StdEncoding.EncodeToString(encode(t, "png", 8, 6))},    // 경로 조작
		{jobs[0].Key, pngSized(t, 8000, 6000)},                                    // 4800만 화소: 원본 그림 한계(maxPixels)보다 크다
	} {
		if err := a.PutRendered(bad.key, bad.data); !errors.Is(err, errRenderData) {
			t.Errorf("PutRendered(%q) = %v, errRenderData여야 한다", bad.key, err)
		}
	}
	renderAll(t, a, gid)
	if jobs, _ := a.ExportPlan(gid); !jobs[0].Cached {
		t.Error("올린 뒤에도 Cached가 아니다")
	}
}

// 숨긴 최상위 스텝의 하위 스텝은 export.Build가 내보내지 않으므로(재정 M3-C2) 합성 그림도 만들지 않는다.
// ExportPlan이 준 것만 올려도 내보내기가 되어야 한다(목록과 Build가 같은 스텝을 고른다).
func TestApp_ExportPlanSkipsChildOfHiddenStep(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	d, _ := a.OpenGuide(gid)
	s1, s2, s3 := d.Steps[0], d.Steps[1], d.Steps[2]
	s1.Hidden = true
	s2.ParentID = s1.ID
	s2.AnnotationsJSON = "[]" // 다른 합성 그림이 되게 한다
	if err := a.SaveSteps(gid, []store.Step{s1, s2}); err != nil {
		t.Fatal(err)
	}
	jobs, err := a.ExportPlan(gid)
	if err != nil || len(jobs) != 1 || jobs[0].StepID != s3.ID {
		t.Fatalf("ExportPlan = %+v, %v (셋째 스텝 하나만이어야 한다)", jobs, err)
	}
	renderAll(t, a, gid)
	if err := a.ExportGuide(gid, "html", "", choose(a, filepath.Join(t.TempDir(), "x.html"))); err != nil {
		t.Errorf("목록대로 올린 뒤 내보내기 = %v", err)
	}
}

func TestApp_ExportGuideNeedsRenderedImages(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	if err := a.ExportGuide(gid, "html", "", choose(a, filepath.Join(t.TempDir(), "x.html"))); !errors.Is(err, errNotRendered) {
		t.Errorf("그림 없이 내보내기 = %v, errNotRendered여야 한다", err)
	}
	for _, c := range []struct{ format, opts, path string }{
		{"exe", "", filepath.Join(t.TempDir(), "x.exe")},
		{"html", "{", filepath.Join(t.TempDir(), "x.html")},
		{"html", "", "x.html"}, // 상대 경로
	} {
		if err := a.ExportGuide(gid, c.format, c.opts, c.path); err == nil {
			t.Errorf("ExportGuide(%q, %q, %q)가 오류 없이 끝났다", c.format, c.opts, c.path)
		}
	}
}

func TestApp_ExportGuideWritesFiles(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	renderAll(t, a, gid)
	dir := t.TempDir()
	for _, format := range []string{"html", "docx", "hwpx"} {
		path := choose(a, filepath.Join(dir, "고정 가이드."+format))
		if err := a.ExportGuide(gid, format, `{"cover":false,"toc":false}`, path); err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if format == "html" {
			if !strings.Contains(string(b), "<title>고정 가이드</title>") || !strings.Contains(string(b), "3. 셋") {
				t.Error("HTML 내용이 틀렸다")
			}
			if a.lastExport != path {
				t.Errorf("lastExport %q, 방금 쓴 %q여야 한다", a.lastExport, path)
			}
			continue
		}
		if _, err := zip.NewReader(strings.NewReader(string(b)), int64(len(b))); err != nil {
			t.Errorf("%s가 ZIP이 아니다: %v", format, err)
		}
	}
	// 열 수 없는 경로(없는 폴더)는 한국어 안내 문장
	if err := a.ExportGuide(gid, "html", "", choose(a, filepath.Join(dir, "없는 폴더", "x.html"))); !errors.Is(err, errExportWrite) {
		t.Errorf("없는 폴더 = %v, errExportWrite여야 한다", err)
	}
}

// 렌더러·파일 쓰기 오류(fs.PathError)에는 로컬 경로와 사용자 이름이 들어 있다. 바인딩은 이를 고정 문장으로 바꿔
// 사용자에게 경로가 보이지 않게 한다(재정 R31, Task 3 메모).
func TestApp_ExportErrorsHideLocalPaths(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	renderAll(t, a, gid)
	user := os.Getenv("USERNAME")
	clean := func(what string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: 오류가 나야 한다", what)
		}
		if msg := err.Error(); strings.ContainsAny(msg, `\/:`) || (user != "" && strings.Contains(strings.ToLower(msg), strings.ToLower(user))) {
			t.Errorf("%s: 사용자 문장에 경로나 사용자 이름이 있다: %q", what, msg)
		}
	}

	// 1) 없는 폴더에 쓰기: os.WriteFile의 PathError
	err := a.ExportGuide(gid, "html", "", choose(a, filepath.Join(t.TempDir(), "없는 폴더", "x.html")))
	clean("없는 폴더", err)
	if !errors.Is(err, errExportWrite) {
		t.Errorf("없는 폴더 = %v, errExportWrite여야 한다", err)
	}

	// 2) 렌더러 실패: 임시 폴더가 없으면 PDF 렌더러가 인쇄 전에(WebView2를 띄우기 전에) 경로 든 PathError를 낸다
	missing := filepath.Join(t.TempDir(), "없는 임시 폴더")
	t.Setenv("TMP", missing)
	t.Setenv("TEMP", missing)
	if d, err := os.MkdirTemp("", "probe-"); err == nil {
		os.Remove(d)
		t.Fatal("TMP를 바꿔도 임시 폴더가 만들어진다. 이 방법으로는 렌더러 실패를 만들 수 없다")
	}
	doc, err := export.Build(a.st, gid, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, raw := export.PDF(doc, export.DefaultOptions()); raw == nil || !strings.Contains(raw.Error(), missing) {
		t.Fatalf("렌더러 오류 %v, 경로가 든 오류여야 한다(전제)", raw)
	}
	err = a.ExportGuide(gid, "pdf", "", choose(a, filepath.Join(t.TempDir(), "x.pdf")))
	clean("PDF 렌더러", err)
	if !errors.Is(err, errExport) {
		t.Errorf("PDF 렌더러 실패 = %v, errExport여야 한다", err)
	}
}

// ExportGuide는 ChooseExportFile이 막 돌려준 경로만 받는다(검수 Minor 7). 안 고른 경로(저장 창을 거치지 않은 경로)는
// 거절하고, 고른 경로는 그대로 진행한다. 등록한 경로는 한 번 쓰면 비워 재사용하지 못한다.
func TestApp_ExportGuideOnlyChosenPath(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	renderAll(t, a, gid)
	dir := t.TempDir()
	notChosen := filepath.Join(dir, "x.html")
	if err := a.ExportGuide(gid, "html", "", notChosen); !errors.Is(err, errExportWrite) {
		t.Errorf("안 고른 경로 = %v, errExportWrite여야 한다", err)
	}
	chosen := choose(a, filepath.Join(dir, "y.html"))
	if err := a.ExportGuide(gid, "html", "", chosen); err != nil {
		t.Errorf("고른 경로 = %v", err)
	}
	// 한 번 쓰면 비운다: 같은 경로를 다시 내밀어도 더는 통과하지 않는다.
	if err := a.ExportGuide(gid, "html", "", chosen); !errors.Is(err, errExportWrite) {
		t.Errorf("다시 쓴 경로 = %v, errExportWrite여야 한다(1회용 표)", err)
	}
}

func TestApp_Presets(t *testing.T) {
	a, _ := fixture(t, t.TempDir())
	defer a.st.Close()
	ps, err := a.ListPresets()
	if err != nil || len(ps) != 1 || ps[0].Name != "기본" || ps[0].Format != "pdf" {
		t.Fatalf("처음 문서 모양 = %+v, %v", ps, err)
	}
	var o export.Options
	if err := json.Unmarshal([]byte(ps[0].Options), &o); err != nil || o != export.DefaultOptions() {
		t.Errorf("기본 문서 모양 옵션 = %+v, %v (기본값으로 채워져야 한다)", o, err)
	}
	if err := a.DeletePreset(ps[0].ID); !errors.Is(err, errPresetDefault) {
		t.Errorf("기본 문서 모양 지우기 = %v", err)
	}
	// 재정 M3-C21: '기본'은 고쳐 쓰지도 못한다
	if _, err := a.SavePreset(store.Preset{ID: ps[0].ID, Name: "기본", Format: "hwpx"}); !errors.Is(err, errPresetDefault) {
		t.Errorf("기본 문서 모양 고치기 = %v", err)
	}
	if ps2, _ := a.ListPresets(); ps2[0].Format != "pdf" {
		t.Errorf("거부한 저장이 쓰였다: %+v", ps2[0])
	}
	id, err := a.SavePreset(store.Preset{Name: "  보고용  ", Format: "hwpx", Options: `{"paper":"Letter","imageWidth":500,"font":"<b>"}`})
	if err != nil {
		t.Fatal(err)
	}
	ps, _ = a.ListPresets()
	if len(ps) != 2 || ps[1].ID != id || ps[1].Name != "보고용" || ps[1].Format != "hwpx" {
		t.Fatalf("저장 뒤 = %+v", ps)
	}
	_ = json.Unmarshal([]byte(ps[1].Options), &o)
	if o.Paper != "Letter" || o.ImageWidth != 100 || o.Font != "" || !o.Cover {
		t.Errorf("저장한 옵션 = %+v (범위 밖 값은 맞추고 빠진 칸은 기본값)", o)
	}
	if _, err := a.SavePreset(store.Preset{ID: id, Name: "", Format: "hwpx"}); !errors.Is(err, errName) {
		t.Errorf("빈 이름 = %v", err)
	}
	if _, err := a.SavePreset(store.Preset{Name: "x", Format: "exe"}); !errors.Is(err, errFormat) {
		t.Errorf("모르는 형식 = %v", err)
	}
	if err := a.DeletePreset(id); err != nil {
		t.Fatal(err)
	}
	if err := a.DeletePreset(id); !errors.Is(err, errGone) {
		t.Errorf("두 번 지우기 = %v", err)
	}
}

func TestApp_ChooseExportFileRejectsUnknownFormat(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	if _, err := a.ChooseExportFile(gid, "exe"); !errors.Is(err, errFormat) {
		t.Errorf("모르는 형식 = %v", err)
	}
	if _, err := a.ChooseExportFile(gid, "pdf"); !errors.Is(err, errNoWindow) {
		t.Errorf("창 없이 = %v, errNoWindow여야 한다", err) // 저장 창은 앱 실행 확인(Task 11)에서 본다
	}
}

// 안 쓰는 원본 그림과 합성 그림 캐시 정리(재정 M3-C3). 녹화 중에는 건너뛴다: 녹화기는 PutImage 뒤에 AppendStep을 해서
// 그 사이의 새 그림이 고아로 보인다. 진짜 녹화 대신 녹화기 자리만 채운다(app_test.go fakeStart와 같다).
func TestApp_PruneSkipsWhileRecording(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	renderAll(t, a, gid)
	jobs, _ := a.ExportPlan(gid)
	live, _ := a.st.RenderedFile(jobs[0].Key)
	used, _ := a.st.ImageFile(jobs[0].ImageSHA)
	_, sha, err := a.st.PutImage(encode(t, "png", 3, 3), 3, 3) // 녹화기가 저장했지만 아직 스텝을 쓰지 않은 그림
	if err != nil {
		t.Fatal(err)
	}
	orphan, _ := a.st.ImageFile(sha)
	stale := strings.Repeat("0", 64) // 예전 그리기의 합성 그림
	if err := a.st.PutRendered(stale, encode(t, "png", 2, 2)); err != nil {
		t.Fatal(err)
	}
	old, _ := a.st.RenderedFile(stale)

	a.rec = new(recorder.Recorder)
	a.prune()
	if !exists(orphan) || !exists(old) {
		t.Fatal("녹화 중에 그림을 지웠다")
	}
	a.rec = nil
	a.prune()
	if exists(orphan) || exists(old) {
		t.Error("녹화가 끝났는데 안 쓰는 그림이 남았다")
	}
	if !exists(used) || !exists(live) {
		t.Fatal("쓰는 그림을 지웠다")
	}
	// 매뉴얼을 지우면 그 그림과 합성 그림도 정리한다
	if err := a.DeleteGuide(gid); err != nil {
		t.Fatal(err)
	}
	if exists(used) || exists(live) {
		t.Error("지운 매뉴얼의 그림이 남았다")
	}
}

// 지운 표시만 된 스텝의 합성 그림은 되살릴 수 있어 남기고, CloseGuide가 스텝을 영구히 지우면 정리한다(재정 M3-C3 부르는 곳).
func TestApp_CloseGuidePrunesDeletedStepCache(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	d, _ := a.OpenGuide(gid)
	s1 := d.Steps[0]
	s1.AnnotationsJSON = "[]" // 나머지 둘과 다른 합성 그림이 되게 한다
	if err := a.SaveSteps(gid, []store.Step{s1}); err != nil {
		t.Fatal(err)
	}
	renderAll(t, a, gid)
	own, _ := a.st.RenderedFile(export.RenderKey(s1))
	shared, _ := a.st.RenderedFile(export.RenderKey(d.Steps[1]))
	if !exists(own) || !exists(shared) {
		t.Fatal("합성 그림을 올리지 못했다")
	}
	s1.Deleted = true
	if err := a.SaveSteps(gid, []store.Step{s1}); err != nil {
		t.Fatal(err)
	}
	a.prune()
	if !exists(own) {
		t.Fatal("지운 표시만 된 스텝(되살릴 수 있음)의 합성 그림을 지웠다")
	}
	if err := a.CloseGuide(gid); err != nil {
		t.Fatal(err)
	}
	if exists(own) {
		t.Error("CloseGuide 뒤에도 영구히 지운 스텝의 합성 그림이 남았다")
	}
	if !exists(shared) {
		t.Error("남은 스텝의 합성 그림을 지웠다")
	}
}

// 그림 넣기와 합성 그림 올리기는 정리(prune)와 같은 a.mu를 잡아, 정리가 PutImage와 InsertStep 사이에 끼지 못한다(검수 I1).
// 그 틈에 정리를 끼워 넣을 고리는 없어서, 정리가 a.mu를 쥔 동안 두 바인딩이 끝나지 않고 기다리는지 본다.
// 잠금을 빼면 바로 끝나 실패하고, 잠금이 있으면 늦게 돌아도 실패하지 않는다(시간에 기대는 쪽은 놓치는 방향뿐이다).
func TestApp_ImageWritesWaitForPrune(t *testing.T) {
	a, gid := fixture(t, t.TempDir())
	defer a.st.Close()
	jobs, _ := a.ExportPlan(gid)
	pasted := base64.StdEncoding.EncodeToString(encode(t, "png", 5, 4)) // 고정 가이드에 없는 새 그림
	render := base64.StdEncoding.EncodeToString(encode(t, "png", 8, 6))
	done := make(chan error, 2)
	var pic store.Step
	a.mu.Lock() // 정리가 도는 중
	go func() {
		var err error
		pic, err = a.AddImageStep(gid, 9, 0, pasted)
		done <- err
	}()
	go func() { done <- a.PutRendered(jobs[0].Key, render) }()
	select {
	case err := <-done:
		a.mu.Unlock()
		t.Fatalf("정리가 a.mu를 쥔 동안 끝났다(%v). 기다려야 한다", err)
	case <-time.After(100 * time.Millisecond):
	}
	a.mu.Unlock()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	a.prune()
	f, _ := a.st.ImageFile(pic.ImageSHA)
	d, _ := a.OpenGuide(gid)
	if !exists(f) || len(d.Steps) != 4 || d.Steps[3].ImageSHA != pic.ImageSHA {
		t.Errorf("넣은 그림 스텝이 정리 뒤 남아야 한다: 파일 %v, 스텝 %d개", exists(f), len(d.Steps))
	}
	if p, ok := a.st.RenderedFile(jobs[0].Key); !ok || !exists(p) {
		t.Error("올린 합성 그림이 정리 뒤 남아야 한다")
	}
}

// '폴더 열기'는 마지막으로 내보낸 경로만 연다. 빈 경로나 다른 경로는 탐색기를 띄우지 않고 거부한다.
// 탐색기를 띄우는 경로는 테스트하지 않는다(바탕 화면을 사용자와 함께 쓴다).
func TestApp_RevealFileOnlyLastExport(t *testing.T) {
	a := &App{}
	for _, p := range []string{"", `C:\Windows\System32\cmd.exe`} {
		if err := a.RevealFile(p); !errors.Is(err, errReveal) {
			t.Errorf("RevealFile(%q) = %v, errReveal여야 한다", p, err)
		}
	}
	a.lastExport = `C:\x\a.pdf`
	if err := a.RevealFile(`C:\x\b.pdf`); !errors.Is(err, errReveal) {
		t.Errorf("다른 경로 = %v, errReveal여야 한다", err)
	}
}
