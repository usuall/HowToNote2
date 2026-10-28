// WebView2 완료 핸들러·이벤트 핸들러를 Go로 구현하는 COM 객체 하나와 vtable 호출 도우미.
// WebView2의 핸들러는 모두 Invoke(this, a, b) 모양이라 객체 하나로 네 종류(컨트롤러 완료, CDP 완료, 탐색 완료, 리소스 요청)를 다 쓴다.
// 환경 완료 핸들러는 go-webview2 webviewloader(combridge)가 따로 만든다.
package printer

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"

	"howtonote/internal/win"
)

const (
	sOK          = 0
	eNoInterface = 0x80004002
	ePointer     = 0x80004003
)

// WebView2.h(Microsoft.Web.WebView2 1.0.2903.40)의 MIDL_INTERFACE 값이다.
var (
	iidIUnknown             = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidControllerCompleted  = windows.GUID{Data1: 0x6c4819f3, Data2: 0xc9b7, Data3: 0x4260, Data4: [8]byte{0x81, 0x27, 0xc9, 0xf5, 0xbd, 0xe7, 0xf6, 0x8c}}
	iidDevToolsCompleted    = windows.GUID{Data1: 0x5c4889f0, Data2: 0x5ef6, Data3: 0x4c5a, Data4: [8]byte{0x95, 0x2c, 0xd8, 0xf1, 0xb9, 0x2d, 0x05, 0x74}}
	iidNavigationCompleted  = windows.GUID{Data1: 0xd33a35bf, Data2: 0x1c49, Data3: 0x4f98, Data4: [8]byte{0x93, 0xab, 0x00, 0x6e, 0x05, 0x33, 0xfe, 0x1c}}
	iidWebResourceRequested = windows.GUID{Data1: 0xab00b74c, Data2: 0x15f1, Data3: 0x4646, Data4: [8]byte{0x80, 0xe8, 0xe7, 0x63, 0x41, 0xd2, 0x5d, 0x71}}
	iidICoreWebView2_3      = windows.GUID{Data1: 0xA0D6DF20, Data2: 0x3B92, Data3: 0x416D, Data4: [8]byte{0xAA, 0x0C, 0x43, 0x7A, 0x9C, 0x72, 0x78, 0x57}}
)

type handlerVtbl struct {
	QueryInterface, AddRef, Release, Invoke uintptr
}

// handler는 WebView2에 넘기는 COM 객체다. 첫 칸이 vtable 포인터여야 한다.
// WebView2는 핸들러를 AddRef해 들고 있다가 아무 때나(인쇄 스레드가 끝난 뒤 다른 스레드에서도) Release할 수 있다.
// 그래서 참조 수를 세고, 0이 될 때까지 live가 붙들어 GC가 메모리를 거두지 못하게 한다.
// 처음 참조 하나는 인쇄 세션 몫이고 session.close가 놓는다.
type handler struct {
	vtbl *handlerVtbl
	iid  *windows.GUID
	fn   func(a uintptr, b unsafe.Pointer)
	refs atomic.Int32
}

var (
	liveMu sync.Mutex
	live   = map[*handler]struct{}{} // 참조가 남은 핸들러
)

func newHandler(iid *windows.GUID, fn func(a uintptr, b unsafe.Pointer)) *handler {
	h := &handler{vtbl: theVtbl, iid: iid, fn: fn}
	h.refs.Store(1)
	liveMu.Lock()
	live[h] = struct{}{}
	liveMu.Unlock()
	return h
}

// unref는 참조 하나를 놓고 남은 수를 돌려준다. 0이 되면 live에서 빼 GC가 거둘 수 있게 한다.
func (h *handler) unref() uintptr {
	n := h.refs.Add(-1)
	if n == 0 {
		liveMu.Lock()
		delete(live, h)
		liveMu.Unlock()
	}
	return uintptr(n)
}

// 콜백은 프로세스 전체에서 개수 제한이 있어(windows.NewCallback) 패키지에서 한 번만 만든다.
var theVtbl = &handlerVtbl{
	QueryInterface: windows.NewCallback(func(this *handler, iid, out uintptr) uintptr {
		if out == 0 {
			return ePointer
		}
		if iid == 0 {
			win.StorePtr(out, 0)
			return ePointer
		}
		var g windows.GUID
		win.CopyFrom(unsafe.Pointer(&g), iid, unsafe.Sizeof(g))
		if g == iidIUnknown || g == *this.iid {
			this.refs.Add(1)
			win.StorePtr(out, uintptr(unsafe.Pointer(this)))
			return sOK
		}
		win.StorePtr(out, 0)
		return eNoInterface
	}),
	AddRef:  windows.NewCallback(func(this *handler) uintptr { return uintptr(this.refs.Add(1)) }),
	Release: windows.NewCallback(func(this *handler) uintptr { return this.unref() }),
	Invoke: windows.NewCallback(func(this *handler, a uintptr, b unsafe.Pointer) uintptr {
		this.fn(a, b)
		return sOK
	}),
}

// release는 COM 객체의 참조를 하나 놓는다(IUnknown::Release, slot 2). nil이면 아무것도 하지 않는다.
func release(obj unsafe.Pointer) {
	if obj != nil {
		win.ComCall(obj, 2)
	}
}
