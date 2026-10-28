// 숨긴 WebView2에서 CDP Page.printToPDF로 PDF를 만든다. 외부 브라우저 프로세스를 띄우지 않는다(스펙 7.2절).
// go-webview2의 Embed는 초기화에 실패하면 os.Exit하거나 끝없이 기다려서, 환경·컨트롤러를 직접 만들고 deadline까지만 기다린다.
package printer

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"

	"howtonote/internal/win"
)

type Options struct {
	HeaderHTML string // CDP headerTemplate. font-size를 직접 지정해야 보인다
	FooterHTML string // pageNumber, totalPages 클래스를 쓸 수 있다
	Landscape  bool
}

// virtualHost는 RFC 6761이 예약한 .example 이름이라 실제 인터넷 주소와 겹치지 않는다.
const virtualHost = "howtonote.example"

const (
	slotEnvCreateController    = 3  // ICoreWebView2Environment::CreateCoreWebView2Controller
	slotEnvCreateResponse      = 4  // ICoreWebView2Environment::CreateWebResourceResponse
	slotCtlClose               = 24 // ICoreWebView2Controller::Close
	slotCtlGetWebView          = 25 // ICoreWebView2Controller::get_CoreWebView2
	slotWVNavigate             = 5
	slotWVAddNavCompleted      = 15
	slotWVRemoveNavCompleted   = 16
	slotWVCallDevTools         = 36
	slotWVAddResourceRequested = 55
	slotWVRemoveResRequested   = 56
	slotWVAddResourceFilter    = 57
	slotWV3SetVirtualHost      = 71 // ICoreWebView2_3::SetVirtualHostNameToFolderMapping
	slotNavArgsIsSuccess       = 3
	slotResArgsGetRequest      = 3
	slotResArgsPutResponse     = 5
	slotRequestGetUri          = 3
	hostAccessAllow            = 1 // COREWEBVIEW2_HOST_RESOURCE_ACCESS_KIND_ALLOW
	resourceContextAll         = 0 // COREWEBVIEW2_WEB_RESOURCE_CONTEXT_ALL
)

var (
	ErrNoRuntime = errors.New("printer: WebView2 런타임을 찾지 못했다")
	ErrTimeout   = errors.New("printer: 시간 초과")
	blockedTotal atomic.Int32 // 막은 요청 수(테스트가 본다)
	printMu      sync.Mutex
)

// PrintDir는 dir을 가상 호스트로 열어 index를 PDF로 인쇄한다. 인쇄마다 새 STA 스레드를 쓰고 끝나면 버린다.
// 인쇄 스레드가 COM 호출 안에서 멈춰도 timeout 뒤에는 돌아온다(printMu를 쥔 채 멈추지 않는다).
func PrintDir(dir, index string, opt Options, timeout time.Duration) ([]byte, error) {
	printMu.Lock()
	defer printMu.Unlock()
	type out struct {
		pdf []byte
		err error
	}
	ch := make(chan out, 1) // 늦게 끝난 스레드가 막히지 않게 한 칸
	deadline := time.Now().Add(timeout)
	go func() {
		runtime.LockOSThread() // 풀지 않는다: 고루틴이 끝나면 OS 스레드도 끝나 남은 COM 상태가 함께 사라진다
		pdf, err := printOnThisThread(dir, index, opt, deadline)
		ch <- out{pdf, err}
	}()
	select {
	case r := <-ch:
		return r.pdf, r.err
	case <-time.After(timeout + 5*time.Second):
		return nil, ErrTimeout
	}
}

// dataDir는 인쇄용 WebView2 사용자 데이터 폴더다. 앱 본창(Wails)과 다른 폴더라 서로 설정이 섞이지 않는다.
func dataDir() string {
	base, err := os.UserCacheDir()
	if err != nil { // LOCALAPPDATA가 비었으면 오류다
		base = os.TempDir()
	}
	return filepath.Join(base, "HowtoNote", "print-webview")
}

// session은 인쇄 한 번이 쥐는 COM 객체와 핸들러의 세션 몫 참조다.
type session struct {
	deadline       time.Time
	env, ctl, wv   unsafe.Pointer
	wv3            unsafe.Pointer
	handlers       []*handler
	resTok, navTok *int64 // 등록에 성공한 이벤트 토큰(EventRegistrationToken)
}

