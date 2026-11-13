//go:build windows

package main

// The actions that appear beside a message under the pointer (react, reply, more) and the
// small bar of reactions that opens from the first of them.

import (
	"unsafe"

	"lazyfmess/internal/store"
)

const (
	gReact = ""
	gReply = ""
	gMore  = ""

	gCopy  = "\uE8C8"

	actReact = 0
	actReply = 1
	actMore  = 2
	actCopy  = 3

	wmCaptureChanged = 0x0215
)

var (
	pCreateRoundRectRgn = gdi32.NewProc("CreateRoundRectRgn")
	pSetWindowRgn       = user32.NewProc("SetWindowRgn")

	pick struct {
		hwnd uintptr
		msg  store.Message
		hot  int
		w, h int32
	}
)

// hoverable reports whether the actions make sense for a message.
func hoverable(it *vitem) bool {
	return it.kind == itMsg && !it.m.Unsent && !it.m.Pending && it.m.Error == ""
}

// itemActions lists the actions of a message in the order they are drawn from the bubble.
func itemActions(it *vitem) []int {
	if it.m.Text != "" && it.body != "" {
		return []int{actReact, actReply, actCopy, actMore}
	}
	return []int{actReact, actReply, actMore}
}

var actGlyph = map[int]string{actReact: gReact, actReply: gReply, actCopy: gCopy, actMore: gMore}

// actRect is where action k of an item is drawn, in view coordinates.
func actRect(it *vitem, k int32) rect {
	s, gap := dp(28), dp(2)
	y := it.y + it.top + (it.bh-s)/2 - mv.scroll
	x := it.bx + it.bw + dp(6) + k*(s+gap)
	if it.me {
		x = it.bx - dp(6) - s - k*(s+gap)
	}
	return rect{x, y, x + s, y + s}
}

// viewHover notes which message and which of its actions the pointer is over.
func viewHover(x, y int32) {
	item, act := -1, -1
	if x >= 0 && pick.hwnd == 0 {
		for i := range mv.items {
			it := &mv.items[i]
			if !hoverable(it) || y+mv.scroll < it.y || y+mv.scroll >= it.y+it.h {
				continue
			}
			item = i
			for k, a := range itemActions(it) {
				if r := actRect(it, int32(k)); x >= r.left && x < r.right && y >= r.top && y < r.bottom {
					act = a
				}
			}
			break
		}
	}
	if item != mv.hover || act != mv.hotAct {
		mv.hover, mv.hotAct = item, act
		invalidate(mv.hwnd)
	}
}

// paintActions draws the three actions of the hovered message: shapes first, glyphs second.
func paintActions(g gfx, c canvas, shapes bool) {
	if mv.hover < 0 || mv.hover >= len(mv.items) {
		return
	}
	it := &mv.items[mv.hover]
	for k, a := range itemActions(it) {
		r := actRect(it, int32(k))
		hot := a == mv.hotAct
		if shapes {
			if hot {
				g.ellipse(r.left, r.top, r.right-r.left, r.bottom-r.top, mix(pal.fg, pal.layer, 12))
			}
			continue
		}
		col := pal.fg2
		if hot {
			col = pal.fg
		}
		c.font(fnt.icon)
		c.text(actGlyph[a], r, col, dtCenter|dtVCenter|dtSingleLine)
	}
}

// actionClick runs the action under the pointer; it reports whether there was one.
func actionClick() bool {
	if mv.hover < 0 || mv.hover >= len(mv.items) || mv.hotAct < 0 {
		return false
	}
	it := &mv.items[mv.hover]
	r := actRect(it, 0)
	for k, a := range itemActions(it) {
		if a == mv.hotAct {
			r = actRect(it, int32(k))
		}
	}
	menuMsg, menuHas = it.m, true
	switch mv.hotAct {
	case actReact:
		showPicker(it.m, (r.left+r.right)/2, r.top)
	case actReply:
		messageAction(idMsgReply)
	case actCopy:
		copyText(it.m.Text)
		viewToast("Skopiowano wiadomość")
	case actMore:
		messageMenu(it.m, r.left, r.bottom)
	}
	return true
}

