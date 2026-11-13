//go:build windows

package webview

import (
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// HRESULT values and COM constants used here.
const (
	sOK             = 0
	sFalse          = 1
	eNoInterface    = 0x80004002
	ePointer        = 0x80004003
	eFail           = 0x80004005
	rpcEChangedMode = 0x80010106
	coinitApartment = 0x2 // COINIT_APARTMENTTHREADED

	// IUnknown, the first three vtable slots of every COM interface.
	iUnknownQueryInterface = 0
	iUnknownAddRef         = 1
	iUnknownRelease        = 2

	ptrSize = unsafe.Sizeof(uintptr(0))
)

var (
	ole32           = syscall.NewLazyDLL("ole32.dll")
	pCoInitializeEx = ole32.NewProc("CoInitializeEx")
	pCoTaskMemFree  = ole32.NewProc("CoTaskMemFree")

	iidIUnknown = guid{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}

	handlerVtbl     [4]uintptr // QueryInterface, AddRef, Release, Invoke: shared by all handlers
	handlerVtblOnce sync.Once
	handlersMu      sync.Mutex
	handlers        = map[uintptr]*handler{} // keeps every live handler reachable for the GC
)

// guid has the memory layout of a Windows GUID.
type guid struct {
	d1 uint32
	d2 uint16
	d3 uint16
	d4 [8]byte
}

// at turns an address handed over by native code into a pointer. The memory is not Go
// memory, so the garbage collector never moves or frees it.
func at(addr uintptr) unsafe.Pointer {
	return unsafe.Add(unsafe.Pointer(nil), addr)
}

// comCall calls method number idx of the COM object obj and returns its HRESULT (or its
// 32-bit result for AddRef/Release). Pointers converted to uintptr in the argument list
// are kept alive and unmoved for the duration of the call.
//
//go:uintptrescapes
func comCall(obj uintptr, idx uintptr, args ...uintptr) int32 {
	if obj == 0 {
		return hresult(ePointer)
	}
	vtbl := *(*uintptr)(at(obj))
	fn := *(*uintptr)(at(vtbl + idx*ptrSize))
	all := make([]uintptr, 0, 8)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return hresult(r)
}

// hresult takes the 32-bit HRESULT out of a register-sized result.
func hresult(r uintptr) int32 { return int32(uint32(r)) }

// comRelease releases the object *p points at and clears it.
func comRelease(p *uintptr) {
	if *p != 0 {
		comCall(*p, iUnknownRelease)
		*p = 0
	}
}

// coInit puts the calling thread into a single-threaded apartment. A thread that is
// already initialised (S_FALSE), also in another mode (RPC_E_CHANGED_MODE), is accepted.
func coInit() error {
	r, _, _ := pCoInitializeEx.Call(0, coinitApartment)
	switch uint32(r) {
	case sOK, sFalse, rpcEChangedMode:
		return nil
	}
	return hresultError("CoInitializeEx", hresult(r))
}

func hresultError(step string, hr int32) error {
	return fmt.Errorf("webview: %s failed: HRESULT 0x%08X", step, uint32(hr))
}

// takeString copies a NUL-terminated UTF-16 string allocated by COM and frees it.
func takeString(addr uintptr) string {
	if addr == 0 {
		return ""
	}
	n := 0
	for *(*uint16)(at(addr + uintptr(n)*2)) != 0 {
		n++
	}
	s := syscall.UTF16ToString(unsafe.Slice((*uint16)(at(addr)), n))
	pCoTaskMemFree.Call(addr)
	return s
}

// handler is a COM object implemented in Go: its first word points at a vtable of
// callbacks, which is all native code looks at. One type serves the three handler
// interfaces used here, because each of them is IUnknown plus a single Invoke taking two
// pointer-sized arguments.
type handler struct {
	vtbl   *[4]uintptr // must stay the first field
	refs   int32
	iid    *guid
	invoke func(a, b uintptr)
}

// newHandler makes a handler answering to iid, holding one reference owned by the caller.
func newHandler(iid *guid, invoke func(a, b uintptr)) *handler {
	handlerVtblOnce.Do(func() {
		// Callback slots are limited and never freed, so these four are made once.
		handlerVtbl[0] = syscall.NewCallback(handlerQueryInterface)
		handlerVtbl[1] = syscall.NewCallback(handlerAddRef)
		handlerVtbl[2] = syscall.NewCallback(handlerRelease)
		handlerVtbl[3] = syscall.NewCallback(handlerInvoke)
	})
	h := &handler{vtbl: &handlerVtbl, refs: 1, iid: iid, invoke: invoke}
	handlersMu.Lock()
	handlers[h.addr()] = h
	handlersMu.Unlock()
	return h
}

// addr is the interface pointer given to native code.
func (h *handler) addr() uintptr { return uintptr(unsafe.Pointer(h)) }

func (h *handler) addRef() int32 { return atomic.AddInt32(&h.refs, 1) }

// release drops one reference; with the last one the handler stops being kept alive.
func (h *handler) release() int32 {
	n := atomic.AddInt32(&h.refs, -1)
	if n == 0 {
		handlersMu.Lock()
		delete(handlers, h.addr())
		handlersMu.Unlock()
	}
	return n
}

func findHandler(this uintptr) *handler {
	handlersMu.Lock()
	h := handlers[this]
	handlersMu.Unlock()
	return h
}

func handlerQueryInterface(this, riid, out uintptr) uintptr {
	if out == 0 {
		return ePointer
	}
	h := findHandler(this)
	if h == nil || riid == 0 {
		*(*uintptr)(at(out)) = 0
		return eNoInterface
	}
	want := *(*guid)(at(riid))
	if want != iidIUnknown && want != *h.iid {
		*(*uintptr)(at(out)) = 0
		return eNoInterface
	}
	h.addRef()
	*(*uintptr)(at(out)) = this
	return sOK
}

func handlerAddRef(this uintptr) uintptr {
	if h := findHandler(this); h != nil {
		return uintptr(h.addRef())
	}
	return 1
}

func handlerRelease(this uintptr) uintptr {
	if h := findHandler(this); h != nil {
		return uintptr(h.release())
	}
	return 0
}

func handlerInvoke(this, a, b uintptr) uintptr {
	if h := findHandler(this); h != nil {
		h.invoke(a, b)
	}
	return sOK
}
