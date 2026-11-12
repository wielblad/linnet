//go:build windows

package main

// The program icon and the notification-area icon are drawn at run time: a speech bubble
// while everything is read, the number of unread messages otherwise.

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	pGdipCreateHICON        = gdiplus.NewProc("GdipCreateHICONFromBitmap")
	pGdipFillPolygonI       = gdiplus.NewProc("GdipFillPolygonI")
	pGdipCreateFontFromLogfont = gdiplus.NewProc("GdipCreateFontFromLogfontW")
	pGdipDeleteFont         = gdiplus.NewProc("GdipDeleteFont")
	pGdipCreateStringFormat = gdiplus.NewProc("GdipCreateStringFormat")
	pGdipSetStringAlign     = gdiplus.NewProc("GdipSetStringFormatAlign")
	pGdipSetStringLineAlign = gdiplus.NewProc("GdipSetStringFormatLineAlign")
	pGdipDeleteStringFormat = gdiplus.NewProc("GdipDeleteStringFormat")
	pGdipSetStringTrimming  = gdiplus.NewProc("GdipSetStringFormatTrimming")
	pGdipDrawString         = gdiplus.NewProc("GdipDrawString")
	pGdipSetTextHint        = gdiplus.NewProc("GdipSetTextRenderingHint")
	pGdipSaveImageToFile    = gdiplus.NewProc("GdipSaveImageToFile")
	pDestroyIcon            = user32.NewProc("DestroyIcon")
	pGetSystemMetricsForDpi = user32.NewProc("GetSystemMetricsForDpi")
)

type logFont struct {
	height, width, escapement, orientation, weight int32
	italic, underline, strikeOut, charSet          byte
	outPrecision, clipPrecision, quality, pitch    byte
	face                                           [32]uint16
}

const (
	wmSetIcon  = 0x0080
	pargbPixel = 0x000E200B // 32 bits per pixel with premultiplied alpha
)

// newBitmap returns an empty transparent GDI+ bitmap and a canvas for shapes on it.
func newBitmap(w, h int32) (uintptr, gfx) {
	var bmp, g uintptr
	pGdipCreateBitmapScan0.Call(uintptr(w), uintptr(h), 0, pargbPixel, 0, uintptr(unsafe.Pointer(&bmp)))
	if bmp == 0 {
		return 0, 0
	}
	pGdipGetImageGraphicsCtx.Call(bmp, uintptr(unsafe.Pointer(&g)))
	pGdipSetSmoothing.Call(g, 4)
	pGdipSetPixelOffset.Call(g, 4)
	pGdipSetTextHint.Call(g, 4) // anti-aliased, no ClearType: the background is transparent
	return bmp, gfx(g)
}

func (g gfx) poly(pts []point, rgb uint32) {
	var brush uintptr
	pGdipCreateSolidFill.Call(uintptr(0xFF000000|rgb), uintptr(unsafe.Pointer(&brush)))
	pGdipFillPolygonI.Call(uintptr(g), brush, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)), 0)
	pGdipDeleteBrush.Call(brush)
}

// label draws short centred text with GDI+, for surfaces with transparency where GDI text cannot go.
func (g gfx) label(s string, x, y, w, h, px int32, bold bool, rgb uint32) {
	g.words(s, x, y, w, h, px, bold, rgb, true, false)
}

// words draws text with GDI+: centred on one line, or left-aligned and wrapped with an ellipsis.
func (g gfx) words(s string, x, y, w, h, px int32, bold bool, rgb uint32, centre, wrap bool) {
	var font, format, brush uintptr
	// The font comes from a LOGFONT: every argument is a pointer, so no floating-point
	// value has to travel through a register.
	lf := logFont{height: -px, weight: 400, quality: 4}
	if bold {
		lf.weight = 700
	}
	copyU16(lf.face[:], "Segoe UI")
	screen, _, _ := pGetDC.Call(0)
	pGdipCreateFontFromLogfont.Call(screen, uintptr(unsafe.Pointer(&lf)), uintptr(unsafe.Pointer(&font)))
	pReleaseDC.Call(0, screen)
	flags := uintptr(0x1000 | 0x4000) // one line, not clipped
	if wrap {
		flags = 0
	}
	pGdipCreateStringFormat.Call(flags, 0, uintptr(unsafe.Pointer(&format)))
	if centre {
		pGdipSetStringAlign.Call(format, 1)
		pGdipSetStringLineAlign.Call(format, 1)
	}
	if wrap {
		pGdipSetStringTrimming.Call(format, 3) // ellipsis at the end
	}
	pGdipCreateSolidFill.Call(uintptr(0xFF000000|rgb), uintptr(unsafe.Pointer(&brush)))
	box := rectF{float32(x), float32(y), float32(w), float32(h)} // GDI+ wants x, y, width, height
	t := syscall.StringToUTF16(s)
	if font != 0 {
		pGdipDrawString.Call(uintptr(g), uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), font, uintptr(unsafe.Pointer(&box)), format, brush)
		pGdipDeleteFont.Call(font)
	}
	pGdipDeleteBrush.Call(brush)
	pGdipDeleteStringFormat.Call(format)
}

