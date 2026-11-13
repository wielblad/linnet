//go:build windows

// Package webview embeds the Microsoft Edge WebView2 browser control in a Win32 window
// using nothing but the standard library: no cgo and no WebView2Loader.dll. It finds the
// installed Evergreen runtime in the registry, loads its EmbeddedBrowserWebView.dll and
// talks to it through COM vtables.
//
// Everything here belongs to one thread: the one that owns the parent window and runs its
// message loop. WebView2 delivers its callbacks through that loop.
package webview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Vtable slots, counted from IUnknown's three. Each was checked against the vtable
// structs in Microsoft's WebView2.h and against the Go bindings named in the comments.
const (
	// ICoreWebView2Environment (WebView2.h; jchv/go-webview2 pkg/edge/corewebview2.go)
	environmentCreateController = 3 // CreateCoreWebView2Controller(HWND, handler)

	// ICoreWebView2Controller (WebView2.h; jchv/go-webview2 pkg/edge/ICoreWebView2Controller.go)
	controllerPutIsVisible = 4  // put_IsVisible(BOOL)
	controllerPutBounds    = 6  // put_Bounds(RECT)
	controllerClose        = 24 // Close()
	controllerGetWebView   = 25 // get_CoreWebView2(ICoreWebView2**)

	// ICoreWebView2Controller2 continues the controller's table
	// (jchv/go-webview2 and wailsapp/go-webview2 pkg/edge/ICoreWebView2Controller2.go)
	controller2PutBackground = 27 // put_DefaultBackgroundColor(COREWEBVIEW2_COLOR)

	// ICoreWebView2 (WebView2.h; jchv/go-webview2 pkg/edge/corewebview2.go)
	webViewNavigate            = 5  // Navigate(LPCWSTR)
	webViewAddNewWindowRequest = 44 // add_NewWindowRequested(handler, EventRegistrationToken*)

	// ICoreWebView2_2 continues ICoreWebView2's table, which ends at slot 60 (generated
	// bindings of wravery/webview2-rs crates/bindings/src/bindings.rs, built from
	// Microsoft's metadata; wailsapp/go-webview2 pkg/edge/ICoreWebView2_2.go)
	webView2GetCookieManager = 66 // get_CookieManager(ICoreWebView2CookieManager**)

	// ICoreWebView2CookieManager (webview2-rs bindings.rs;
	// wailsapp/go-webview2 pkg/webview2/ICoreWebView2CookieManager.go)
	cookieManagerCreateCookie = 3 // CreateCookie(name, value, domain, path, ICoreWebView2Cookie**)
	cookieManagerAddOrUpdate  = 6 // AddOrUpdateCookie(ICoreWebView2Cookie*)

	// ICoreWebView2Cookie (webview2-rs bindings.rs;
	// wailsapp/go-webview2 pkg/webview2/ICoreWebView2Cookie.go)
	cookiePutIsHTTPOnly = 11 // put_IsHttpOnly(BOOL)
	cookiePutIsSecure   = 15 // put_IsSecure(BOOL)

	// ICoreWebView2NewWindowRequestedEventArgs
	// (WebView2.h; wailsapp/go-webview2 pkg/webview2/ICoreWebView2NewWindowRequestedEventArgs.go)
	newWindowGetURI     = 3 // get_Uri(LPWSTR*)
	newWindowPutHandled = 6 // put_Handled(BOOL)
)

// Interface identifiers, from WebView2.h (ICoreWebView2Controller2 and ICoreWebView2_2:
// from the Go bindings and the generated Rust bindings).
var (
	iidWebView2           = guid{0x9e8f0cf8, 0xe670, 0x4b5e, [8]byte{0xb2, 0xbc, 0x73, 0xe0, 0x61, 0xe3, 0x18, 0x4c}}
	iidEnvironment        = guid{0xb96d755e, 0x0319, 0x4e92, [8]byte{0xa2, 0x96, 0x23, 0x43, 0x6f, 0x46, 0xa1, 0xfc}}
	iidController2        = guid{0xc979903e, 0xd4ca, 0x4228, [8]byte{0x92, 0xeb, 0x47, 0xee, 0x3f, 0xa9, 0x6e, 0xab}}
	iidEnvironmentHandler = guid{0x4e8a3389, 0xc9d8, 0x4bd2, [8]byte{0xb6, 0xb5, 0x12, 0x4f, 0xee, 0x6c, 0xc1, 0x4d}}
	iidControllerHandler  = guid{0x6c4819f3, 0xc9b7, 0x4260, [8]byte{0x81, 0x27, 0xc9, 0xf5, 0xbd, 0xe7, 0xf6, 0x8c}}
	iidNewWindowHandler   = guid{0xd4c185fe, 0xc81c, 0x4989, [8]byte{0x97, 0xaf, 0x2d, 0x3f, 0xa7, 0xab, 0x56, 0x51}}
)

