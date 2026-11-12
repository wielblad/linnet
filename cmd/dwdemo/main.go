//go:build windows

// dwdemo: a throwaway window that draws the same chat lines with GDI (left) and with
// Direct2D + DirectWrite (right), and reports what the second engine costs in memory.
package main

import (
	"flag"
	"fmt"
	"math"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	d2d1     = syscall.NewLazyDLL("d2d1.dll")
	dwrite   = syscall.NewLazyDLL("dwrite.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")

	pRegisterClassExW = user32.NewProc("RegisterClassExW")
	pCreateWindowExW  = user32.NewProc("CreateWindowExW")
	pDefWindowProcW   = user32.NewProc("DefWindowProcW")
	pShowWindow       = user32.NewProc("ShowWindow")
	pGetMessageW      = user32.NewProc("GetMessageW")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessageW = user32.NewProc("DispatchMessageW")
	pPostQuitMessage  = user32.NewProc("PostQuitMessage")
	pBeginPaint       = user32.NewProc("BeginPaint")
	pEndPaint         = user32.NewProc("EndPaint")
	pGetClientRect    = user32.NewProc("GetClientRect")
	pFillRect         = user32.NewProc("FillRect")
	pDrawTextW        = user32.NewProc("DrawTextW")
	pSetTimer         = user32.NewProc("SetTimer")
	pKillTimer        = user32.NewProc("KillTimer")
	pInvalidateRect   = user32.NewProc("InvalidateRect")
	pLoadCursorW      = user32.NewProc("LoadCursorW")
	pGetDpiForWindow  = user32.NewProc("GetDpiForWindow")
	pSetDpiCtx        = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetDC            = user32.NewProc("GetDC")
	pReleaseDC        = user32.NewProc("ReleaseDC")

	pCreateFontW         = gdi32.NewProc("CreateFontW")
	pDeleteObject        = gdi32.NewProc("DeleteObject")
	pCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBmp = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject        = gdi32.NewProc("SelectObject")
	pDeleteDC            = gdi32.NewProc("DeleteDC")
	pBitBlt              = gdi32.NewProc("BitBlt")
	pCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	pSetBkMode           = gdi32.NewProc("SetBkMode")
	pSetTextColor        = gdi32.NewProc("SetTextColor")
	pRoundRect           = gdi32.NewProc("RoundRect")
	pGetStockObject      = gdi32.NewProc("GetStockObject")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pGetCurrentProc   = kernel32.NewProc("GetCurrentProcess")
	pGetProcMemInfo   = kernel32.NewProc("K32GetProcessMemoryInfo")

	pD2D1CreateFactory   = d2d1.NewProc("D2D1CreateFactory")
	pDWriteCreateFactory = dwrite.NewProc("DWriteCreateFactory")
	pDwmSetAttr          = dwmapi.NewProc("DwmSetWindowAttribute")
)

type rect struct{ left, top, right, bottom int32 }
type rectF struct{ left, top, right, bottom float32 }
type roundedRect struct {
	r      rectF
	rx, ry float32
}
type colorF struct{ r, g, b, a float32 }
type rtProps struct {
	typ, format, alpha uint32
	dpiX, dpiY         float32
	usage, minLevel    uint32
}
type wndClassEx struct {
	cbSize, style         uint32
	lpfnWndProc           uintptr
	cbClsExtra, cbWndExtr int32
	hInstance, hIcon      uintptr
	hCursor, hbrBackgrnd  uintptr
	lpszMenuName          *uint16
	lpszClassName         *uint16
	hIconSm               uintptr
}
type msgT struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
}
type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}
type memCountersEx2 struct {
	cb, pageFaults                        uint32
	peakWS, ws, qpp, qp, qpnp, qnp, pf, ppf uintptr
	privateUsage, privateWS               uintptr
	sharedCommit                          uint64
}

const (
	mica, layer, card, own, fg, fg2, line = 0x1C2026, 0x2A2A33, 0x353A42, 0x574C63, 0xF2F2F2, 0xB4B8BF, 0x373C44
)

type item struct {
	text  string
	mine  bool
	kind  int // 0 body 14, 1 small 12, 2 big 30
	plain bool
	x, y  int32
	w, h  int32
}