// showPicker opens the bar of reactions above a point of the conversation view.
func showPicker(m store.Message, x, y int32) {
	closePicker()
	cell := dp(40)
	pick.w, pick.h = int32(len(reactions))*cell+dp(12), cell+dp(8)
	pick.msg, pick.hot = m, -1
	pt := point{x, y}
	pClientToScreen.Call(mv.hwnd, uintptr(unsafe.Pointer(&pt)))
	var work rect
	pSystemParametersInfo.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0)
	px, py := pt.x-pick.w/2, pt.y-pick.h-dp(6)
	if px+pick.w > work.right-dp(8) {
		px = work.right - dp(8) - pick.w
	}
	if px < work.left+dp(8) {
		px = work.left + dp(8)
	}
	if py < work.top+dp(8) {
		py = pt.y + dp(34)
	}
	pick.hwnd = createWindow(wsExTopmost|wsExToolWindow|wsExNoActivate, "MliteReact", "Reakcje", wsPopup, 0, 0)
	if pick.hwnd == 0 {
		return
	}
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(pick.w+1), uintptr(pick.h+1), uintptr(pick.h), uintptr(pick.h))
	pSetWindowRgn.Call(pick.hwnd, rgn, 0)
	pSetWindowPos.Call(pick.hwnd, 0, uintptr(px), uintptr(py), uintptr(pick.w), uintptr(pick.h), 0x0004|0x0010)
	pShowWindow.Call(pick.hwnd, swShowNA)
	pSetCapture.Call(pick.hwnd) // every click reaches the bar, so one outside closes it
	viewHover(-1, -1)
}

func closePicker() {
	if pick.hwnd != 0 {
		h := pick.hwnd
		pick.hwnd = 0
		pReleaseCapture.Call()
		pDestroyWindow.Call(h)
	}
}

func pickAt(x, y int32) int {
	cell := dp(40)
	if y < 0 || y >= pick.h || x < dp(6) || x >= pick.w-dp(6) {
		return -1
	}
	return int((x - dp(6)) / cell)
}

func pickerProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if hwnd != pick.hwnd {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
		return r
	}
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		buf := newBuffer(hdc, pick.w, pick.h)
		c := canvas{buf.hdc}
		c.fill(rect{0, 0, pick.w, pick.h}, pal.line)
		g := newGfx(buf.hdc)
		g.pill(1, 1, pick.w-2, pick.h-2, pal.card)
		cell := dp(40)
		own := ownReaction(pick.msg)
		for i, r := range reactions {
			x := dp(6) + int32(i)*cell
			switch {
			case i == pick.hot:
				g.ellipse(x+dp(2), dp(6), cell-dp(4), cell-dp(4), mix(pal.fg, pal.card, 14))
			case r.emoji == own:
				g.ellipse(x+dp(2), dp(6), cell-dp(4), cell-dp(4), mix(pal.accent, pal.card, 30))
			}
		}
		g.free()
		c.font(fnt.emoji)
		for i, r := range reactions {
			x := dp(6) + int32(i)*cell
			c.text(r.emoji, rect{x, dp(4), x + cell, dp(4) + cell}, pal.fg, dtCenter|dtVCenter|dtSingleLine)
		}
		buf.flush(0, 0)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmMouseMove:
		x, y := lpPoint(lp)
		if hot := pickAt(x, y); hot != pick.hot {
			pick.hot = hot
			invalidate(hwnd)
		}
		return 0
	case wmLButtonDown, wmRButtonUp:
		if x, y := lpPoint(lp); pickAt(x, y) < 0 {
			closePicker()
		}
		return 0
	case wmLButtonUp:
		x, y := lpPoint(lp)
		i := pickAt(x, y)
		m := pick.msg
		closePicker()
		if t, ok := threadByID(selectedID); ok && i >= 0 && i < len(reactions) {
			emoji := reactions[i].emoji
			if emoji == ownReaction(m) { // the same one again takes it back
				emoji = ""
			}
			be.react(t, m, emoji)
		}
		return 0
	case wmCaptureChanged:
		if pick.hwnd != 0 && lp != hwnd {
			h := pick.hwnd
			pick.hwnd = 0
			pDestroyWindow.Call(h)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
