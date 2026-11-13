//go:build windows

package main

// Text through DirectWrite (colour emoji, proper emoji sequences), drawn with a Direct2D
// render target bound to the same GDI device context the rest of the window paints into.
// Everything here is optional: when it cannot start, the canvas falls back to GDI text.

import (
	"math"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	d2d1dll              = syscall.NewLazyDLL("d2d1.dll")
	dwritedll            = syscall.NewLazyDLL("dwrite.dll")
	pD2D1CreateFactory   = d2d1dll.NewProc("D2D1CreateFactory")
	pDWriteCreateFactory = dwritedll.NewProc("DWriteCreateFactory")
	pGetClipBox          = gdi32.NewProc("GetClipBox")
)

type rectF struct{ left, top, right, bottom float32 }
type colorF struct{ r, g, b, a float32 }
type rtProps struct {
	typ, format, alpha uint32
	dpiX, dpiY         float32
	usage, minLevel    uint32
}
type dwTrimming struct{ granularity, delimiter, delimiterCount uint32 }
type dwMetrics struct {
	left, top, width, widthTrailing, height, layoutW, layoutH float32
	maxBidiDepth, lineCount                                   uint32
}
type fontSpec struct {
	px     int32
	weight int
	face   string
	slant  uintptr // DWRITE_FONT_STYLE: 0 upright, 2 italic
}

var dw struct {
	off                     bool // forced off or failed to start
	d2d, dwf, target, brush uintptr
	formats                 map[uintptr]uintptr // GDI font handle -> IDWriteTextFormat
	signs                   map[uintptr]uintptr // GDI font handle -> ellipsis sign
	specs                   map[uintptr]fontSpec
	cur                     uintptr // font chosen last through canvas.font
	dc                      uintptr // device context of the open drawing session
	ox, oy                  int32
}

