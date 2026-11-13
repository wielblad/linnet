//go:build windows

package toast

import (
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// HRESULT values and constants used here.
const (
	sOK             = 0
	sFalse          = 1
	eNoInterface    = 0x80004002
	ePointer        = 0x80004003
	rpcEChangedMode = 0x80010106

	roInitMultithreaded = 1 // RO_INIT_MULTITHREADED

	// IUnknown, the first three vtable slots of every COM interface. A WinRT interface
	// continues with IInspectable's three (GetIids, GetRuntimeClassName, GetTrustLevel:
	// inspectable.h), so its own first method sits in slot 6.
	iUnknownQueryInterface = 0
	iUnknownRelease        = 2
	afterInspectable       = 6

	ptrSize = unsafe.Sizeof(uintptr(0))
)

// Vtable slots. Each was read off the interface declarations of the Windows SDK
// 10.0.22621.0 headers named in the comments (method order in the header, plus six).
const (
	// IToastNotificationManagerStatics (windows.ui.notifications.h)
	managerCreateNotifierWithID = afterInspectable + 1 // CreateToastNotifierWithId(HSTRING, IToastNotifier**)

	// IToastNotificationManagerStatics2 (windows.ui.notifications.h)
	manager2GetHistory = afterInspectable + 0 // get_History(IToastNotificationHistory**)

	// IToastNotificationFactory (windows.ui.notifications.h)
	factoryCreate = afterInspectable + 0 // CreateToastNotification(IXmlDocument*, IToastNotification**)

	// IToastNotifier (windows.ui.notifications.h)
	notifierShow = afterInspectable + 0 // Show(IToastNotification*)
	notifierHide = afterInspectable + 1 // Hide(IToastNotification*)

	// IToastNotification (windows.ui.notifications.h)
	notificationAddActivated    = afterInspectable + 5 // add_Activated(handler, EventRegistrationToken*)
	notificationRemoveActivated = afterInspectable + 6 // remove_Activated(EventRegistrationToken)

	// IToastNotification2 (windows.ui.notifications.h)
	notification2PutTag   = afterInspectable + 0 // put_Tag(HSTRING)
	notification2PutGroup = afterInspectable + 2 // put_Group(HSTRING)

	// IToastNotificationHistory (windows.ui.notifications.h)
	historyRemoveGroupWithID      = afterInspectable + 1 // RemoveGroupWithId(group, applicationId)
	historyRemoveGroupedTagWithID = afterInspectable + 2 // RemoveGroupedTagWithId(tag, group, applicationId)

	// IToastActivatedEventArgs and IToastActivatedEventArgs2 (windows.ui.notifications.h)
	activatedArgsGetArguments  = afterInspectable + 0 // get_Arguments(HSTRING*)
	activatedArgs2GetUserInput = afterInspectable + 0 // get_UserInput(IPropertySet**)

	// IXmlDocumentIO (windows.data.xml.dom.h)
	xmlDocumentIOLoadXML = afterInspectable + 0 // LoadXml(HSTRING)

	// IMap<K, V> (IMap_impl in windows.foundation.collections.h)
	mapLookup = afterInspectable + 0 // Lookup(K, V*)

	// IPropertyValue (windows.foundation.h)
	propertyValueGetString = afterInspectable + 13 // GetString(HSTRING*)

	// ITypedEventHandler<TSender, TArgs> (ITypedEventHandler_impl in
	// windows.foundation.collections.h): a delegate is IUnknown plus Invoke(sender, args),
	// not an IInspectable.
	activatedHandlerInvoke   = 3
	activatedHandlerVtblSize = activatedHandlerInvoke + 1
)

// Runtime class names (the RuntimeClass_ constants of the same headers).
const (
	classXMLDocument              = "Windows.Data.Xml.Dom.XmlDocument"
	classToastNotification        = "Windows.UI.Notifications.ToastNotification"
	classToastNotificationManager = "Windows.UI.Notifications.ToastNotificationManager"
)

// replyInputID is the id of the text box in the toast and the key of its value in the
// event's user input.
const replyInputID = "reply"

// Interface identifiers: the MIDL_INTERFACE / __declspec(uuid) values of the same headers
// (IAgileObject: objidlbase.h). The two parameterised ones are the specialisations the
// headers spell out: IMap<HSTRING, IInspectable*> in windows.foundation.h and
// ITypedEventHandler<ToastNotification*, IInspectable*> in windows.ui.notifications.h.
var (
	iidIUnknown         = guid{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIAgileObject     = guid{0x94ea2b94, 0xe9cc, 0x49e0, [8]byte{0xc0, 0xff, 0xee, 0x64, 0xca, 0x8f, 0x5b, 0x90}}
	iidManagerStatics   = guid{0x50ac103f, 0xd235, 0x4598, [8]byte{0xbb, 0xef, 0x98, 0xfe, 0x4d, 0x1a, 0x3a, 0xd4}}
	iidManagerStatics2  = guid{0x7ab93c52, 0x0e48, 0x4750, [8]byte{0xba, 0x9d, 0x1a, 0x41, 0x13, 0x98, 0x18, 0x47}}
	iidFactory          = guid{0x04124b20, 0x82c6, 0x4229, [8]byte{0xb1, 0x09, 0xfd, 0x9e, 0xd4, 0x66, 0x2b, 0x53}}
	iidNotification2    = guid{0x9dfb9fd1, 0x143a, 0x490e, [8]byte{0x90, 0xbf, 0xb9, 0xfb, 0xa7, 0x13, 0x2d, 0xe7}}
	iidActivatedArgs    = guid{0xe3bf92f3, 0xc197, 0x436f, [8]byte{0x82, 0x65, 0x06, 0x25, 0x82, 0x4f, 0x8d, 0xac}}
	iidActivatedArgs2   = guid{0xab7da512, 0xcc61, 0x568e, [8]byte{0x81, 0xbe, 0x30, 0x4a, 0xc3, 0x10, 0x38, 0xfa}}
	iidXMLDocument      = guid{0xf7f3a506, 0x1e87, 0x42d6, [8]byte{0xbc, 0xfb, 0xb8, 0xc8, 0x09, 0xfa, 0x54, 0x94}}
	iidXMLDocumentIO    = guid{0x6cd0e74e, 0xee65, 0x4489, [8]byte{0x9e, 0xbf, 0xca, 0x43, 0xe8, 0x7b, 0xa6, 0x37}}
	iidMapOfInspectable = guid{0x1b0d3570, 0x0877, 0x5ec2, [8]byte{0x8a, 0x2c, 0x3b, 0x95, 0x39, 0x50, 0x6a, 0xca}}
	iidPropertyValue    = guid{0x4bd682dd, 0x7554, 0x40e9, [8]byte{0x9a, 0x9b, 0x82, 0x65, 0x4e, 0xde, 0x7e, 0x62}}
	iidActivatedHandler = guid{0xab54de2d, 0x97d9, 0x5528, [8]byte{0xb6, 0xad, 0x10, 0x5a, 0xfe, 0x15, 0x65, 0x30}}
)

var (
	combase                    = syscall.NewLazyDLL("combase.dll")
	pRoInitialize              = combase.NewProc("RoInitialize")
	pRoUninitialize            = combase.NewProc("RoUninitialize")
	pRoGetActivationFactory    = combase.NewProc("RoGetActivationFactory")
	pRoActivateInstance        = combase.NewProc("RoActivateInstance")
	pWindowsCreateString       = combase.NewProc("WindowsCreateString")
	pWindowsDeleteString       = combase.NewProc("WindowsDeleteString")
	pWindowsGetStringRawBuffer = combase.NewProc("WindowsGetStringRawBuffer")
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

// comCall calls method number idx of the COM object obj and returns its HRESULT. Pointers
// converted to uintptr in the argument list are kept alive and unmoved for the duration
// of the call.
//
//go:uintptrescapes
func comCall(obj uintptr, idx uintptr, args ...uintptr) int32 {
	if obj == 0 {
		return hresult(ePointer)
	}
	vtbl := *(*uintptr)(at(obj))
	fn := *(*uintptr)(at(vtbl + idx*ptrSize))
	all := make([]uintptr, 0, 6)
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

// queryInterface asks obj for the interface iid; 0 when it has none.
func queryInterface(obj uintptr, iid *guid) uintptr {
	var out uintptr
	if comCall(obj, iUnknownQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))) < 0 {
		return 0
	}
	return out
}

func hresultError(step string, hr int32) error {
	return fmt.Errorf("toast: %s failed: HRESULT 0x%08X", step, uint32(hr))
}

// roInit puts the calling thread into the multithreaded apartment, where events arrive on
// system threads without a message loop. It reports whether the thread owes a matching
// RoUninitialize: a thread that is already initialised (S_FALSE) does, one that sits in
// another mode (RPC_E_CHANGED_MODE) is accepted as it is and does not.
func roInit() (owed bool, err error) {
	r, _, _ := pRoInitialize.Call(roInitMultithreaded)
	switch uint32(r) {
	case sOK, sFalse:
		return true, nil
	case rpcEChangedMode:
		return false, nil
	}
	return false, hresultError("RoInitialize", hresult(r))
}

// newHString makes an HSTRING holding s; the caller deletes it. The empty string is the
// null HSTRING.
func newHString(s string) (uintptr, error) {
	if s == "" {
		return 0, nil
	}
	u := utf16.Encode([]rune(s))
	var h uintptr
	r, _, _ := pWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)), uintptr(unsafe.Pointer(&h)))
	if hresult(r) < 0 {
		return 0, hresultError("WindowsCreateString", hresult(r))
	}
	return h, nil
}

