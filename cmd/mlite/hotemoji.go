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
	mfGrayed  = 0x0001
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

// emojiMenu offers another emoji for the conversation being used, at a point of the screen.
func emojiMenu(x, y int32) {
	t, ok := threadByID(hostThreadID())
	if !ok {
		return
	}
	menu, _, _ := pCreatePopupMenu.Call()
	add := func(flags, id uintptr, label string) {
		pAppendMenuW.Call(menu, flags, id, uintptr(unsafe.Pointer(u16(label))))
	}
	add(mfString|mfGrayed, 0, "Emotka tej rozmowy (zmienia się u wszystkich)")
	add(mfSeparator, 0, "")
	now := threadEmoji(t)
	for i, e := range emojiChoices {
		flags, label := uintptr(mfString), e
		if e == now {
			flags |= mfChecked
		}
		if i == 0 {
			label += "   domyślna"
		}
		add(flags, uintptr(i+1), label)
	}
	host := hostWnd()
	pSetForegroundWindow.Call(host)
	cmd, _, _ := pTrackPopupMenu.Call(menu, tpmRightBtn|tpmReturnCmd|tpmNoNotify, uintptr(x), uintptr(y), 0, host, 0)
	pDestroyMenu.Call(menu)
	if cmd == 0 || int(cmd) > len(emojiChoices) || emojiChoices[cmd-1] == now {
		return
	}
	be.setEmoji(t, emojiChoices[cmd-1])
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
	return m.EmojiSize > 0 && !m.Unsent && m.MediaKind == "" && m.ReplyText == "" && m.Text != ""
}

// msgFont is the font of a message's own words.
func msgFont(m store.Message) uintptr {
	if hotMsg(m) {
		return hotFont([...]int32{dp(30), dp(30), dp(46), dp(64)}[min(m.EmojiSize, 3)])
	}
	return bodyFont(m.Unsent)
}