// com calls method idx of a COM object.
func com(obj uintptr, idx int, args ...uintptr) uintptr {
	vt := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vt + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

// regFont creates a GDI font and remembers how to build its DirectWrite twin.
func regFont(px int32, weight int, face string) uintptr {
	return regFontStyle(px, weight, face, false)
}

// regFontStyle is the same, upright or slanted: a message taken back is written in italics.
func regFontStyle(px int32, weight int, face string, italic bool) uintptr {
	h := makeFontStyle(px, weight, face, italic)
	if dw.specs == nil {
		dw.specs = map[uintptr]fontSpec{}
	}
	slant := uintptr(0)
	if italic {
		slant = 2
	}
	dw.specs[h] = fontSpec{px, weight, face, slant}
	return h
}

// dwForget drops the text formats; they are rebuilt on demand.
func dwForget() {
	dwEnd()
	for _, p := range dw.formats {
		com(p, 2)
	}
	for _, p := range dw.signs {
		if p != 0 {
			com(p, 2)
		}
	}
	dw.formats, dw.signs = nil, nil
}

// dwForgetFonts is called before the GDI fonts are replaced.
func dwForgetFonts() {
	dwForget()
	dw.specs = nil
}

// dwRelease gives back everything DirectWrite and Direct2D hold; the next text call starts them again.
func dwRelease() {
	dwForget()
	for _, p := range []*uintptr{&dw.brush, &dw.target, &dw.dwf, &dw.d2d} {
		if *p != 0 {
			com(*p, 2)
			*p = 0
		}
	}
}

func dwReady() bool {
	if dw.off {
		return false
	}
	if dw.dwf != 0 {
		return true
	}
	if os.Getenv("MLITE_TEXT") == "gdi" || d2d1dll.Load() != nil || dwritedll.Load() != nil {
		dw.off = true
		return false
	}
	iidD2D := syscall.GUID{Data1: 0x06152247, Data2: 0x6f50, Data3: 0x465a, Data4: [8]byte{0x92, 0x45, 0x11, 0x8b, 0xfd, 0x3b, 0x60, 0x07}}
	iidDW := syscall.GUID{Data1: 0xb859ee5a, Data2: 0xd838, Data3: 0x4b5b, Data4: [8]byte{0xa2, 0xe8, 0x1a, 0xdc, 0x7d, 0x93, 0xdb, 0x48}}
	hr1, _, _ := pD2D1CreateFactory.Call(0, uintptr(unsafe.Pointer(&iidD2D)), 0, uintptr(unsafe.Pointer(&dw.d2d)))
	hr2, _, _ := pDWriteCreateFactory.Call(0, uintptr(unsafe.Pointer(&iidDW)), uintptr(unsafe.Pointer(&dw.dwf)))
	if hr1 != 0 || hr2 != 0 || dw.d2d == 0 || dw.dwf == 0 {
		dwRelease()
		dw.off = true
		return false
	}
	return true
}

func dwFormat(f uintptr) uintptr {
	if p := dw.formats[f]; p != 0 {
		return p
	}
	spec, ok := dw.specs[f]
	if !ok {
		return 0
	}
	var p, sign uintptr
	hr := com(dw.dwf, 15, uintptr(unsafe.Pointer(u16(spec.face))), 0, uintptr(spec.weight), spec.slant, 5,
		uintptr(math.Float32bits(float32(spec.px))), uintptr(unsafe.Pointer(u16("pl-pl"))), uintptr(unsafe.Pointer(&p))) // CreateTextFormat
	if hr != 0 || p == 0 {
		return 0
	}
	com(dw.dwf, 20, p, uintptr(unsafe.Pointer(&sign))) // CreateEllipsisTrimmingSign
	if dw.formats == nil {
		dw.formats, dw.signs = map[uintptr]uintptr{}, map[uintptr]uintptr{}
	}
	dw.formats[f], dw.signs[f] = p, sign
	return p
}

// dwBegin opens a drawing session on the device context; what GDI drew there before stays underneath.
func dwBegin(hdc uintptr) bool {
	if dw.dc == hdc {
		return true
	}
	dwEnd()
	if dw.target == 0 {
		// B8G8R8A8, alpha ignored, one unit = one pixel. The software rasteriser is used on purpose:
		// measured here it costs about 5 MB, the graphics-card path about 21 MB.
		props := rtProps{typ: 1, format: 87, alpha: 3, dpiX: 96, dpiY: 96}
		if os.Getenv("MLITE_D2D") == "gpu" { // for measurements only
			props.typ = 0
		}
		if hr := com(dw.d2d, 16, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&dw.target))); hr != 0 || dw.target == 0 { // CreateDCRenderTarget
			dw.target = 0
			dwRelease()
			dw.off = true
			return false
		}
		white := colorF{1, 1, 1, 1}
		com(dw.target, 8, uintptr(unsafe.Pointer(&white)), 0, uintptr(unsafe.Pointer(&dw.brush))) // CreateSolidColorBrush
		if dw.brush == 0 {
			dwRelease()
			dw.off = true
			return false
		}
	}
	var box rect
	pGetClipBox.Call(hdc, uintptr(unsafe.Pointer(&box)))
	if box.right <= box.left || box.bottom <= box.top {
		return false
	}
	if hr := com(dw.target, 57, hdc, uintptr(unsafe.Pointer(&box))); hr != 0 { // BindDC
		return false
	}
	com(dw.target, 48) // BeginDraw
	dw.dc, dw.ox, dw.oy = hdc, box.left, box.top
	return true
}

// dwEnd closes the open session, so that GDI can draw on the device context again.
func dwEnd() {
	if dw.dc == 0 {
		return
	}
	hr := com(dw.target, 49, 0, 0) // EndDraw
	dw.dc = 0
	if hr != 0 { // the device was lost: build the target again next time
		com(dw.brush, 2)
		com(dw.target, 2)
		dw.brush, dw.target = 0, 0
	}
}