func countLabel(n int) string {
	if n > 99 {
		return "99+"
	}
	return fmt.Sprint(n)
}

// iconBitmap draws the icon: the bubble, or the unread count; quiet marks "do not disturb".
func iconBitmap(size int32, count int, quiet bool) uintptr {
	bmp, g := newBitmap(size, size)
	if bmp == 0 {
		return 0
	}
	defer g.free()
	fill, ink := pal.accent, pal.onAccent
	if quiet {
		fill, ink = 0x7A7F87, 0xFFFFFF
	}
	pc := func(v int32) int32 { return size * v / 100 }
	if count > 0 {
		g.ellipse(0, 0, size, size, fill)
		s := countLabel(count)
		px := pc(72)
		switch len(s) {
		case 2:
			px = pc(60)
		case 3:
			px = pc(42)
		}
		g.label(s, 0, -pc(4), size, size, px, true, ink)
		return bmp
	}
	r := pc(24)
	g.round(0, 0, size, size, r, r, r, r, fill)
	br := pc(15)
	g.round(pc(19), pc(21), pc(62), pc(43), br, br, br, br, ink)
	g.poly([]point{{pc(29), pc(58)}, {pc(29), pc(81)}, {pc(50), pc(60)}}, ink)
	return bmp
}

// makeIcon returns an icon handle; the caller destroys it.
func makeIcon(size int32, count int, quiet bool) uintptr {
	bmp := iconBitmap(size, count, quiet)
	if bmp == 0 {
		return 0
	}
	var icon uintptr
	pGdipCreateHICON.Call(bmp, uintptr(unsafe.Pointer(&icon)))
	pGdipDisposeImage.Call(bmp)
	return icon
}

func traySize() int32 {
	if s, _, _ := pGetSystemMetricsForDpi.Call(49, uintptr(dpi)); s != 0 { // small icon width
		return int32(s)
	}
	return dp(16)
}

var (
	winIcons  [2]uintptr
	trayShown struct {
		count  int
		quiet  bool
		accent uint32
		size   int32
		valid  bool
	}
)

// setWindowIcons gives the window and its taskbar button the bubble icon in the current accent.
func setWindowIcons() {
	fresh := [2]uintptr{makeIcon(traySize(), 0, false), makeIcon(dp(32), 0, false)}
	for i, ic := range fresh {
		if ic == 0 {
			continue
		}
		sendMsg(hMain, wmSetIcon, uintptr(i), ic)
		if winIcons[i] != 0 {
			pDestroyIcon.Call(winIcons[i])
		}
		winIcons[i] = ic
	}
}

func unreadPhrase(n int) string {
	switch {
	case n == 1:
		return "1 nieprzeczytana"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return fmt.Sprintf("%d nieprzeczytane", n)
	}
	return fmt.Sprintf("%d nieprzeczytanych", n)
}

// trayRefresh keeps the notification-area icon and its tooltip in step with the unread count.
func trayRefresh() {
	size := traySize()
	if trayShown.valid && trayShown.count == unreadTotal && trayShown.quiet == cfg.DND && trayShown.accent == pal.accent && trayShown.size == size {
		return
	}
	icon := makeIcon(size, unreadTotal, cfg.DND)
	if icon == 0 {
		return
	}
	tip := appName
	if unreadTotal > 0 {
		tip = appName + ": " + unreadPhrase(unreadTotal)
	}
	if cfg.DND {
		tip += " (nie przeszkadzać)"
	}
	old := tray.hIcon
	tray.hIcon = icon
	n := tray
	n.uFlags = nifIcon | nifTip
	copyU16(n.szTip[:], tip)
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&n)))
	if old != 0 && trayShown.valid {
		pDestroyIcon.Call(old)
	}
	trayShown.count, trayShown.quiet, trayShown.accent, trayShown.size, trayShown.valid = unreadTotal, cfg.DND, pal.accent, size, true
}

// dumpIcons saves the icon variants as PNG files, to look at them without the notification area.
func dumpIcons(dir string) {
	png := syscall.GUID{Data1: 0x557CF406, Data2: 0x1A04, Data3: 0x11D3, Data4: [8]byte{0x9A, 0x73, 0x00, 0x00, 0xF8, 0x1E, 0xF3, 0x2E}}
	os.MkdirAll(dir, 0o700)
	for _, size := range []int32{16, 24, 32, 64} {
		for _, v := range []struct {
			count int
			quiet bool
		}{{0, false}, {3, false}, {12, false}, {120, false}, {0, true}, {5, true}} {
			bmp := iconBitmap(size, v.count, v.quiet)
			if bmp == 0 {
				continue
			}
			name := filepath.Join(dir, fmt.Sprintf("icon-%d-%d-%v.png", size, v.count, v.quiet))
			pGdipSaveImageToFile.Call(bmp, uintptr(unsafe.Pointer(u16(name))), uintptr(unsafe.Pointer(&png)), 0)
			pGdipDisposeImage.Call(bmp)
		}
	}
}
