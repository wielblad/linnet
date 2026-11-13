//go:build windows

package main

import (
	"math"
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

// Colours are stored as 0xRRGGBB. The palette follows Windows: light or dark app mode,
// the system accent, three layers (window base, content layer, cards).
type palette struct {
	dark                                  bool
	mica, card, fg, fg2, line, layer, own uint32
	accent, onAccent, signal, hover       uint32
	accentText, ownMeta, ownAccent        uint32 // accent and secondary text that stay readable on their surfaces
	bubble, ownFg, sel                    uint32 // incoming bubble, text on one's own bubble, the selected row
	accentHover, accentDown               uint32 // the accent under the pointer and while pressed
	avatars                               [6]uint32
}

var (
	pal       palette
	dpi       int32 = 96
	textScale int32 = 100
)

func dp(v int) int32 { return int32(v) * dpi / 96 }
func sp(v int) int32 { return int32(v) * dpi * textScale / 9600 }

func cref(rgb uint32) uintptr {
	return uintptr((rgb&0xFF)<<16 | rgb&0xFF00 | (rgb>>16)&0xFF)
}

func mix(a, b uint32, pa uint32) uint32 {
	ch := func(sh uint) uint32 {
		return (((a>>sh)&0xFF)*pa + ((b>>sh)&0xFF)*(100-pa)) / 100
	}
	return ch(16)<<16 | ch(8)<<8 | ch(0)
}

func lum(c uint32) float64 {
	f := func(v uint32) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f((c>>16)&0xFF) + 0.7152*f((c>>8)&0xFF) + 0.0722*f(c&0xFF)
}

func contrast(a, b uint32) float64 {
	la, lb := lum(a), lum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// legible moves c towards ink until it reaches 4.5:1 on every given background.
func legible(c, ink uint32, bgs ...uint32) uint32 {
	out := c
	for k := uint32(0); k <= 100; k += 6 {
		out = mix(ink, c, k)
		ok := true
		for _, bg := range bgs {
			if contrast(out, bg) < 4.5 {
				ok = false
			}
		}
		if ok {
			break
		}
	}
	return out
}

func loadPalette() {
	const personalize = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	dark := regDword(personalize, "AppsUseLightTheme", 1) == 0
	switch cfg.Theme { // chosen in the settings; "" follows Windows
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	if v := os.Getenv("MLITE_THEME"); v != "" { // for screenshots of the other theme only
		dark = v == "dark"
	}
	p := palette{dark: dark}
	// Colours of the Linnet identity (brand/04-system/design-tokens.json). The blue is the
	// brand's own and no longer follows the Windows accent colour.
	if dark {
		p.mica, p.layer, p.card, p.bubble = 0x0D1422, 0x151F31, 0x151F31, 0x1D2A40 // canvas, surface, surface, elevated
		p.fg, p.fg2, p.line = 0xF4F7FC, 0xB4C0D3, 0x344259
		p.accent, p.onAccent = 0x76ADFF, 0x0D1422
		p.accentHover, p.accentDown = mix(0xFFFFFF, p.accent, 14), mix(0x0D1422, p.accent, 14)
		p.own, p.ownFg = p.accent, p.onAccent
		p.sel = mix(p.accent, p.mica, 18)
		p.hover = mix(p.fg, p.mica, 7)
		p.signal = legible(0xB42332, 0xFFFFFF, p.mica, p.layer, p.bubble) // the danger red, lightened until it reads on dark
		for i, c := range [6]uint32{0x2B4A73, 0x4D4A3D, 0x5A3F52, 0x2F5647, 0x4A4470, 0x3B4A63} {
			p.avatars[i] = c
		}
	} else {
		p.mica, p.layer, p.card = 0xF4F7FC, 0xFFFFFF, 0xFFFFFF // canvas, surface, surface
		p.bubble = mix(0xDCE3EF, 0xFFFFFF, 55)                 // the border colour, thinned: a bubble that shows on white
		p.fg, p.fg2, p.line = 0x14213D, 0x58657D, 0xDCE3EF
		p.accent, p.onAccent = 0x0866E8, 0xFFFFFF
		p.accentHover, p.accentDown = 0x0758C7, 0x0649A6
		p.own, p.ownFg = p.accent, p.onAccent
		p.sel = 0xEAF2FF // tint
		p.hover = mix(p.fg, p.mica, 5)
		p.signal = 0xB42332
		for i, c := range [6]uint32{0xC7DAF5, 0xDDD6C3, 0xEBD3DC, 0xC3DDCB, 0xD6D2EE, 0xCBD3E0} {
			p.avatars[i] = c
		}
	}
	if highContrast() {
		p = contrastPalette()
	}
	p.accentText = legible(p.accent, p.fg, p.mica, p.layer, p.card, p.bubble)
	p.ownMeta = legible(mix(p.ownFg, p.own, 82), p.ownFg, p.own)
	p.ownAccent = p.ownFg
	pal = p
	textScale = int32(regDword(`Software\Microsoft\Accessibility`, "TextScaleFactor", 100))
	if v, err := strconv.Atoi(os.Getenv("MLITE_TEXTSCALE")); err == nil { // for screenshots at other text sizes only
		textScale = int32(v)
	}
	if textScale < 100 || textScale > 225 {
		textScale = 100
	}
}

// ---- fonts

var fnt struct {
	body, bodySemi, small, smallSemi, prev, name, nameBold, title, head, icon, iconSm, iconLg uintptr
	bodyIt                                                                                    uintptr // a message taken back
	av, avSm                                                                                  uintptr // initials inside avatars: sized with the circle, not with the text setting
	emoji                                                                                     uintptr // the reaction bar
	lineH                                                                                     int32
}

func makeFont(px int32, weight int, face string) uintptr { return makeFontStyle(px, weight, face, false) }

func makeFontStyle(px int32, weight int, face string, italic bool) uintptr {
	slant := uintptr(0)
	if italic {
		slant = 1
	}
	h, _, _ := pCreateFontW.Call(uintptr(-int(px)), 0, 0, 0, uintptr(weight), slant, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(u16(face))))
	return h
}

// pickFace returns the first installed face among the candidates.
func pickFace(hwnd uintptr, faces ...string) string {
	hdc, _, _ := pGetDC.Call(hwnd)
	defer pReleaseDC.Call(hwnd, hdc)
	for _, face := range faces {
		f := makeFont(16, 400, face)
		old, _, _ := pSelectObject.Call(hdc, f)
		buf := make([]uint16, 64)
		pGetTextFaceW.Call(hdc, 64, uintptr(unsafe.Pointer(&buf[0])))
		pSelectObject.Call(hdc, old)
		pDeleteObject.Call(f)
		if syscall.UTF16ToString(buf) == face {
			return face
		}
	}
	return faces[len(faces)-1]
}

func loadFonts(hwnd uintptr) {
	dwForgetFonts()
	for _, h := range []uintptr{fnt.body, fnt.bodyIt, fnt.bodySemi, fnt.small, fnt.smallSemi, fnt.prev, fnt.name, fnt.nameBold, fnt.title, fnt.head, fnt.icon, fnt.iconSm, fnt.iconLg, fnt.av, fnt.avSm, fnt.emoji} {
		if h != 0 {
			pDeleteObject.Call(h)
		}
	}
	text := pickFace(hwnd, "Segoe UI Variable Text", "Segoe UI")
	display := pickFace(hwnd, "Segoe UI Variable Display", "Segoe UI")
	icons := pickFace(hwnd, "Segoe Fluent Icons", "Segoe MDL2 Assets")
	fnt.body = regFont(sp(14), 400, text)
	fnt.bodyIt = regFontStyle(sp(14), 400, text, true)
	fnt.bodySemi = regFont(sp(12), 600, text)
	fnt.small = regFont(sp(12), 400, text)
	fnt.smallSemi = regFont(sp(12), 600, text)
	fnt.prev = regFont(sp(14), 400, text)
	fnt.name = regFont(sp(14), 600, text)
	fnt.nameBold = regFont(sp(14), 700, text)
	fnt.title = regFont(sp(20), 600, display)
	fnt.head = regFont(sp(16), 600, text)
	fnt.icon = regFont(dp(16), 400, icons)
	fnt.iconSm = regFont(dp(12), 400, icons)
	fnt.iconLg = regFont(dp(20), 400, icons)
	fnt.av = regFont(dp(14), 600, text)
	fnt.emoji = regFont(dp(24), 400, "Segoe UI Emoji")
	fnt.avSm = regFont(dp(12), 600, text)
	fnt.lineH = sp(20)
}

// icon glyphs shared by Segoe Fluent Icons and Segoe MDL2 Assets
const (
	gChat     = ""
	gPeople   = ""
	gSettings = ""
	gSearch   = ""
	gSend     = ""
	gTick     = ""
	gClock    = ""
	gError    = ""
	gMute     = ""
	gPhoto    = ""
)

// ---- GDI helpers

type canvas struct {
	hdc uintptr
}

func (c canvas) fill(r rect, rgb uint32) {
	dwEnd()
	br, _, _ := pCreateSolidBrush.Call(cref(rgb))
	pFillRect.Call(c.hdc, uintptr(unsafe.Pointer(&r)), br)
	pDeleteObject.Call(br)
}

func (c canvas) font(f uintptr) {
	dw.cur = f
	pSelectObject.Call(c.hdc, f)
}

// text draws with DirectWrite when it is available and with GDI otherwise.
func (c canvas) text(s string, r rect, rgb uint32, flags uint32) {
	if s == "" || dwDraw(c.hdc, s, r, rgb, flags) {
		return
	}
	c.gdiText(s, r, rgb, flags)
}

// gdiText draws with GDI, the engine the native edit fields use.
func (c canvas) gdiText(s string, r rect, rgb uint32, flags uint32) {
	if s == "" {
		return
	}
	dwEnd()
	pSetBkMode.Call(c.hdc, transparnt)
	pSetTextColor.Call(c.hdc, cref(rgb))
	w := u16s(s)
	pDrawTextW.Call(c.hdc, uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)-1), uintptr(unsafe.Pointer(&r)), uintptr(flags|dtNoPrefix))
}