var items = []item{
	{text: "Cześć! Jak tam weekend? 😊"},
	{text: "Pogoda dopisała ☀️ widać było Tatry 🏔️", mine: true},
	{text: "Super! 🎉🎉 To co, w piątek powtórka? 👍"},
	{text: "Jasne 😂 biorę kuchenkę ⛺🔥", mine: true},
	{text: "👍 2    ❤️    😂 3", kind: 1},
	{text: "Złożone: rodzina 👨‍👩‍👧, flaga 🇵🇱, dłoń 👋🏽"},
	{text: "😊 👍 ❤️ 🎉 🎂 🥳", kind: 2, mine: true},
	{text: "edytowano 15:17 · Zażółć gęślą jaźń ąćęłńóśźż ĄĆĘŁŃÓŚŹŻ", kind: 1, plain: true},
}

var (
	hwnd       uintptr
	dpi        int32 = 96
	soft       bool
	gdiFont    [4]uintptr // body, small, big, title
	dwFormat   [3]uintptr
	factory    uintptr
	dwFactory  uintptr
	target     uintptr
	brush      uintptr
	d2dReady   bool
	d2dErr     string
	memBefore  int
	memAfter   int
	laidOutFor int32
)

func dp(v int) int32 { return int32(v) * dpi / 96 }
func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}
func cref(rgb uint32) uintptr { return uintptr((rgb&0xFF)<<16 | rgb&0xFF00 | (rgb>>16)&0xFF) }
func cf(rgb uint32) colorF {
	return colorF{float32((rgb>>16)&0xFF) / 255, float32((rgb>>8)&0xFF) / 255, float32(rgb&0xFF) / 255, 1}
}

// com calls method idx of a COM object.
func com(obj uintptr, idx int, args ...uintptr) uintptr {
	vt := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vt + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func privateMB() int {
	h, _, _ := pGetCurrentProc.Call()
	var ex memCountersEx2
	ex.cb = uint32(unsafe.Sizeof(ex))
	if r, _, _ := pGetProcMemInfo.Call(h, uintptr(unsafe.Pointer(&ex)), uintptr(ex.cb)); r != 0 {
		return int((ex.privateWS + 512*1024) / (1024 * 1024))
	}
	return 0
}

func makeFont(px int32, weight int) uintptr {
	h, _, _ := pCreateFontW.Call(uintptr(-int(px)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(u16("Segoe UI Variable Text"))))
	return h
}

func initD2D() {
	iidD2D := syscall.GUID{Data1: 0x06152247, Data2: 0x6f50, Data3: 0x465a, Data4: [8]byte{0x92, 0x45, 0x11, 0x8b, 0xfd, 0x3b, 0x60, 0x07}}
	iidDW := syscall.GUID{Data1: 0xb859ee5a, Data2: 0xd838, Data3: 0x4b5b, Data4: [8]byte{0xa2, 0xe8, 0x1a, 0xdc, 0x7d, 0x93, 0xdb, 0x48}}
	if hr, _, _ := pD2D1CreateFactory.Call(0, uintptr(unsafe.Pointer(&iidD2D)), 0, uintptr(unsafe.Pointer(&factory))); hr != 0 {
		d2dErr = fmt.Sprintf("D2D1CreateFactory %08X", uint32(hr))
		return
	}
	if hr, _, _ := pDWriteCreateFactory.Call(0, uintptr(unsafe.Pointer(&iidDW)), uintptr(unsafe.Pointer(&dwFactory))); hr != 0 {
		d2dErr = fmt.Sprintf("DWriteCreateFactory %08X", uint32(hr))
		return
	}
	props := rtProps{format: 87, alpha: 3, dpiX: 96, dpiY: 96} // B8G8R8A8_UNORM, alpha ignored
	if soft {
		props.typ = 1 // software rasteriser
	}
	if hr := com(factory, 16, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&target))); hr != 0 { // CreateDCRenderTarget
		d2dErr = fmt.Sprintf("CreateDCRenderTarget %08X", uint32(hr))
		return
	}
	white := cf(fg)
	com(target, 8, uintptr(unsafe.Pointer(&white)), 0, uintptr(unsafe.Pointer(&brush))) // CreateSolidColorBrush
	for i, px := range []int{14, 12, 30} {
		size := float32(dp(px))
		hr := com(dwFactory, 15, uintptr(unsafe.Pointer(u16("Segoe UI Variable Text"))), 0, 400, 0, 5,
			uintptr(math.Float32bits(size)), uintptr(unsafe.Pointer(u16("pl-pl"))), uintptr(unsafe.Pointer(&dwFormat[i]))) // CreateTextFormat
		if hr != 0 {
			d2dErr = fmt.Sprintf("CreateTextFormat %08X", uint32(hr))
			return
		}
	}
	d2dReady = true
}