// close는 이벤트를 떼고 컨트롤러를 닫은 뒤 COM 참조와 핸들러의 세션 몫 참조를 놓는다.
// 정리 단계라 실패해도 할 일이 없어 HRESULT는 보지 않는다.
func (s *session) close() {
	// EventRegistrationToken은 8바이트 구조체라 x64에서는 값 자체가 레지스터로 넘어간다(포인터가 아니다).
	if s.resTok != nil {
		win.ComCall(s.wv, slotWVRemoveResRequested, uintptr(*s.resTok))
	}
	if s.navTok != nil {
		win.ComCall(s.wv, slotWVRemoveNavCompleted, uintptr(*s.navTok))
	}
	if s.ctl != nil {
		win.ComCall(s.ctl, slotCtlClose)
	}
	release(s.wv3)
	release(s.wv)
	release(s.ctl)
	release(s.env)
	for _, h := range s.handlers {
		h.unref()
	}
}

type envWaiter struct {
	done bool
	hr   webviewloader.HRESULT
	env  unsafe.Pointer
}

func (w *envWaiter) EnvironmentCompleted(hr webviewloader.HRESULT, env *webviewloader.ICoreWebView2Environment) webviewloader.HRESULT {
	w.done, w.hr = true, hr
	if hr >= 0 && env != nil {
		p := unsafe.Pointer(env)
		win.ComCall(p, 1) // AddRef: 로더가 콜백 뒤에 Release한다
		w.env = p
	}
	return 0
}

func printOnThisThread(dir, index string, opt Options, deadline time.Time) ([]byte, error) {
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		return nil, fmt.Errorf("CoInitializeEx: %w", err)
	}
	defer windows.CoUninitialize()
	hwnd, err := win.CreateHiddenWindow(1000, 1400)
	if err != nil {
		return nil, err
	}
	defer win.ProcDestroyWindow.Call(hwnd)

	s := &session{deadline: deadline}
	defer s.close()

	ew := &envWaiter{}
	if err := webviewloader.CreateCoreWebView2EnvironmentWithOptions(ew, webviewloader.WithUserDataFolder(dataDir())); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoRuntime, err)
	}
	if !pumpUntil(func() bool { return ew.done }, deadline) {
		return nil, fmt.Errorf("WebView2 환경 만들기: %w", ErrTimeout)
	}
	if ew.env == nil {
		return nil, fmt.Errorf("WebView2 환경 만들기 실패: HRESULT 0x%08X", uint32(ew.hr))
	}
	s.env = ew.env

	if err := s.createController(hwnd); err != nil {
		return nil, err
	}
	if err := s.navigate(dir, index); err != nil {
		return nil, err
	}
	params, _ := json.Marshal(map[string]any{
		"printBackground":         true,
		"preferCSSPageSize":       true,
		"displayHeaderFooter":     opt.HeaderHTML != "" || opt.FooterHTML != "",
		"headerTemplate":          orEmpty(opt.HeaderHTML),
		"footerTemplate":          orEmpty(opt.FooterHTML),
		"landscape":               opt.Landscape,
		"generateTaggedPDF":       true,
		"generateDocumentOutline": true,
	})
	res, err := s.callDevTools("Page.printToPDF", string(params))
	if err != nil {
		return nil, err
	}
	var r struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal([]byte(res), &r); err != nil {
		return nil, fmt.Errorf("CDP 응답 해석: %w", err)
	}
	if r.Data == "" {
		return nil, errors.New("CDP Page.printToPDF가 빈 PDF를 돌려줬다")
	}
	return base64.StdEncoding.DecodeString(r.Data)
}

