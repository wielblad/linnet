//go:build windows

package main

// The emoji of a conversation. While the field is empty the button beside it is not for
// sending what was written but for sending one emoji: the thumb, or the one the people of
// the conversation chose. Held down, the emoji grows through three sizes, and it is sent
// in the size it had when the button was let go.
//
// Encrypted chats have no place for a size in their messages: there the emoji goes as
// plain text and the button does not grow.

import (
	"time"

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
	since time.Time
	grows bool
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
	hot.on, hot.host, hot.since, hot.grows = true, host, time.Now(), meta.HotSizes(t)
	if hot.grows {
		pSetTimer.Call(hMain, timerHot, 40, 0)
	}
	invalidate(host)
}

// hotSize is the size the held emoji has reached: 1, 2 or 3.
func hotSize() int64 {
	if !hot.on || !hot.grows {
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
	if !hot.on || hot.host != host || !hot.grows {
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
	host := hot.host
	hot.on, hot.host = false, 0
	pKillTimer.Call(hMain, timerHot)
	if host != 0 {
		invalidate(host)
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
}

// sendHot sends the emoji of the conversation being used, in the given size.
func sendHot(size int64) {
	t, ok := threadByID(hostThreadID())
	if !ok {
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