func layout(hdc uintptr, half int32) {
	if laidOutFor == half {
		return
	}
	laidOutFor = half
	y := dp(56)
	maxW := half - dp(32) - dp(60)
	for i := range items {
		it := &items[i]
		old, _, _ := pSelectObject.Call(hdc, gdiFont[it.kind])
		r := rect{0, 0, maxW - dp(24), 0}
		t := syscall.StringToUTF16(it.text)
		pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), 0x0400|0x0010|0x0800)
		pSelectObject.Call(hdc, old)
		it.w = r.right + dp(24) + dp(10) // a little slack: the two engines measure slightly differently
		it.h = r.bottom + dp(16)
		if it.kind == 2 {
			it.h += dp(8)
		}
		it.x = dp(16)
		if it.mine {
			it.x = half - dp(16) - it.w
		}
		it.y = y
		y += it.h + dp(8)
	}
}

func setBrush(rgb uint32) {
	c := cf(rgb)
	com(brush, 8, uintptr(unsafe.Pointer(&c))) // ID2D1SolidColorBrush::SetColor
}

func bubbleColor(it *item) uint32 {
	switch {
	case it.plain:
		return mica
	case it.mine:
		return own
	}
	return card
}

func paint(target0 uintptr) {
	var rc rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.right, rc.bottom
	half := w / 2
	mem, _, _ := pCreateCompatibleDC.Call(target0)
	bmp, _, _ := pCreateCompatibleBmp.Call(target0, uintptr(w), uintptr(h))
	oldBmp, _, _ := pSelectObject.Call(mem, bmp)
	layout(mem, half)

	fill := func(r rect, rgb uint32) {
		br, _, _ := pCreateSolidBrush.Call(cref(rgb))
		pFillRect.Call(mem, uintptr(unsafe.Pointer(&r)), br)
		pDeleteObject.Call(br)
	}
	gdiText := func(s string, r rect, font uintptr, rgb uint32, flags uintptr) {
		old, _, _ := pSelectObject.Call(mem, font)
		pSetBkMode.Call(mem, 1)
		pSetTextColor.Call(mem, cref(rgb))
		t := syscall.StringToUTF16(s)
		pDrawTextW.Call(mem, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), flags|0x0800)
		pSelectObject.Call(mem, old)
	}

	if d2dReady {
		full := rect{0, 0, w, h}
		com(target, 57, mem, uintptr(unsafe.Pointer(&full))) // BindDC
		com(target, 48)                                       // BeginDraw
		bg := cf(mica)
		com(target, 47, uintptr(unsafe.Pointer(&bg))) // Clear
		for side := int32(0); side < 2; side++ {
			ox := side * half
			for i := range items {
				it := &items[i]
				if it.plain {
					continue
				}
				rad := float32(dp(14))
				if it.kind == 1 {
					rad = float32(it.h) / 2
				}
				rr := roundedRect{rectF{float32(ox + it.x), float32(it.y), float32(ox + it.x + it.w), float32(it.y + it.h)}, rad, rad}
				setBrush(bubbleColor(it))
				com(target, 19, uintptr(unsafe.Pointer(&rr)), brush) // FillRoundedRectangle
			}
		}
		for i := range items {
			it := &items[i]
			col := uint32(fg)
			if it.plain {
				col = fg2
			}
			setBrush(col)
			top := it.y + dp(8)
			if it.kind == 2 {
				top += dp(2)
			}
			lr := rectF{float32(half + it.x + dp(12)), float32(top), float32(half + it.x + it.w - dp(6)), float32(it.y + it.h)}
			t := syscall.StringToUTF16(it.text)
			// DrawText with D2D1_DRAW_TEXT_OPTIONS_ENABLE_COLOR_FONT (4)
			com(target, 27, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), dwFormat[it.kind], uintptr(unsafe.Pointer(&lr)), brush, 4, 0)
		}
		if hr := com(target, 49, 0, 0); hr != 0 { // EndDraw
			d2dErr = fmt.Sprintf("EndDraw %08X", uint32(hr))
		}
	} else {
		fill(rect{0, 0, w, h}, mica)
		null, _, _ := pGetStockObject.Call(8) // NULL_PEN
		oldPen, _, _ := pSelectObject.Call(mem, null)
		for i := range items {
			it := &items[i]
			if it.plain {
				continue
			}
			br, _, _ := pCreateSolidBrush.Call(cref(bubbleColor(it)))
			oldBr, _, _ := pSelectObject.Call(mem, br)
			pRoundRect.Call(mem, uintptr(it.x), uintptr(it.y), uintptr(it.x+it.w), uintptr(it.y+it.h), uintptr(dp(28)), uintptr(dp(28)))
			pSelectObject.Call(mem, oldBr)
			pDeleteObject.Call(br)
		}
		pSelectObject.Call(mem, oldPen)
	}

	// left half: the same lines with GDI
	for i := range items {
		it := &items[i]
		col := uint32(fg)
		if it.plain {
			col = fg2
		}
		top := it.y + dp(8)
		if it.kind == 2 {
			top += dp(2)
		}
		gdiText(it.text, rect{it.x + dp(12), top, it.x + it.w - dp(6), it.y + it.h}, gdiFont[it.kind], col, 0x0010)
	}
	fill(rect{half, 0, half + 1, h - dp(40)}, line)
	gdiText("GDI: tak jest teraz", rect{dp(16), dp(14), half, dp(44)}, gdiFont[3], fg, 0x0020)
	right := "DirectWrite: po zmianie"
	if !d2dReady {
		right = "DirectWrite: uruchamiam…"
		if d2dErr != "" {
			right = "DirectWrite: błąd " + d2dErr
		}
	}
	gdiText(right, rect{half + dp(16), dp(14), w, dp(44)}, gdiFont[3], fg, 0x0020)
	fill(rect{0, h - dp(40), w, h}, layer)
	mode := "sprzętowy"
	if soft {
		mode = "programowy"
	}
	foot := fmt.Sprintf("Pamięć procesu (jak w Menedżerze zadań): tylko GDI %d MB", memBefore)
	if memAfter > 0 {
		foot += fmt.Sprintf(", z DirectWrite %d MB (tryb %s)", memAfter, mode)
	}
	if d2dErr != "" {
		foot += "   " + d2dErr
	}
	gdiText(foot, rect{dp(16), h - dp(40), w, h}, gdiFont[1], fg2, 0x0020|0x0004)

	pBitBlt.Call(target0, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, 0x00CC0020)
	pSelectObject.Call(mem, oldBmp)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mem)
}