// background is the colour shown before a page has painted: A=255, R=14, G=15, B=18.
// COREWEBVIEW2_COLOR is four bytes in the order A, R, G, B and is passed by value, so in
// a little-endian register the alpha byte is the lowest one.
const background = 0xFF | 14<<8 | 15<<16 | 18<<24

type rect struct{ left, top, right, bottom int32 }

// Cookie is one cookie to set in the browser before the first navigation.
type Cookie struct {
	Name, Value, Domain, Path string // Domain like ".facebook.com"; Path "/" when empty
	Secure, HTTPOnly          bool
}

// View is one embedded browser. All its methods must be called on the thread that
// called Open.
type View struct {
	parent  uintptr
	bounds  rect
	url     string   // waiting to be opened once the browser exists
	cookies []Cookie // waiting to be set before that
	done    func(error)
	closed  bool

	opening bool  // inside Open: failures go to its caller, not to done
	openErr error // such a failure

	environment uintptr // ICoreWebView2Environment
	controller  uintptr // ICoreWebView2Controller
	webView     uintptr // ICoreWebView2

	// The view's own references to its callback objects, dropped by Close.
	onEnvironment *handler
	onController  *handler
	onNewWindow   *handler
}

// Open starts creating a browser inside the window parent, placed at bounds (left, top,
// right, bottom in parent client pixels), keeping its profile in userDataDir, and
// navigates to url once it exists, after setting cookies (as session cookies: no expiry
// date, they live as long as the browser does). If the cookies cannot be set, done gets
// the error and nothing is opened. It must be called on the thread that owns parent and
// runs its message loop; creation completes asynchronously on that same thread, and then
// done is called there (nil on success). The returned View can be resized and closed at
// any time, including before done has been called.
//
// An error returned here means nothing was started and done will not be called; done
// never reports an error before Open has returned. done is not called either when the
// view is closed before creation has finished. An empty
// userDataDir lets the runtime pick its default folder next to the executable.
func Open(parent uintptr, bounds [4]int32, userDataDir, url string, cookies []Cookie, done func(error)) (*View, error) {
	if parent == 0 {
		return nil, errors.New("webview: no parent window")
	}
	create, err := loadRuntime()
	if err != nil {
		return nil, err
	}
	var dataDir *uint16
	if userDataDir != "" {
		abs, err := filepath.Abs(userDataDir)
		if err != nil {
			return nil, fmt.Errorf("webview: user data folder %q: %w", userDataDir, err)
		}
		if err := os.MkdirAll(abs, 0o700); err != nil {
			return nil, fmt.Errorf("webview: creating the user data folder: %w", err)
		}
		if dataDir, err = syscall.UTF16PtrFromString(abs); err != nil {
			return nil, fmt.Errorf("webview: user data folder %q: %w", abs, err)
		}
	}
	if err := coInit(); err != nil {
		return nil, err
	}

	v := &View{
		parent:  parent,
		bounds:  rect{bounds[0], bounds[1], bounds[2], bounds[3]},
		url:     url,
		cookies: append([]Cookie(nil), cookies...),
		done:    done,
	}
	v.onEnvironment = newHandler(&iidEnvironmentHandler, v.environmentCompleted)
	v.onController = newHandler(&iidControllerHandler, v.controllerCompleted)
	v.onNewWindow = newHandler(&iidNewWindowHandler, v.newWindowRequested)

	// The entry point is undocumented. As reconstructed by the open-source loaders
	// (jchv/OpenWebView2Loader, wailsapp/go-webview2 webviewloader) it is
	//   HRESULT (bool, int runtimeType, PCWSTR userDataFolder, IUnknown* options,
	//            ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler* handler)
	// and they pass true and 0 for an installed (not app-bundled) runtime. No options
	// object is given, which is what the official loader's option-less call does too.
	// The runtime may report the environment from inside this very call.
	v.opening = true
	r, _, _ := create.Call(1, 0, uintptr(unsafe.Pointer(dataDir)), 0, v.onEnvironment.addr())
	v.opening = false
	if hr := hresult(r); hr < 0 {
		v.Close()
		return nil, hresultError(runtimeEntry, hr)
	}
	if err := v.openErr; err != nil {
		v.Close()
		return nil, err
	}
	return v, nil
}

