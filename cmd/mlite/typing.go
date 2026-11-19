//go:build windows

package main

// Someone is writing. Messenger says so as it happens; the program shows it wherever the
// conversation can be seen: three dots under the last message of every open view of it,
// and the same three dots on its head, whether the conversation is open or not.
//
// Encrypted chats tell about writing only those who have said they are there. So while a
// window of the program or a head is on the screen, the program says it is there, and
// takes it back when nothing of it is left to look at.

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	itTyping  = 100             // the dots under the last message: not a message, only a state
	typingFor = 8 * time.Second // the news is repeated every few seconds while it lasts
)

var (
	typingWho    sync.Map // conversation -> who is writing there; written where the news arrives
	typingDebug  = os.Getenv("MLITE_TYPING_DEBUG") != ""
	typingShow   = os.Getenv("MLITE_TYPING") // for looking at the dots only: a conversation, or "all"
	presenceSent bool                        // what Messenger was last told: here, or away
)

// typingNow reports whether someone is writing in the conversation at this moment.
func typingNow(id int64) bool {
	if id == 0 || cfg.NoTyping {
		return false
	}
	if typingShow == "all" || (typingShow != "" && typingShow == strconv.FormatInt(id, 10)) {
		return true
	}
	until, ok := typingUntil[id]
	if !ok {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	delete(typingUntil, id)
	return false
}

// typingAny reports whether the dots have to be kept turning anywhere.
func typingAny() bool {
	if typingShow != "" && !cfg.NoTyping {
		return true
	}
	for id := range typingUntil {
		if typingNow(id) {
			return true
		}
	}
	return false
}

// typingBanks runs fn in every view of a conversation there is: the window's own and each
// conversation open beside the heads.
func typingBanks(fn func()) {
	leave := enterBank(&mainBank)
	fn()
	leave()
	for _, p := range panes {
		if p.bank == nil {
			continue
		}
		leave := enterBank(p.bank)
		fn()
		leave()
	}
}

func typingHeight() int32 { return dp(8) + dp(34) }

// typingLay puts the dots under the last message of the view being laid out.
func typingLay() {
	if !mv.has || !typingNow(mv.thread.ID) {
		return
	}
	mv.items = append(mv.items, vitem{kind: itTyping, y: mv.total - dp(12), h: typingHeight()})
	mv.total += typingHeight()
}

// typingView brings one view in line with what is known: the dots come, go, or turn. A
// reader at the newest message stays there; one reading further up is not moved.
func typingView() {
	if mv.hwnd == 0 {
		return
	}
	n := len(mv.items)
	has := n > 0 && mv.items[n-1].kind == itTyping
	want := mv.has && typingNow(mv.thread.ID)
	switch {
	case has == want:
		if !want {
			return
		}
	case want:
		atBottom := mv.scroll >= mv.total-mv.h-dp(48)
		typingLay()
		if atBottom {
			mv.scroll = mv.total - mv.h
		}
	default:
		mv.items = mv.items[:n-1]
		mv.total -= typingHeight()
	}
	clampScroll()
	invalidate(mv.hwnd)
}

// typingSync shows the present state everywhere: in the views and on the heads.
func typingSync() {
	typingBanks(typingView)
	headsSync(false)
	headDotsTimer()
}

// typingStopped is a message arriving: whoever was writing it has finished.
func typingStopped(id int64) {
	if _, ok := typingUntil[id]; ok {
		delete(typingUntil, id)
		typingSync()
	}
}

// paintTyping draws the dots as a bubble of the other person, at y on the screen.
func paintTyping(g gfx, y int32, shadow uint32) {
	x, by := dp(16), y+dp(8)
	w, h := dp(54), dp(34)
	if mv.group { // whose bubble it is, where their picture stands beside their messages
		who, _ := typingWho.Load(mv.thread.ID)
		id, _ := who.(int64)
		if id == 0 || !g.avatar(id, dp(16), by+h-dp(28), dp(28)) {
			g.ellipse(dp(16), by+h-dp(28), dp(28), dp(28), avatarColor(id))
		}
		x += dp(36)
	}
	r := h / 2
	g.round(x, by+1, w, h, r, r, r, r, shadow)
	g.round(x, by, w, h, r, r, r, r, pal.bubble)
	d := dp(7)
	for k := int32(0); k < 3; k++ {
		col, lift := mix(pal.fg2, pal.bubble, 45), int32(0)
		if int32(headPhase%3) == k { // one after another darker and a little higher
			col, lift = pal.fg2, dp(2)
		}
		g.ellipse(x+w/2-d/2+(k-1)*dp(11), by+(h-d)/2-lift, d, d, col)
	}
}

// presenceWanted reports whether there is anything of the program on the screen that
// could show someone writing.
func presenceWanted() bool {
	if cfg.NoTyping || hMain == 0 {
		return false
	}
	if len(panes) > 0 {
		return true
	}
	for _, h := range heads {
		if !h.hidden {
			return true
		}
	}
	vis, _, _ := pIsWindowVisible.Call(hMain)
	min, _, _ := pIsIconic.Call(hMain)
	return vis != 0 && min == 0
}

// presenceSync tells Messenger when that changes.
func presenceSync() {
	if want := presenceWanted(); want != presenceSent {
		presenceSent = want
		be.setPresence(want)
	}
}

// setPresence says to the encrypted chats that the person is here, or no longer is.
func (b *backend) setPresence(here bool) {
	b.here.Store(here)
	client := b.current()
	if client == nil {
		return
	}
	go func() {
		err := client.SetPresence(b.ctx, here)
		if err != nil {
			log.Warn().Err(err).Bool("here", here).Msg("tell encrypted chats whether the person is here")
		} else if typingDebug {
			log.Warn().Bool("here", here).Msg("typing: presence told")
		}
	}()
}
