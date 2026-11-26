//go:build windows

package main

// The emoji of a conversation. While the field is empty the button beside it is not for
// sending what was written but for sending one emoji: the thumb, or the one the people of
// the conversation chose. Held down, the emoji grows through three sizes, and it is sent
// in the size it had when the button was let go.
//
// While it is held, the growing emoji is shown large in the corner of the conversation,
// where it will land. The other button of the mouse opens the choice of another emoji for
// the conversation; that changes it for everyone in it, as in Messenger.

import (
	"time"
	"unsafe"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/store"
)

const (
	timerHot  = 28 // the emoji grows while its button is held
	hotMedium = 450 * time.Millisecond
	hotLarge  = 1100 * time.Millisecond
)

// hot is the button being held, if any.
var hot struct {
	on    bool
	host  uintptr // the window that shows the button: a pane, or the send button itself
	view  uintptr // the conversation above it, where the growing emoji is shown
	since time.Time
	let   int64 // the size reached when the send button of the window was let go
}

// threadEmoji is the emoji of a conversation's button.
func threadEmoji(t store.Thread) string {
	if t.Emoji != "" {
		return t.Emoji
	}
	return meta.ThumbEmoji
}

// emojiMode reports whether the send button of the view being used sends the emoji: the
// field is empty and is not being used for a file, an answer or a correction.
func emojiMode() bool {
	if compose.mode != cmNone {
		return false
	}
	if p := curBank.pane; p != nil {
		return p.empty
	}
	n, _, _ := pGetWindowTextLengthW.Call(hInput)
	return n == 0
}

// hotStart is the button going down.
func hotStart(host uintptr, t store.Thread) {
	hot.on, hot.host, hot.view, hot.since = true, host, mv.hwnd, time.Now()
	pSetTimer.Call(hMain, timerHot, 30, 0)
	invalidate(host)
}

// hotSize is the size the held emoji has reached: 1, 2 or 3.
func hotSize() int64 {
	if !hot.on {
		return 1
	}
	switch held := time.Since(hot.since); {
	case held >= hotLarge:
		return 3
	case held >= hotMedium:
		return 2
	}
	return 1
}

// hotGrowth is how far the held emoji has grown, 0 to 100, for drawing it between sizes.
func hotGrowth(host uintptr) int32 {
	if !hot.on || hot.host != host {
		return 0
	}
	g := int32(time.Since(hot.since) * 100 / hotLarge)
	if g > 100 {
		g = 100
	}
	return g
}

// hotEnd is the button let go, or the hold given up: it gives the size reached.
func hotEnd() int64 {
	size := hotSize()
	host, view := hot.host, hot.view
	hot.on, hot.host, hot.view = false, 0, 0
	pKillTimer.Call(hMain, timerHot)
	if host != 0 {
		invalidate(host)
	}
	if view != 0 {
		invalidate(view)
	}
	return size
}

// hotTick redraws the growing emoji.
func hotTick() {
	if !hot.on {
		pKillTimer.Call(hMain, timerHot)
		return
	}
	invalidate(hot.host)
	invalidate(hot.view)
}

// hotHost is the window that shows the emoji button of the view being used.
func hotHost() uintptr {
	if p := curBank.pane; p != nil {
		return p.hwnd
	}
	return hSend
}

// paintHotPreview draws the held emoji in the lower corner of the conversation, growing
// from the size of a small one to the size of the largest. A short click shows nothing.
func paintHotPreview(c canvas) {
	if !hot.on || hot.view != mv.hwnd || time.Since(hot.since) < 140*time.Millisecond {
		return
	}
	t, ok := threadByID(hostThreadID())
	if !ok {
		return
	}
	px := dp(30) + (dp(64)-dp(30))*hotGrowth(hot.host)/100
	px -= px % 2 // fewer fonts to make on the way
	c.font(hotFont(px))
	box := px + px/2
	c.text(threadEmoji(t), rect{mv.w - dp(16) - box, mv.h - dp(10) - box, mv.w - dp(16), mv.h - dp(10)}, pal.fg, dtCenter|dtVCenter|dtSingleLine)
}