func (s *session) createController(hwnd uintptr) error {
	done, hr := false, uintptr(0)
	h := newHandler(&iidControllerCompleted, func(a uintptr, b unsafe.Pointer) {
		done, hr = true, a
		if !win.Failed(a) && b != nil {
			win.ComCall(b, 1) // AddRef: 콜백이 끝난 뒤에도 쓴다
			s.ctl = b
		}
	})
	s.handlers = append(s.handlers, h)
	if r := win.ComCall(s.env, slotEnvCreateController, hwnd, uintptr(unsafe.Pointer(h))); win.Failed(r) {
		return fmt.Errorf("CreateCoreWebView2Controller: HRESULT 0x%08X", uint32(r))
	}
	if !pumpUntil(func() bool { return done }, s.deadline) {
		return fmt.Errorf("WebView2 컨트롤러 만들기: %w", ErrTimeout)
	}
	if s.ctl == nil {
		return fmt.Errorf("WebView2 컨트롤러 만들기 실패: HRESULT 0x%08X", uint32(hr))
	}
	wv := new(unsafe.Pointer)
	if r := win.ComCall(s.ctl, slotCtlGetWebView, uintptr(unsafe.Pointer(wv))); win.Failed(r) || *wv == nil {
		return fmt.Errorf("get_CoreWebView2: HRESULT 0x%08X", uint32(r))
	}
	s.wv = *wv
	wv3 := new(unsafe.Pointer)
	if r := win.ComCall(s.wv, 0, uintptr(unsafe.Pointer(&iidICoreWebView2_3)), uintptr(unsafe.Pointer(wv3))); win.Failed(r) || *wv3 == nil {
		return fmt.Errorf("ICoreWebView2_3 없음(WebView2 런타임이 너무 오래되었다): HRESULT 0x%08X", uint32(r))
	}
	s.wv3 = *wv3
	return nil
}

// navigate는 dir을 가상 호스트에 연결하고 index를 연다. 가상 호스트 밖으로 나가는 요청은 모두 403으로 막는다(스펙 9절 네트워크 없음).
func (s *session) navigate(dir, index string) error {
	host, _ := windows.UTF16PtrFromString(virtualHost)
	folder, _ := windows.UTF16PtrFromString(dir)
	if r := win.ComCall(s.wv3, slotWV3SetVirtualHost, uintptr(unsafe.Pointer(host)), uintptr(unsafe.Pointer(folder)), hostAccessAllow); win.Failed(r) {
		return fmt.Errorf("가상 호스트 연결: HRESULT 0x%08X", uint32(r))
	}
	prefix := "https://" + virtualHost + "/"
	block := newHandler(&iidWebResourceRequested, func(_ uintptr, args unsafe.Pointer) {
		if !strings.HasPrefix(requestURI(args), prefix) {
			s.respond403(args)
		}
	})
	all, _ := windows.UTF16PtrFromString("*")
	s.handlers = append(s.handlers, block)
	if r := win.ComCall(s.wv, slotWVAddResourceFilter, uintptr(unsafe.Pointer(all)), resourceContextAll); win.Failed(r) {
		return fmt.Errorf("AddWebResourceRequestedFilter: HRESULT 0x%08X", uint32(r))
	}
	resTok := new(int64)
	if r := win.ComCall(s.wv, slotWVAddResourceRequested, uintptr(unsafe.Pointer(block)), uintptr(unsafe.Pointer(resTok))); win.Failed(r) {
		return fmt.Errorf("add_WebResourceRequested: HRESULT 0x%08X", uint32(r))
	}
	s.resTok = resTok

	navDone, navOK := false, false
	nav := newHandler(&iidNavigationCompleted, func(_ uintptr, args unsafe.Pointer) {
		ok := new(int32)
		r := win.ComCall(args, slotNavArgsIsSuccess, uintptr(unsafe.Pointer(ok)))
		navDone, navOK = true, !win.Failed(r) && *ok != 0
	})
	s.handlers = append(s.handlers, nav)
	navTok := new(int64)
	if r := win.ComCall(s.wv, slotWVAddNavCompleted, uintptr(unsafe.Pointer(nav)), uintptr(unsafe.Pointer(navTok))); win.Failed(r) {
		return fmt.Errorf("add_NavigationCompleted: HRESULT 0x%08X", uint32(r))
	}
	s.navTok = navTok
	url, _ := windows.UTF16PtrFromString(prefix + index)
	if r := win.ComCall(s.wv, slotWVNavigate, uintptr(unsafe.Pointer(url))); win.Failed(r) {
		return fmt.Errorf("Navigate: HRESULT 0x%08X", uint32(r))
	}
	runtime.KeepAlive(host)
	runtime.KeepAlive(folder)
	runtime.KeepAlive(all)
	runtime.KeepAlive(url)
	if !pumpUntil(func() bool { return navDone }, s.deadline) {
		return fmt.Errorf("페이지 열기: %w", ErrTimeout)
	}
	if !navOK {
		return errors.New("인쇄할 페이지를 열지 못했다")
	}
	return nil
}

