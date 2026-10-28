// 앱 상태 테스트. 녹화 상태 잠금은 go test -race로 돌려 경합이 없는지 본다(M0 리뷰: 버튼을 연달아 누르면 a.hook 경합).
package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"howtonote/internal/export"
	"howtonote/internal/recorder"
	"howtonote/internal/store"
)

func fakeStart() (*recorder.Recorder, error) { return new(recorder.Recorder), nil }

func TestApp_BeginIsExclusiveUnderConcurrency(t *testing.T) {
	a := &App{}
	var began, ended atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			if a.begin(fakeStart) == nil {
				began.Add(1)
			}
		}()
		go func() {
			defer wg.Done()
			if a.endRecording() {
				ended.Add(1)
			}
		}()
		go func() {
			defer wg.Done()
			_ = a.current()
		}()
	}
	wg.Wait()
	still := int32(0)
	if a.current() != nil {
		still = 1
	}
	// 녹화기는 비어 있을 때만 생기고 차 있을 때만 비워진다: 시작 수 = 끝 수 + 남은 것
	if began.Load() != ended.Load()+still {
		t.Fatalf("시작 %d, 끝 %d, 남음 %d", began.Load(), ended.Load(), still)
	}
}

func TestApp_BeginRefusesWhileRecording(t *testing.T) {
	a := &App{}
	if err := a.begin(fakeStart); err != nil {
		t.Fatal(err)
	}
	if err := a.begin(fakeStart); !errors.Is(err, errRecording) {
		t.Fatalf("녹화 중에 다시 시작하면 errRecording이어야 한다: %v", err)
	}
	if !a.endRecording() {
		t.Fatal("녹화기를 비우지 못했다")
	}
	if err := a.begin(fakeStart); err != nil {
		t.Fatalf("끝낸 뒤에는 다시 시작할 수 있어야 한다: %v", err)
	}
}

// TestApp_BindingWaitsForStartup은 R21을 본다: Wails v2.16은 페이지를 먼저 띄우고 OnStartup을 고루틴으로 돌려,
// 첫 화면의 ListGuides가 startup보다 먼저 올 수 있다. 바인딩은 startup이 끝날 때까지 기다렸다가 준비된 저장소를 써야 한다.
// -race로 돌려 startup이 채운 필드를 바인딩이 경합 없이 읽는지도 본다.
func TestApp_BindingWaitsForStartup(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir()) // 개발 PC의 진짜 %APPDATA%\HowtoNote를 건드리지 않는다
	a := NewApp()
	got := make(chan error, 1)
	go func() {
		_, err := a.ListGuides()
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("startup 전에 돌아왔다(오류 %v): 기다려야 한다", err)
	case <-time.After(100 * time.Millisecond):
	}
	a.startup(context.Background())
	defer a.shutdown(context.Background())
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("startup 뒤 목록 오류: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("startup이 끝났는데 바인딩이 돌아오지 않았다")
	}
}

// APPDATA가 비면 작업 폴더에 데이터를 만들지 말고 설정 오류를 알려야 한다.
func TestApp_StartupWithoutAPPDATA(t *testing.T) {
	t.Setenv("APPDATA", "")
	a := NewApp()
	a.startup(context.Background())
	defer a.shutdown(context.Background())
	if _, err := a.ListGuides(); !errors.Is(err, errSettings) {
		t.Fatalf("오류 %v, errSettings여야 한다", err)
	}
}

func TestApp_ServeAsset(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, sha, err := st.PutImage([]byte("png-bytes"), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{st: st}
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		a.serveAsset(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	if r := get("/img/" + sha + ".png"); r.Code != http.StatusOK || r.Body.String() != "png-bytes" {
		t.Errorf("이미지 응답 %d %q", r.Code, r.Body.String())
	}
	for _, p := range []string{"/img/../x.png", "/img/abc.png", "/frozen.png", "/other"} {
		if r := get(p); r.Code != http.StatusNotFound {
			t.Errorf("%s 응답 %d, 404여야 한다", p, r.Code)
		}
	}
	a.frozen = []byte("frozen")
	if r := get("/frozen.png"); r.Code != http.StatusOK || r.Body.String() != "frozen" {
		t.Errorf("멈춘 화면 응답 %d %q", r.Code, r.Body.String())
	}
}

// 앱 화면 글꼴은 내보내기에 넣어 둔 Pretendard를 그대로 받는다(UI 설계 4.2절). 모르는 이름은 404다.
func TestApp_ServeFont(t *testing.T) {
	a := &App{}
	for path, bold := range map[string]bool{"/fonts/Pretendard-Regular.ttf": false, "/fonts/Pretendard-Bold.ttf": true} {
		rec := httptest.NewRecorder()
		a.serveAsset(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "font/ttf" {
			t.Fatalf("%s 응답 %d %q", path, rec.Code, rec.Header().Get("Content-Type"))
		}
		if b := export.Font(bold); len(b) < 1000 || !bytes.Equal(rec.Body.Bytes(), b) {
			t.Errorf("%s 본문이 내보내기 글꼴과 다르다(%d바이트)", path, rec.Body.Len())
		}
	}
	rec := httptest.NewRecorder()
	a.serveAsset(rec, httptest.NewRequest(http.MethodGet, "/fonts/Other.ttf", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("모르는 글꼴 응답 %d, 404여야 한다", rec.Code)
	}
}

// 글꼴은 startup(저장소·UIA 준비)을 기다리지 않아야 한다: ready를 영영 닫지 않아도 응답이 와야 한다(F2).
func TestApp_ServeFontBeforeReady(t *testing.T) {
	a := &App{ready: make(chan struct{})} // 일부러 닫지 않는다
	got := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		a.serveAsset(rec, httptest.NewRequest(http.MethodGet, "/fonts/Pretendard-Regular.ttf", nil))
		got <- rec
	}()
	select {
	case rec := <-got:
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "font/ttf" {
			t.Fatalf("응답 %d %q", rec.Code, rec.Header().Get("Content-Type"))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("글꼴 응답이 startup을 기다리느라 2초 안에 오지 않았다")
	}
}
