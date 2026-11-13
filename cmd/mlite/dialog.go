//go:build windows

package main

// A question the program asks in its own window instead of a system message box: the
// window as it was stays visible, dimmed, behind a card with the question and two buttons.
// The safe answer has the focus; Esc and a click outside the card choose it too.

import "unsafe"

var (
	pPrintWindow        = user32.NewProc("PrintWindow")
	pGdipFillRectangleI = gdiplus.NewProc("GdipFillRectangleI")

	dlg struct {
		hwnd                    uintptr
		snap                    uintptr // the window as it looked when the question came up
		sw, sh                  int32
		w, h                    int32
		title, text, ok, cancel string
		danger                  bool
		then                    func()
		focus, hot              int // 0 the safe answer, 1 the action; hot -1 when none
		card                    rect
		btn                     [2]rect
	}
)

// confirmBox asks a yes-or-no question; then runs only when the action is chosen.
func confirmBox(title, text, ok string, danger bool, then func()) {
	if dlg.hwnd != 0 {
		return
	}
	dlg.title, dlg.text, dlg.ok, dlg.cancel, dlg.danger, dlg.then = title, text, ok, "Anuluj", danger, then
	dlg.focus, dlg.hot = 0, -1
	dlg.snap, dlg.sw, dlg.sh = snapshotMain()
	dlg.hwnd = createWindow(0, "MliteDialog", title, wsChild|wsVisible|wsTabStop, hMain, 0)
	layout()
	pSetFocus.Call(dlg.hwnd)
}

func closeDialog(run bool) {
	if dlg.hwnd == 0 {
		return
	}
	h, then := dlg.hwnd, dlg.then
	dlg.hwnd, dlg.then = 0, nil
	if dlg.snap != 0 {
		pGdipDisposeImage.Call(dlg.snap)
		dlg.snap = 0
	}
	pDestroyWindow.Call(h)
	layout()
	pSetFocus.Call(hView)
	if run && then != nil {
		then()
	}
}

// snapshotMain copies what the main window shows into a picture.
func snapshotMain() (uintptr, int32, int32) {
	var rc rect
	pGetClientRect.Call(hMain, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.right, rc.bottom
	if w < 1 || h < 1 {
		return 0, 0, 0
	}
	screen, _, _ := pGetDC.Call(hMain)
	mem, _, _ := pCreateCompatibleDC.Call(screen)
	bmp, _, _ := pCreateCompatibleBmp.Call(screen, uintptr(w), uintptr(h))
	old, _, _ := pSelectObject.Call(mem, bmp)
	pPrintWindow.Call(hMain, mem, 3) // the client area with everything in it
	pSelectObject.Call(mem, old)
	var img uintptr
	pGdipFromHBITMAP.Call(bmp, 0, uintptr(unsafe.Pointer(&img)))
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mem)
	pReleaseDC.Call(hMain, screen)
	return img, w, h
}

func (g gfx) rectA(x, y, w, h int32, argb uint32) {
	var br uintptr
	pGdipCreateSolidFill.Call(uintptr(argb), uintptr(unsafe.Pointer(&br)))
	pGdipFillRectangleI.Call(uintptr(g), br, iarg(x), iarg(y), iarg(w), iarg(h))
	pGdipDeleteBrush.Call(br)
}