// requestURI는 WebResourceRequested 인자의 요청 주소다. 못 읽으면 빈 문자열(그래서 막힌다).
func requestURI(args unsafe.Pointer) string {
	req := new(unsafe.Pointer)
	if r := win.ComCall(args, slotResArgsGetRequest, uintptr(unsafe.Pointer(req))); win.Failed(r) || *req == nil {
		return ""
	}
	defer release(*req)
	uri := new(*uint16)
	if r := win.ComCall(*req, slotRequestGetUri, uintptr(unsafe.Pointer(uri))); win.Failed(r) || *uri == nil {
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(*uri))
	return windows.UTF16PtrToString(*uri)
}

// respond403은 요청에 403 응답을 붙여 막는다. 응답을 만들거나 붙이지 못하면 요청은 그대로 나가지만,
// 인쇄할 HTML의 CSP(default-src 'none'; img-src data: 'self')가 둘째 벽으로 가상 호스트 밖 요청을 막는다.
// blockedTotal은 응답을 실제로 붙였을 때만 세어 테스트가 진짜 차단을 확인한다.
func (s *session) respond403(args unsafe.Pointer) {
	reason, _ := windows.UTF16PtrFromString("Blocked")
	headers, _ := windows.UTF16PtrFromString("")
	resp := new(unsafe.Pointer)
	r := win.ComCall(s.env, slotEnvCreateResponse, 0, 403, uintptr(unsafe.Pointer(reason)), uintptr(unsafe.Pointer(headers)), uintptr(unsafe.Pointer(resp)))
	runtime.KeepAlive(reason)
	runtime.KeepAlive(headers)
	if win.Failed(r) || *resp == nil {
		return
	}
	defer release(*resp)
	if r := win.ComCall(args, slotResArgsPutResponse, uintptr(*resp)); win.Failed(r) {
		return
	}
	blockedTotal.Add(1)
}

func (s *session) callDevTools(method, params string) (string, error) {
	m, _ := windows.UTF16PtrFromString(method)
	p, _ := windows.UTF16PtrFromString(params)
	done, hr, result := false, uintptr(0), ""
	h := newHandler(&iidDevToolsCompleted, func(a uintptr, b unsafe.Pointer) {
		done, hr = true, a
		if b != nil {
			result = windows.UTF16PtrToString((*uint16)(b))
		}
	})
	s.handlers = append(s.handlers, h)
	r := win.ComCall(s.wv, slotWVCallDevTools, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(h)))
	runtime.KeepAlive(m)
	runtime.KeepAlive(p)
	if win.Failed(r) {
		return "", fmt.Errorf("CallDevToolsProtocolMethod: HRESULT 0x%08X", uint32(r))
	}
	if !pumpUntil(func() bool { return done }, s.deadline) {
		return "", fmt.Errorf("PDF 만들기: %w", ErrTimeout)
	}
	if win.Failed(hr) {
		return "", fmt.Errorf("%s 실패: HRESULT 0x%08X %s", method, uint32(hr), result)
	}
	return result, nil
}

// orEmpty: 템플릿을 빈 문자열로 두면 Chromium 기본 머리글(날짜·제목)이 나오므로 빈 요소를 넣는다.
func orEmpty(s string) string {
	if s == "" {
		return "<span></span>"
	}
	return s
}

// pumpUntil은 WebView2 콜백이 이 스레드의 메시지로 오므로 메시지를 돌리며 기다린다.
func pumpUntil(cond func() bool, deadline time.Time) bool {
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		win.Pump()
		time.Sleep(5 * time.Millisecond)
	}
	return true
}
