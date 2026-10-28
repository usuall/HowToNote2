// handler COM 객체의 참조 수와 live 등록부: WebView2가 들고 있는 동안 GC가 거두지 못하고, 다 놓으면 빠진다.
package printer

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"howtonote/internal/win"
)

func isLive(h *handler) bool {
	liveMu.Lock()
	defer liveMu.Unlock()
	_, ok := live[h]
	return ok
}

// qi는 vtable을 거쳐 QueryInterface를 부른다(nil은 0으로 넘어간다). 변환을 호출식 안에 둬야 uintptrescapes가 out을 붙든다.
func qi(h *handler, iid *windows.GUID, out *uintptr) uintptr {
	return win.ComCall(unsafe.Pointer(h), 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(out)))
}

func TestHandlerRefcountAndRegistry(t *testing.T) {
	var gotA uintptr
	h := newHandler(&iidNavigationCompleted, func(a uintptr, _ unsafe.Pointer) { gotA = a })
	self := uintptr(unsafe.Pointer(h))
	if !isLive(h) || h.refs.Load() != 1 {
		t.Fatalf("새 핸들러: live=%v refs=%d, live·1이어야 한다", isLive(h), h.refs.Load())
	}
	if n := win.ComCall(unsafe.Pointer(h), 1); n != 2 { // AddRef
		t.Fatalf("AddRef = %d, 2여야 한다", n)
	}

	out := new(uintptr)
	if r := qi(h, &iidNavigationCompleted, out); r != sOK || *out != self || h.refs.Load() != 3 {
		t.Fatalf("자기 IID QI = 0x%X out=%v refs=%d, S_OK·자기·3이어야 한다", r, *out == self, h.refs.Load())
	}
	*out = 1
	if r := qi(h, &iidIUnknown, out); r != sOK || *out != self || h.refs.Load() != 4 {
		t.Fatalf("IUnknown QI = 0x%X refs=%d, S_OK·4여야 한다", r, h.refs.Load())
	}
	*out = 1
	if r := qi(h, &iidDevToolsCompleted, out); r != eNoInterface || *out != 0 || h.refs.Load() != 4 {
		t.Fatalf("다른 IID QI = 0x%X out=%d refs=%d, E_NOINTERFACE·0·4여야 한다", r, *out, h.refs.Load())
	}
	if r := qi(h, &iidNavigationCompleted, nil); r != ePointer {
		t.Fatalf("ppv가 NULL인 QI = 0x%X, E_POINTER여야 한다", r)
	}
	*out = 1
	if r := qi(h, nil, out); r != ePointer || *out != 0 || h.refs.Load() != 4 {
		t.Fatalf("riid가 NULL인 QI = 0x%X out=%d refs=%d, E_POINTER·0·4여야 한다", r, *out, h.refs.Load())
	}

	win.ComCall(unsafe.Pointer(h), 3, 7, 0) // Invoke
	if gotA != 7 {
		t.Fatalf("Invoke가 fn에 a=%d를 넘겼다, 7이어야 한다", gotA)
	}

	for want := uintptr(3); want >= 1; want-- { // WebView2 몫 Release 셋
		if n := win.ComCall(unsafe.Pointer(h), 2); n != want {
			t.Fatalf("Release = %d, %d여야 한다", n, want)
		}
		if !isLive(h) {
			t.Fatalf("참조 %d개가 남았는데 live에서 빠졌다", want)
		}
	}
	if n := h.unref(); n != 0 || isLive(h) { // 세션 몫
		t.Fatalf("마지막 참조를 놓은 뒤 refs=%d live=%v, 0·빠짐이어야 한다", n, isLive(h))
	}
}