func dwApply(p, f uintptr, flags uint32) {
	align, para, wrap := uintptr(0), uintptr(0), uintptr(1)
	switch {
	case flags&dtCenter != 0:
		align = 2
	case flags&dtRight != 0:
		align = 1
	}
	switch {
	case flags&dtVCenter != 0:
		para = 2
	case flags&0x0008 != 0: // DT_BOTTOM
		para = 1
	}
	if flags&dtWordBreak != 0 {
		wrap = 0
	}
	com(p, 3, align) // SetTextAlignment
	com(p, 4, para)  // SetParagraphAlignment
	com(p, 5, wrap)  // SetWordWrapping
	var trim dwTrimming
	sign := uintptr(0)
	if flags&dtEndEllipsis != 0 {
		trim.granularity, sign = 1, dw.signs[f]
	}
	com(p, 9, uintptr(unsafe.Pointer(&trim)), sign) // SetTrimming
}

// dwDraw draws text with the font chosen last; false means the caller should use GDI.
func dwDraw(hdc uintptr, s string, r rect, rgb uint32, flags uint32) bool {
	if !dwReady() {
		return false
	}
	p := dwFormat(dw.cur)
	if p == 0 || !dwBegin(hdc) {
		return false
	}
	dwApply(p, dw.cur, flags)
	col := colorF{float32((rgb>>16)&0xFF) / 255, float32((rgb>>8)&0xFF) / 255, float32(rgb&0xFF) / 255, 1}
	com(dw.brush, 8, uintptr(unsafe.Pointer(&col))) // SetColor
	if flags&dtSingleLine != 0 && strings.ContainsAny(s, "\r\n") {
		s = strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
	}
	t := u16s(s)
	lr := rectF{float32(r.left - dw.ox), float32(r.top - dw.oy), float32(r.right - dw.ox), float32(r.bottom - dw.oy)}
	// DrawText with colour fonts enabled
	com(dw.target, 27, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), p, uintptr(unsafe.Pointer(&lr)), dw.brush, 4, 0)
	return true
}

// dwMeasure returns the size of the text with the font chosen last, wrapped at width when wrap is set.
func dwMeasure(s string, width int32, wrap bool) (w, h int32, ok bool) {
	if !dwReady() {
		return 0, 0, false
	}
	p := dwFormat(dw.cur)
	if p == 0 {
		return 0, 0, false
	}
	flags := uint32(0)
	maxW := float32(100000)
	if wrap {
		flags = dtWordBreak
		maxW = float32(width)
	}
	dwApply(p, dw.cur, flags)
	t := u16s(s)
	var layout uintptr
	hr := com(dw.dwf, 18, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), p,
		uintptr(math.Float32bits(maxW)), uintptr(math.Float32bits(100000)), uintptr(unsafe.Pointer(&layout))) // CreateTextLayout
	if hr != 0 || layout == 0 {
		return 0, 0, false
	}
	var m dwMetrics
	com(layout, 60, uintptr(unsafe.Pointer(&m))) // GetMetrics
	com(layout, 2)
	width32 := m.widthTrailing
	if wrap {
		width32 = m.width
	}
	return int32(math.Ceil(float64(width32))), int32(math.Ceil(float64(m.height))), true
}

// drawFocus draws the keyboard focus rectangle with GDI.
// drawFocus marks keyboard focus the identity's way: a two-pixel rounded frame in the
// focus colour, which is 3:1 or more against every surface.
func drawFocus(hdc uintptr, r *rect) {
	dwEnd()
	pen, _, _ := pCreatePen.Call(0, uintptr(dp(2)), cref(pal.accent))
	hollow, _, _ := pGetStockObject.Call(5)
	oldPen, _, _ := pSelectObject.Call(hdc, pen)
	oldBrush, _, _ := pSelectObject.Call(hdc, hollow)
	pRoundRect.Call(hdc, uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom), uintptr(dp(12)), uintptr(dp(12)))
	pSelectObject.Call(hdc, oldBrush)
	pSelectObject.Call(hdc, oldPen)
	pDeleteObject.Call(pen)
}