func wndProc(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0014: // WM_ERASEBKGND
		return 1
	case 0x000F: // WM_PAINT
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
		paint(hdc)
		pEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0005: // WM_SIZE
		laidOutFor = 0
		pInvalidateRect.Call(h, 0, 0)
		return 0
	case 0x0113: // WM_TIMER
		pKillTimer.Call(h, wp)
		switch wp {
		case 1:
			memBefore = privateMB()
			initD2D()
			pInvalidateRect.Call(h, 0, 0)
			pSetTimer.Call(h, 2, 1500, 0)
		case 2:
			memAfter = privateMB()
			pInvalidateRect.Call(h, 0, 0)
		}
		return 0
	case 0x0002: // WM_DESTROY
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(h, uintptr(msg), wp, lp)
	return r
}

func main() {
	runtime.LockOSThread()
	flag.BoolVar(&soft, "soft", false, "programowy tryb rysowania Direct2D")
	flag.String("data-dir", "", "ignorowane")
	flag.Parse()
	pSetDpiCtx.Call(^uintptr(3))
	hinst, _, _ := pGetModuleHandleW.Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, 32512)
	wc := wndClassEx{lpfnWndProc: syscall.NewCallback(wndProc), hInstance: hinst, hCursor: cursor, lpszClassName: u16("DwDemo")}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(u16("DwDemo"))), uintptr(unsafe.Pointer(u16("Porównanie rysowania tekstu"))),
		0x00CF0000, 80, 60, 1000, 640, 0, 0, hinst, 0)
	if d, _, _ := pGetDpiForWindow.Call(hwnd); d != 0 {
		dpi = int32(d)
	}
	dark, caption := int32(1), uint32(cref(mica))
	pDwmSetAttr.Call(hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4)
	pDwmSetAttr.Call(hwnd, 35, uintptr(unsafe.Pointer(&caption)), 4)
	gdiFont = [4]uintptr{makeFont(dp(14), 400), makeFont(dp(12), 400), makeFont(dp(30), 400), makeFont(dp(15), 600)}
	pShowWindow.Call(hwnd, 5)
	pSetTimer.Call(hwnd, 1, 1000, 0)
	var m msgT
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	_ = pGetDC
	_ = pReleaseDC
}
