//go:build windows

// wvdemo: a bare Win32 window filled with a WebView2 browser, to try internal/webview.
//
//	wvdemo [-seconds N] [-data DIR] [-top] [-early MS] [-cookie name=value@domain]... [URL]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"lazyfmess/internal/webview"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassExW    = user32.NewProc("RegisterClassExW")
	pCreateWindowExW     = user32.NewProc("CreateWindowExW")
	pDefWindowProcW      = user32.NewProc("DefWindowProcW")
	pDestroyWindow       = user32.NewProc("DestroyWindow")
	pShowWindow          = user32.NewProc("ShowWindow")
	pUpdateWindow        = user32.NewProc("UpdateWindow")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	pGetMessageW         = user32.NewProc("GetMessageW")
	pTranslateMessage    = user32.NewProc("TranslateMessage")
	pDispatchMessageW    = user32.NewProc("DispatchMessageW")
	pPostMessageW        = user32.NewProc("PostMessageW")
	pPostQuitMessage     = user32.NewProc("PostQuitMessage")
	pGetClientRect       = user32.NewProc("GetClientRect")
	pSetTimer            = user32.NewProc("SetTimer")
	pKillTimer           = user32.NewProc("KillTimer")
	pLoadCursorW         = user32.NewProc("LoadCursorW")
	pSetDpiCtx           = user32.NewProc("SetProcessDpiAwarenessContext")
	pCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	pGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy = 0x0002
	wmSize    = 0x0005
	wmClose   = 0x0010
	wmTimer   = 0x0113

	wsOverlappedWindow = 0x00CF0000
	wsClipChildren     = 0x02000000
	wsExTopmost        = 0x00000008
	cwUseDefault       = 0x80000000
	sizeMinimized      = 1
	swShow             = 5
	idcArrow           = 32512

	timerQuit  = 1
	timerEarly = 2

	ground = 0x120F0E // COLORREF (0x00BBGGRR) of the near-black 0E0F12
)

type rect struct{ left, top, right, bottom int32 }

type wndClassEx struct {
	cbSize, style          uint32
	lpfnWndProc            uintptr
	cbClsExtra, cbWndExtra int32
	hInstance, hIcon       uintptr
	hCursor, hbrBackground uintptr
	lpszMenuName           *uint16
	lpszClassName          *uint16
	hIconSm                uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
}

var (
	view          *webview.View
	ready         bool
	closeReturned bool
)

func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmSize:
		if wParam != sizeMinimized {
			view.SetBounds(0, 0, int32(lParam&0xFFFF), int32(lParam>>16&0xFFFF))
		}
		return 0
	case wmTimer:
		pKillTimer.Call(hwnd, wParam)
		if wParam == timerEarly {
			closeEarly()
		} else {
			pPostMessageW.Call(hwnd, wmClose, 0, 0)
		}
		return 0
	case wmClose:
		view.Close()
		closeReturned = true
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

// closeEarly closes the browser while the window lives on.
func closeEarly() {
	view.Close()
	fmt.Println("closed early; ready seen before:", ready)
}

// cookieFlag collects the repeatable -cookie name=value@domain.
type cookieFlag []webview.Cookie

func (c *cookieFlag) String() string { return fmt.Sprint(len(*c), " cookies") }

func (c *cookieFlag) Set(s string) error {
	at := strings.LastIndex(s, "@")
	eq := strings.Index(s, "=")
	if eq <= 0 || at < eq || at == len(s)-1 {
		return fmt.Errorf("want name=value@domain")
	}
	*c = append(*c, webview.Cookie{Name: s[:eq], Value: s[eq+1 : at], Domain: s[at+1:]})
	return nil
}

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func main() {
	seconds := flag.Int("seconds", 0, "close the window by itself after this many seconds (0: never)")
	data := flag.String("data", filepath.Join(".local", "wvdemo-data"), "user data folder of the browser")
	top := flag.Bool("top", false, "keep the window above all others (for unattended screenshots)")
	early := flag.Int("early", -1, "close the browser this many milliseconds after asking for it, 0 at once (tests closing before it exists)")
	var cookies cookieFlag
	flag.Var(&cookies, "cookie", "cookie to set before the page opens, as name=value@domain (repeatable)")
	flag.Parse()
	url := "https://example.com"
	if flag.NArg() > 0 {
		url = flag.Arg(0)
	}

	// The window, its message loop and the browser all live on this one thread.
	runtime.LockOSThread()
	pSetDpiCtx.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)

	if !webview.Available() {
		fmt.Println("the WebView2 runtime is not installed")
	}

	inst, _, _ := pGetModuleHandleW.Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, idcArrow)
	brush, _, _ := pCreateSolidBrush.Call(ground)
	class := u16("wvdemo")
	wc := wndClassEx{
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     inst,
		hCursor:       cursor,
		hbrBackground: brush,
		lpszClassName: class,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if atom, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		fmt.Println("RegisterClassExW:", err)
		os.Exit(1)
	}
	var exStyle uintptr
	if *top {
		exStyle = wsExTopmost
	}
	hwnd, _, err := pCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(u16("wvdemo"))),
		wsOverlappedWindow|wsClipChildren, cwUseDefault, cwUseDefault, 1000, 700, 0, 0, inst, 0)
	if hwnd == 0 {
		fmt.Println("CreateWindowExW:", err)
		os.Exit(1)
	}

	var client rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	failed := false
	v, openErr := webview.Open(hwnd, [4]int32{0, 0, client.right, client.bottom}, *data, url, cookies, func(err error) {
		if err != nil {
			failed = true
			fmt.Println("error:", err)
			return
		}
		ready = true
		fmt.Println("ready")
	})
	if openErr != nil {
		fmt.Println("error:", openErr)
		pDestroyWindow.Call(hwnd)
		os.Exit(1)
	}
	view = v
	if *early == 0 {
		closeEarly()
	} else if *early > 0 {
		pSetTimer.Call(hwnd, timerEarly, uintptr(*early), 0)
	}

	pShowWindow.Call(hwnd, swShow)
	pUpdateWindow.Call(hwnd)
	pSetForegroundWindow.Call(hwnd)
	if *seconds > 0 {
		pSetTimer.Call(hwnd, timerQuit, uintptr(*seconds)*1000, 0)
	}

	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	fmt.Println("Close returned:", closeReturned)
	if failed {
		os.Exit(1)
	}
}