// emojiChoices are the emoji offered for a conversation; the first is the one everyone
// starts with.
var emojiChoices = []string{
	meta.ThumbEmoji, "\u2764\uFE0F", "\U0001F602", "\U0001F62E", "\U0001F622", "\U0001F621",
	"\U0001F525", "\U0001F4AA", "\U0001F389", "\U0001F44C", "\U0001F64F", "\U0001F60D",
	"\U0001F60E", "\U0001F914", "\U0001F4AF", "\U0001F44B",
}

// epick is the small window that offers another emoji for a conversation: the program's
// own, in its own colours, not a menu of the system.
var epick struct {
	hwnd   uintptr
	hot    int
	w, h   int32
	thread store.Thread
	bank   *viewBank // the view it was opened from
}

const epickCols = 4

func epickCell() int32 { return dp(46) }
func epickPad() int32  { return dp(10) }
func epickHead() int32 { return dp(12) + sp(18) + sp(16) + dp(8) }

// emojiMenu opens the choice of another emoji for the conversation being used, above a
// point of the screen: the upper right corner of its button.
func emojiMenu(x, y int32) {
	t, ok := threadByID(hostThreadID())
	if !ok {
		return
	}
	epickClose()
	closePicker()
	rows := int32((len(emojiChoices) + epickCols - 1) / epickCols)
	epick.w = epickCols*epickCell() + 2*epickPad()
	epick.h = epickHead() + rows*epickCell() + epickPad()
	epick.thread, epick.hot, epick.bank = t, -1, curBank
	work := workAt(rect{x, y, x + 1, y + 1})
	px, py := x-epick.w, y-epick.h-dp(8)
	if px+epick.w > work.right-dp(8) {
		px = work.right - dp(8) - epick.w
	}
	if px < work.left+dp(8) {
		px = work.left + dp(8)
	}
	if py < work.top+dp(8) {
		py = y + dp(44)
	}
	epick.hwnd = createWindow(wsExTopmost|wsExToolWindow|wsExNoActivate, "MliteEmoji", "Emotka rozmowy", wsPopup, 0, 0)
	if epick.hwnd == 0 {
		return
	}
	r := dp(12)
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(epick.w+1), uintptr(epick.h+1), uintptr(2*r), uintptr(2*r))
	pSetWindowRgn.Call(epick.hwnd, rgn, 0)
	pSetWindowPos.Call(epick.hwnd, 0, uintptr(px), uintptr(py), uintptr(epick.w), uintptr(epick.h), 0x0004|0x0010)
	pShowWindow.Call(epick.hwnd, swShowNA)
	pSetCapture.Call(epick.hwnd) // every click reaches it, so one outside closes it
}

func epickClose() {
	if epick.hwnd != 0 {
		h := epick.hwnd
		epick.hwnd = 0
		pReleaseCapture.Call()
		pDestroyWindow.Call(h)
	}
}

// epickAt tells which emoji is at a point of the window, -1 for none.
func epickAt(x, y int32) int {
	x, y = x-epickPad(), y-epickHead()
	if x < 0 || y < 0 || x >= epickCols*epickCell() {
		return -1
	}
	i := int(y/epickCell())*epickCols + int(x/epickCell())
	if i >= len(emojiChoices) {
		return -1
	}
	return i
}

func epickProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if hwnd != epick.hwnd {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
		return r
	}
	if epick.bank != nil && epick.bank != curBank {
		defer enterBank(epick.bank)()
	}
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		epickPaint(hdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmMouseMove:
		x, y := lpPoint(lp)
		if hot := epickAt(x, y); hot != epick.hot {
			epick.hot = hot
			invalidate(hwnd)
		}
		return 0
	case wmLButtonDown, wmRButtonUp:
		if x, y := lpPoint(lp); x < 0 || y < 0 || x >= epick.w || y >= epick.h {
			epickClose()
		}
		return 0
	case wmLButtonUp:
		x, y := lpPoint(lp)
		i, t := epickAt(x, y), epick.thread
		if i < 0 {
			return 0 // on its own edge or its heading: it stays
		}
		epickClose()
		if emojiChoices[i] != threadEmoji(t) {
			be.setEmoji(t, emojiChoices[i])
		}
		return 0
	case wmCaptureChanged:
		if epick.hwnd != 0 && lp != hwnd {
			h := epick.hwnd
			epick.hwnd = 0
			pDestroyWindow.Call(h)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func epickPaint(hdc uintptr) {
	buf := newBuffer(hdc, epick.w, epick.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, epick.w, epick.h}, pal.edge)
	g := newGfx(buf.hdc)
	r := dp(12)
	g.round(1, 1, epick.w-2, epick.h-2, r-1, r-1, r-1, r-1, pal.card)
	cell, now := epickCell(), threadEmoji(epick.thread)
	at := func(i int) (int32, int32) {
		return epickPad() + int32(i%epickCols)*cell, epickHead() + int32(i/epickCols)*cell
	}
	for i, e := range emojiChoices {
		x, y := at(i)
		rr := dp(8)
		switch {
		case e == now: // the one the conversation has: a ring in the accent
			g.round(x+dp(2), y+dp(2), cell-dp(4), cell-dp(4), rr, rr, rr, rr, pal.accent)
			fill := mix(pal.accent, pal.card, 14)
			if i == epick.hot {
				fill = mix(pal.accent, pal.card, 24)
			}
			g.round(x+dp(4), y+dp(4), cell-dp(8), cell-dp(8), rr-dp(2), rr-dp(2), rr-dp(2), rr-dp(2), fill)
		case i == epick.hot:
			g.round(x+dp(2), y+dp(2), cell-dp(4), cell-dp(4), rr, rr, rr, rr, mix(pal.fg, pal.card, 10))
		}
	}
	g.free()
	c.font(fnt.smallSemi)
	c.text("Emotka rozmowy", rect{epickPad() + dp(4), dp(12), epick.w - epickPad(), dp(12) + sp(18)}, pal.fg, dtSingleLine|dtVCenter)
	c.font(fnt.small)
	c.text("Zmienia się u wszystkich", rect{epickPad() + dp(4), dp(12) + sp(18), epick.w - epickPad(), dp(12) + sp(18) + sp(16)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
	c.font(hotFont(dp(26)))
	for i, e := range emojiChoices {
		x, y := at(i)
		c.text(e, rect{x, y, x + cell, y + cell}, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	}
	buf.flush(0, 0)
}

// sendHot sends the emoji of the conversation being used, in the given size.
func sendHot(size int64) {
	t, ok := threadByID(hostThreadID())
	if !ok {
		return
	}
	if be.current() == nil { // said in the conversation itself, wherever it is open
		viewToast("Brak połączenia. Emotka nie została wysłana.")
		return
	}
	if be.sendEmoji(t, size) {
		mv.hasNew = false
	}
}

// drawHotButton draws the emoji in the place of the send sign, as large as it has grown.
func drawHotButton(c canvas, r rect, host uintptr, t store.Thread) {
	px := dp(20) + (dp(30)-dp(20))*hotGrowth(host)/100
	f := hotFont(px)
	c.font(f)
	c.text(threadEmoji(t), r, pal.fg, dtCenter|dtVCenter|dtSingleLine)
}

// hotFonts keeps the emoji fonts by size; they are made when first asked for.
var hotFonts = map[int32]uintptr{}

func hotFont(px int32) uintptr {
	if f := hotFonts[px]; f != 0 {
		return f
	}
	f := regFont(px, 400, "Segoe UI Emoji")
	hotFonts[px] = f
	return f
}

// hotForgetFonts lets the emoji fonts go when the sizes of everything change.
func hotForgetFonts() {
	for px, f := range hotFonts {
		if f != 0 {
			pDeleteObject.Call(f)
		}
		delete(hotFonts, px)
	}
}

// hotMsg reports whether a message is an emoji sent with the button, shown large and
// without a bubble.
func hotMsg(m store.Message) bool {
	return m.EmojiSize > 0 && !m.Unsent && m.MediaKind == "" && m.ReplyText == "" && meta.EmojiOnly(m.Text)
}

// msgFont is the font of a message's own words.
func msgFont(m store.Message) uintptr {
	if hotMsg(m) {
		return hotFont([...]int32{dp(30), dp(30), dp(46), dp(64)}[min(m.EmojiSize, 3)])
	}
	return bodyFont(m.Unsent)
}