// SetBounds moves the browser to a new rectangle in parent client pixels.
func (v *View) SetBounds(left, top, right, bottom int32) {
	if v == nil || v.closed {
		return
	}
	v.bounds = rect{left, top, right, bottom}
	if v.controller != 0 {
		putBounds(v.controller, v.bounds)
	}
}

// Navigate opens url; before the browser exists it replaces the address to start with.
func (v *View) Navigate(url string) {
	if v == nil || v.closed {
		return
	}
	if v.webView == 0 {
		v.url = url
		return
	}
	v.navigate(url)
}

// Close closes the controller and releases everything; safe to call twice. A browser
// still being created is closed as soon as it arrives.
func (v *View) Close() {
	if v == nil || v.closed {
		return
	}
	v.closed = true
	v.done = nil
	v.cookies = nil
	if v.controller != 0 {
		comCall(v.controller, controllerClose)
	}
	comRelease(&v.webView)
	comRelease(&v.controller)
	comRelease(&v.environment)
	// The runtime keeps its own references to handlers it still has to call.
	v.onEnvironment.release()
	v.onController.release()
	v.onNewWindow.release()
}

func (v *View) navigate(url string) error {
	p, err := syscall.UTF16PtrFromString(url)
	if err != nil {
		return fmt.Errorf("webview: address %q: %w", url, err)
	}
	if hr := comCall(v.webView, webViewNavigate, uintptr(unsafe.Pointer(p))); hr < 0 {
		return hresultError("ICoreWebView2.Navigate", hr)
	}
	return nil
}

// setCookies puts the cookies into the browser's store. Errors name the cookie and the
// failed step, never its value.
func (v *View) setCookies(cookies []Cookie) error {
	if len(cookies) == 0 {
		return nil
	}
	var webView2, manager uintptr
	if hr := comCall(v.webView, iUnknownQueryInterface,
		uintptr(unsafe.Pointer(&iidWebView2)), uintptr(unsafe.Pointer(&webView2))); hr < 0 || webView2 == 0 {
		return fmt.Errorf("webview: cookies cannot be set, this WebView2 runtime has no ICoreWebView2_2: HRESULT 0x%08X", uint32(hr))
	}
	defer comRelease(&webView2)
	if hr := comCall(webView2, webView2GetCookieManager, uintptr(unsafe.Pointer(&manager))); hr < 0 || manager == 0 {
		return fmt.Errorf("webview: cookies cannot be set, ICoreWebView2_2.get_CookieManager failed: HRESULT 0x%08X", uint32(hr))
	}
	defer comRelease(&manager)
	for _, c := range cookies {
		if err := setCookie(manager, c); err != nil {
			return err
		}
	}
	return nil
}

func setCookie(manager uintptr, c Cookie) error {
	path := c.Path
	if path == "" {
		path = "/"
	}
	var text [4]*uint16
	for i, s := range [4]string{c.Name, c.Value, c.Domain, path} {
		p, err := syscall.UTF16PtrFromString(s)
		if err != nil {
			return fmt.Errorf("webview: cookie %q for %q holds a NUL character", c.Name, c.Domain)
		}
		text[i] = p
	}
	failed := func(step string, hr int32) error {
		return fmt.Errorf("webview: cookie %q for %q: %s failed: HRESULT 0x%08X", c.Name, c.Domain, step, uint32(hr))
	}
	var cookie uintptr
	if hr := comCall(manager, cookieManagerCreateCookie,
		uintptr(unsafe.Pointer(text[0])), uintptr(unsafe.Pointer(text[1])),
		uintptr(unsafe.Pointer(text[2])), uintptr(unsafe.Pointer(text[3])),
		uintptr(unsafe.Pointer(&cookie))); hr < 0 || cookie == 0 {
		return failed("ICoreWebView2CookieManager.CreateCookie", hr)
	}
	defer comRelease(&cookie)
	if hr := comCall(cookie, cookiePutIsSecure, boolArg(c.Secure)); hr < 0 {
		return failed("ICoreWebView2Cookie.put_IsSecure", hr)
	}
	if hr := comCall(cookie, cookiePutIsHTTPOnly, boolArg(c.HTTPOnly)); hr < 0 {
		return failed("ICoreWebView2Cookie.put_IsHttpOnly", hr)
	}
	if hr := comCall(manager, cookieManagerAddOrUpdate, cookie); hr < 0 {
		return failed("ICoreWebView2CookieManager.AddOrUpdateCookie", hr)
	}
	return nil
}