func paintDialog(target uintptr) {
	buf := newBuffer(target, dlg.w, dlg.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, dlg.w, dlg.h}, pal.mica)
	// measure
	pad := dp(24)
	cw := dp(420)
	if cw > dlg.w-dp(32) {
		cw = dlg.w - dp(32)
	}
	c.font(fnt.body)
	_, th := c.measure(dlg.text, cw-2*pad)
	bh := dp(36)
	c.font(fnt.name)
	bwOK := c.width(dlg.ok) + dp(36)
	bwNo := c.width(dlg.cancel) + dp(36)
	ch := pad + sp(24) + dp(8) + th + dp(24) + bh + dp(20)
	x, y := (dlg.w-cw)/2, (dlg.h-ch)/2
	dlg.card = rect{x, y, x + cw, y + ch}
	by := y + ch - dp(20) - bh
	dlg.btn[1] = rect{x + cw - pad - bwOK, by, x + cw - pad, by + bh}
	dlg.btn[0] = rect{dlg.btn[1].left - dp(8) - bwNo, by, dlg.btn[1].left - dp(8), by + bh}

	g := newGfx(buf.hdc)
	if dlg.snap != 0 {
		g.image(dlg.snap, 0, 0, dlg.sw, dlg.sh)
	}
	scrim := uint32(0x8C0D1422) // the dark canvas colour, a little over half opaque
	if !pal.dark {
		scrim = 0x6614213D // ink, lighter: a light window is dimmed, not blacked out
	}
	g.rectA(0, 0, dlg.w, dlg.h, scrim)
	r := dp(12)
	g.round(x, y+dp(3), cw, ch, r, r, r, r, mix(0x000000, pal.mica, 55)) // a soft drop under the card
	g.round(x, y, cw, ch, r, r, r, r, pal.line)
	g.round(x+1, y+1, cw-2, ch-2, r-1, r-1, r-1, r-1, pal.card)
	// buttons
	const dangerFill = 0xB42332
	b0, b1 := dlg.btn[0], dlg.btn[1]
	noFill := pal.card
	if dlg.hot == 0 {
		noFill = mix(pal.fg, pal.card, 7)
	}
	g.round(b0.left, b0.top, b0.right-b0.left, bh, dp(8), dp(8), dp(8), dp(8), mix(pal.fg2, pal.line, 65))
	g.round(b0.left+1, b0.top+1, b0.right-b0.left-2, bh-2, dp(7), dp(7), dp(7), dp(7), noFill)
	okFill, okInk := pal.accent, pal.onAccent
	if dlg.hot == 1 {
		okFill = pal.accentHover
	}
	if dlg.danger {
		okFill, okInk = dangerFill, 0xFFFFFF
		if dlg.hot == 1 {
			okFill = mix(0x000000, dangerFill, 14)
		}
	}
	g.round(b1.left, b1.top, b1.right-b1.left, bh, dp(8), dp(8), dp(8), dp(8), okFill)
	g.free()

	c.font(fnt.head)
	c.text(dlg.title, rect{x + pad, y + pad, x + cw - pad, y + pad + sp(24)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
	c.font(fnt.body)
	c.text(dlg.text, rect{x + pad, y + pad + sp(24) + dp(8), x + cw - pad, y + pad + sp(24) + dp(8) + th}, pal.fg2, dtWordBreak)
	c.font(fnt.name)
	c.text(dlg.cancel, b0, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	c.text(dlg.ok, b1, okInk, dtCenter|dtVCenter|dtSingleLine)
	fr := dlg.btn[dlg.focus]
	fr = rect{fr.left - dp(3), fr.top - dp(3), fr.right + dp(3), fr.bottom + dp(3)}
	drawFocus(buf.hdc, &fr)
	buf.flush(0, 0)
}

func dialogHit(x, y int32) int {
	for i, b := range dlg.btn {
		if x >= b.left && x < b.right && y >= b.top && y < b.bottom {
			return i
		}
	}
	return -1
}

func dialogProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if dlg.hwnd == hwnd {
			paintDialog(hdc)
		}
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSize:
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		dlg.w, dlg.h = rc.right, rc.bottom
		invalidate(hwnd)
		return 0
	case wmMouseMove:
		x, y := lpPoint(lp)
		if hot := dialogHit(x, y); hot != dlg.hot {
			dlg.hot = hot
			invalidate(hwnd)
		}
		return 0
	case wmSetCursor:
		if lp&0xFFFF == 1 && dlg.hot >= 0 {
			cur, _, _ := pLoadCursorW.Call(0, idcHand)
			pSetCursor.Call(cur)
			return 1
		}
	case wmLButtonDown:
		pSetFocus.Call(hwnd)
		return 0
	case wmLButtonUp:
		x, y := lpPoint(lp)
		k := dlg.card
		switch hit := dialogHit(x, y); {
		case hit == 1:
			closeDialog(true)
		case hit == 0, x < k.left || x >= k.right || y < k.top || y >= k.bottom:
			closeDialog(false) // "Anuluj", or a click on the dimmed window
		}
		return 0
	case wmGetDlgCode:
		return 0x0004 // every key, Tab included, stays in the question
	case wmKeyDown:
		switch wp {
		case vkEscape:
			closeDialog(false)
		case 0x09, vkLeft, vkRight: // Tab and the arrows move between the two answers
			dlg.focus = 1 - dlg.focus
			invalidate(hwnd)
		case vkReturn, vkSpace:
			closeDialog(dlg.focus == 1)
		}
		return 0
	case wmKillFocus:
		if dlg.hwnd == hwnd && wp != 0 && wp != hwnd { // the question keeps the keyboard while it is up
			pSetFocus.Call(hwnd)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