// measure returns the size of wrapped text at the given width with the current font.
func (c canvas) measure(s string, width int32) (w, h int32) {
	if s == "" {
		return 0, 0
	}
	if mw, mh, ok := dwMeasure(s, width, true); ok {
		return mw, mh
	}
	r := rect{0, 0, width, 0}
	u := u16s(s)
	pDrawTextW.Call(c.hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&r)),
		dtCalcRect|dtWordBreak|dtNoPrefix|dtEditControl)
	return r.right - r.left, r.bottom - r.top
}

func (c canvas) width(s string) int32 {
	if s == "" {
		return 0
	}
	if mw, _, ok := dwMeasure(s, 0, false); ok {
		return mw
	}
	var sz sizeT
	u := u16s(s)
	pGetTextExtentPoint32.Call(c.hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
	return sz.cx
}

// ---- GDI+ (anti-aliased shapes only; text stays in GDI for ClearType)

var gdipToken uintptr

func gdipInit() {
	in := struct {
		version  uint32
		callback uintptr
		a, b     int32
	}{version: 1}
	pGdiplusStartup.Call(uintptr(unsafe.Pointer(&gdipToken)), uintptr(unsafe.Pointer(&in)), 0)
}

type gfx uintptr

func newGfx(hdc uintptr) gfx {
	dwEnd()
	var g uintptr
	pGdipCreateFromHDC.Call(hdc, uintptr(unsafe.Pointer(&g)))
	pGdipSetSmoothing.Call(g, 4)
	pGdipSetPixelOffset.Call(g, 4)
	return gfx(g)
}

func (g gfx) free() {
	if g != 0 {
		pGdipDeleteGraphics.Call(uintptr(g))
	}
}

func iarg(v int32) uintptr   { return uintptr(uint32(v)) }
func farg(v float32) uintptr { return uintptr(math.Float32bits(v)) }

func (g gfx) ellipse(x, y, w, h int32, rgb uint32) {
	var br uintptr
	pGdipCreateSolidFill.Call(uintptr(0xFF000000|rgb), uintptr(unsafe.Pointer(&br)))
	pGdipFillEllipseI.Call(uintptr(g), br, iarg(x), iarg(y), iarg(w), iarg(h))
	pGdipDeleteBrush.Call(br)
}

// round fills a rectangle with per-corner radii (top-left, top-right, bottom-right, bottom-left).
func (g gfx) round(x, y, w, h, tl, tr, br, bl int32, rgb uint32) {
	if w <= 0 || h <= 0 {
		return
	}
	lim := w
	if h < lim {
		lim = h
	}
	fix := func(r int32) int32 {
		if r*2 > lim {
			r = lim / 2
		}
		if r < 1 {
			r = 1
		}
		return r
	}
	tl, tr, br, bl = fix(tl), fix(tr), fix(br), fix(bl)
	var path, brush uintptr
	pGdipCreatePath.Call(0, uintptr(unsafe.Pointer(&path)))
	pGdipAddPathArcI.Call(path, iarg(x), iarg(y), iarg(tl*2), iarg(tl*2), farg(180), farg(90))
	pGdipAddPathArcI.Call(path, iarg(x+w-tr*2), iarg(y), iarg(tr*2), iarg(tr*2), farg(270), farg(90))
	pGdipAddPathArcI.Call(path, iarg(x+w-br*2), iarg(y+h-br*2), iarg(br*2), iarg(br*2), farg(0), farg(90))
	pGdipAddPathArcI.Call(path, iarg(x), iarg(y+h-bl*2), iarg(bl*2), iarg(bl*2), farg(90), farg(90))
	pGdipClosePathFigure.Call(path)
	pGdipCreateSolidFill.Call(uintptr(0xFF000000|rgb), uintptr(unsafe.Pointer(&brush)))
	pGdipFillPath.Call(uintptr(g), brush, path)
	pGdipDeleteBrush.Call(brush)
	pGdipDeletePath.Call(path)
}

func (g gfx) pill(x, y, w, h int32, rgb uint32) {
	r := h / 2
	g.round(x, y, w, h, r, r, r, r, rgb)
}

// buffer is an off-screen bitmap used for flicker-free painting.
type buffer struct {
	hdc, bmp, old, target uintptr
	w, h                  int32
}

func newBuffer(target uintptr, w, h int32) buffer {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	mem, _, _ := pCreateCompatibleDC.Call(target)
	bmp, _, _ := pCreateCompatibleBmp.Call(target, uintptr(w), uintptr(h))
	old, _, _ := pSelectObject.Call(mem, bmp)
	return buffer{hdc: mem, bmp: bmp, old: old, target: target, w: w, h: h}
}

func (b buffer) flush(x, y int32) {
	dwEnd()
	pBitBlt.Call(b.target, uintptr(x), uintptr(y), uintptr(b.w), uintptr(b.h), b.hdc, 0, 0, srcCopy)
	pSelectObject.Call(b.hdc, b.old)
	pDeleteObject.Call(b.bmp)
	pDeleteDC.Call(b.hdc)
}

// ---- Windows contrast themes

var (
	pGetSysColor         = user32.NewProc("GetSysColor")
	pSystemParametersHC  = user32.NewProc("SystemParametersInfoW")
	highContrastForTests = os.Getenv("MLITE_HC") != ""
)

// highContrast reports whether a Windows contrast theme is on.
func highContrast() bool {
	if highContrastForTests {
		return true
	}
	hc := struct {
		size, flags uint32
		scheme      *uint16
	}{}
	hc.size = uint32(unsafe.Sizeof(hc))
	r, _, _ := pSystemParametersHC.Call(0x0042, uintptr(hc.size), uintptr(unsafe.Pointer(&hc)), 0)
	return r != 0 && hc.flags&1 != 0
}

func sysColor(index uintptr) uint32 {
	c, _, _ := pGetSysColor.Call(index)
	return uint32(c&0xFF)<<16 | uint32(c&0xFF00) | uint32(c>>16)&0xFF
}

// contrastPalette takes every colour from the contrast theme the person chose in Windows.
func contrastPalette() palette {
	bg, text := sysColor(5), sysColor(8)       // window, window text
	sel, selText := sysColor(13), sysColor(14) // highlight, highlighted text
	face := sysColor(15)                       // button face
	p := palette{dark: lum(bg) < 0.4}
	p.mica, p.layer, p.card, p.own = bg, bg, face, face
	p.fg, p.fg2, p.line = text, text, text
	p.accent, p.onAccent, p.signal = sel, selText, text
	p.hover = face
	p.bubble, p.ownFg, p.sel, p.accentHover, p.accentDown = face, text, face, sel, sel
	for i := range p.avatars {
		p.avatars[i] = face
	}
	return p
}