// boolArg is a Win32 BOOL argument.
func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// finish reports the outcome of Open, once.
func (v *View) finish(err error) {
	if v.opening && err != nil {
		v.openErr = err
		return
	}
	if done := v.done; done != nil {
		v.done = nil
		done(err)
	}
}

// environmentCompleted is ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler's
// Invoke(HRESULT result, ICoreWebView2Environment* environment).
func (v *View) environmentCompleted(result, environment uintptr) {
	if v.closed {
		return
	}
	hr := hresult(result)
	if hr >= 0 && environment == 0 {
		hr = hresult(eFail)
	}
	if hr < 0 {
		v.finish(hresultError("creating the WebView2 environment", hr))
		return
	}
	// As the official loader does, ask for the interface by name; this also gives the
	// view a reference of its own that outlives the callback.
	if hr := comCall(environment, iUnknownQueryInterface,
		uintptr(unsafe.Pointer(&iidEnvironment)), uintptr(unsafe.Pointer(&v.environment))); hr < 0 || v.environment == 0 {
		v.environment = 0
		v.finish(hresultError("ICoreWebView2Environment.QueryInterface", hr))
		return
	}
	if hr := comCall(v.environment, environmentCreateController, v.parent, v.onController.addr()); hr < 0 {
		v.finish(hresultError("ICoreWebView2Environment.CreateCoreWebView2Controller", hr))
	}
}

// controllerCompleted is ICoreWebView2CreateCoreWebView2ControllerCompletedHandler's
// Invoke(HRESULT result, ICoreWebView2Controller* controller).
func (v *View) controllerCompleted(result, controller uintptr) {
	hr := hresult(result)
	if hr >= 0 && controller == 0 {
		hr = hresult(eFail)
	}
	if v.closed {
		// Close came first: shut the newcomer down without keeping it.
		if hr >= 0 {
			comCall(controller, controllerClose)
		}
		return
	}
	if hr < 0 {
		v.finish(hresultError("creating the WebView2 controller", hr))
		return
	}
	comCall(controller, iUnknownAddRef)
	v.controller = controller

	if hr := comCall(controller, controllerGetWebView, uintptr(unsafe.Pointer(&v.webView))); hr < 0 || v.webView == 0 {
		v.webView = 0
		v.finish(hresultError("ICoreWebView2Controller.get_CoreWebView2", hr))
		return
	}
	var first error
	note := func(step string, hr int32) {
		if hr < 0 && first == nil {
			first = hresultError(step, hr)
		}
	}
	note("ICoreWebView2Controller.put_Bounds", putBounds(controller, v.bounds))
	note("ICoreWebView2Controller.put_IsVisible", comCall(controller, controllerPutIsVisible, 1))

	// Optional: runtimes older than 89 have no ICoreWebView2Controller2 and stay white.
	var controller2 uintptr
	if hr := comCall(controller, iUnknownQueryInterface,
		uintptr(unsafe.Pointer(&iidController2)), uintptr(unsafe.Pointer(&controller2))); hr >= 0 && controller2 != 0 {
		comCall(controller2, controller2PutBackground, background)
		comRelease(&controller2)
	}

	var token int64 // EventRegistrationToken; never needed, Close ends the subscription
	note("ICoreWebView2.add_NewWindowRequested",
		comCall(v.webView, webViewAddNewWindowRequest, v.onNewWindow.addr(), uintptr(unsafe.Pointer(&token))))

	if first == nil {
		first = v.setCookies(v.cookies)
	}
	v.cookies = nil
	if v.url != "" && first == nil {
		first = v.navigate(v.url)
	}
	v.url = ""
	v.finish(first)
}

// newWindowRequested is ICoreWebView2NewWindowRequestedEventHandler's
// Invoke(ICoreWebView2* sender, ICoreWebView2NewWindowRequestedEventArgs* args). Pages
// asking for another window (target=_blank, window.open) get the address opened in this
// view instead; no separate browser window appears.
func (v *View) newWindowRequested(_, args uintptr) {
	if v.closed || args == 0 {
		return
	}
	comCall(args, newWindowPutHandled, 1)
	var uri uintptr
	if hr := comCall(args, newWindowGetURI, uintptr(unsafe.Pointer(&uri))); hr < 0 {
		return
	}
	if url := takeString(uri); url != "" && url != "about:blank" && v.webView != 0 {
		v.navigate(url)
	}
}