func deleteHString(h uintptr) {
	if h != 0 {
		pWindowsDeleteString.Call(h)
	}
}

// takeHString copies an HSTRING received from WinRT into a Go string and deletes it.
func takeHString(h uintptr) string {
	if h == 0 {
		return ""
	}
	var n uint32
	buf, _, _ := pWindowsGetStringRawBuffer.Call(h, uintptr(unsafe.Pointer(&n)))
	s := ""
	if buf != 0 && n > 0 {
		s = string(utf16.Decode(unsafe.Slice((*uint16)(at(buf)), n)))
	}
	pWindowsDeleteString.Call(h)
	return s
}

// activationFactory returns the interface iid of the activation factory of a class.
func activationFactory(class string, iid *guid) (uintptr, error) {
	name, err := newHString(class)
	if err != nil {
		return 0, err
	}
	defer deleteHString(name)
	var out uintptr
	r, _, _ := pRoGetActivationFactory.Call(name, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if hresult(r) < 0 || out == 0 {
		return 0, hresultError("RoGetActivationFactory("+class+")", hresult(r))
	}
	return out, nil
}

// activateInstance creates an object of a class with its default constructor and returns
// its IInspectable.
func activateInstance(class string) (uintptr, error) {
	name, err := newHString(class)
	if err != nil {
		return 0, err
	}
	defer deleteHString(name)
	var out uintptr
	r, _, _ := pRoActivateInstance.Call(name, uintptr(unsafe.Pointer(&out)))
	if hresult(r) < 0 || out == 0 {
		return 0, hresultError("RoActivateInstance("+class+")", hresult(r))
	}
	return out, nil
}

// removeToken calls a remove_Xxx(EventRegistrationToken) method. The token is a struct of
// one 64-bit integer passed by value: one register on 64-bit Windows, two stack words on
// 32-bit x86.
func removeToken(obj, idx uintptr, token int64) {
	if ptrSize == 4 {
		comCall(obj, idx, uintptr(uint32(token)), uintptr(uint32(token>>32)))
		return
	}
	comCall(obj, idx, uintptr(token))
}

// activatedHandler is the one COM object implemented here: the delegate
// TypedEventHandler<ToastNotification, IInspectable> given to every toast's Activated
// event. Native code only looks at its first word, the vtable. It is a package variable:
// it is never moved or freed, so its reference count is kept only for form.
type handlerObject struct {
	vtbl *[activatedHandlerVtblSize]uintptr // must stay the first field
	refs int32
}

var (
	activatedHandler     handlerObject
	activatedHandlerVtbl [activatedHandlerVtblSize]uintptr
	activatedHandlerOnce sync.Once

	// receiver gets the activations; nil outside Start..Stop.
	receiver atomic.Pointer[func(Activation)]
)

// activatedHandlerAddr prepares the delegate on first use and returns its interface
// pointer.
func activatedHandlerAddr() uintptr {
	activatedHandlerOnce.Do(func() {
		// Callback slots are limited and never freed, so these four are made once.
		activatedHandlerVtbl[0] = syscall.NewCallback(handlerQueryInterface)
		activatedHandlerVtbl[1] = syscall.NewCallback(handlerAddRef)
		activatedHandlerVtbl[2] = syscall.NewCallback(handlerRelease)
		activatedHandlerVtbl[activatedHandlerInvoke] = syscall.NewCallback(handlerInvoke)
		activatedHandler.vtbl = &activatedHandlerVtbl
		activatedHandler.refs = 1
	})
	return uintptr(unsafe.Pointer(&activatedHandler))
}

// handlerQueryInterface answers IUnknown, the delegate's own interface and IAgileObject:
// the last one tells COM that the object may be called from any thread directly, without
// a marshaller.
func handlerQueryInterface(this, riid, out uintptr) uintptr {
	if out == 0 {
		return ePointer
	}
	if riid == 0 {
		*(*uintptr)(at(out)) = 0
		return eNoInterface
	}
	want := *(*guid)(at(riid))
	if want != iidIUnknown && want != iidActivatedHandler && want != iidIAgileObject {
		*(*uintptr)(at(out)) = 0
		return eNoInterface
	}
	atomic.AddInt32(&activatedHandler.refs, 1)
	*(*uintptr)(at(out)) = this
	return sOK
}

func handlerAddRef(this uintptr) uintptr {
	return uintptr(atomic.AddInt32(&activatedHandler.refs, 1))
}

func handlerRelease(this uintptr) uintptr {
	return uintptr(atomic.AddInt32(&activatedHandler.refs, -1))
}

// handlerInvoke runs on a system thread when a toast is clicked or its reply is sent. It
// only copies the event's data into Go strings and hands them on.
func handlerInvoke(this, sender, args uintptr) uintptr {
	notify := receiver.Load()
	if notify == nil || args == 0 {
		return sOK
	}
	(*notify)(readActivation(args))
	return sOK
}

// readActivation copies what the event carries: the arguments string, and the text of
// the reply box when there is one.
func readActivation(args uintptr) Activation {
	var a Activation
	if ea := queryInterface(args, &iidActivatedArgs); ea != 0 {
		var h uintptr
		if comCall(ea, activatedArgsGetArguments, uintptr(unsafe.Pointer(&h))) >= 0 {
			a.Args = takeHString(h)
		}
		comRelease(&ea)
	}
	ea2 := queryInterface(args, &iidActivatedArgs2)
	if ea2 == 0 {
		return a
	}
	defer comRelease(&ea2)
	var input uintptr // IPropertySet
	if comCall(ea2, activatedArgs2GetUserInput, uintptr(unsafe.Pointer(&input))) < 0 || input == 0 {
		return a
	}
	defer comRelease(&input)
	values := queryInterface(input, &iidMapOfInspectable)
	if values == 0 {
		return a
	}
	defer comRelease(&values)
	key, err := newHString(replyInputID)
	if err != nil {
		return a
	}
	defer deleteHString(key)
	var boxed uintptr // IInspectable; the lookup fails when the toast had no text box
	if comCall(values, mapLookup, key, uintptr(unsafe.Pointer(&boxed))) < 0 || boxed == 0 {
		return a
	}
	defer comRelease(&boxed)
	value := queryInterface(boxed, &iidPropertyValue)
	if value == 0 {
		return a
	}
	defer comRelease(&value)
	var h uintptr
	if comCall(value, propertyValueGetString, uintptr(unsafe.Pointer(&h))) >= 0 {
		a.Text = takeHString(h)
	}
	return a
}
