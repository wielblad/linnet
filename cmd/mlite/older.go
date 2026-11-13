//go:build windows

package main

// Older messages arrive as the reader scrolls back: first the ones already kept on this
// computer, a batch at a time, then page by page from Messenger. What is on screen stays
// where it is while the new ones are added above.

import (
	"errors"
	"time"

	"github.com/rs/zerolog/log"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/store"
)

const (
	windowStart = 120 // messages a conversation opens with
	windowStep  = 160 // how many more each step back adds
	wmAppOlder  = wmApp + 10
	timerOlder  = 25
	timerSpin   = 26
)

var older struct {
	busy   int64          // thread a page is being fetched for, 0 when none
	done   map[int64]bool // threads whose beginning has been reached
	locked map[int64]bool // encrypted threads whose older messages wait for the PIN of the copy
	failed time.Time      // last failed fetch: no retry for a moment
}

// fetchOlder asks Messenger for one page older than what is stored. The window is told
// when it ends: wParam 1 the server has more, 0 that was the beginning, 2 it failed, 3 the
// older messages are in the copy that has not been unlocked on this computer.
func (b *backend) fetchOlder(thread store.Thread) bool {
	client := b.current()
	if client == nil {
		return false
	}
	go func() {
		more, err := client.FetchOlder(b.ctx, b.db, thread)
		res := uintptr(0)
		switch {
		case errors.Is(err, meta.ErrHistoryLocked):
			res = 3
		case err != nil:
			log.Warn().Err(err).Msg("fetch older messages")
			res = 2
		case more:
			res = 1
		}
		postMsg(hMain, wmAppOlder, res, uintptr(thread.ID))
	}()
	return true
}

// olderState says what the top of the conversation should tell: 0 nothing, 1 fetching,
// 2 the beginning of the conversation is shown, 3 what is older waits for the PIN.
func olderState() int {
	if !mv.has || len(mv.msgs) == 0 {
		return 0
	}
	return olderStateFor(mv.localMore)
}

// olderSoon looks, a moment later, whether the reader is close enough to the top for
// more messages; scrolling calls it often, so the look itself is put off a little.
func olderSoon() {
	if mv.has && len(mv.msgs) > 0 && mv.scroll < mv.h*3/2 {
		pSetTimer.Call(mv.hwnd, timerOlder, 40, 0)
	}
}

func olderCheck() {
	if !mv.has || len(mv.msgs) == 0 || mv.scroll >= mv.h*3/2 {
		return
	}
	id := mv.thread.ID
	if mv.localMore { // kept on this computer already: instant
		mv.limit += windowStep
		loadView(true)
		return
	}
	if older.busy != 0 || older.done[id] || av.offline || time.Since(older.failed) < 20*time.Second {
		return
	}
	if older.locked[id] { // asked again only once the copy has been unlocked
		if c := be.current(); c == nil || !c.HasHistoryDevice() {
			return
		}
		delete(older.locked, id)
	}
	if be.fetchOlder(mv.thread) {
		older.busy = id
		pSetTimer.Call(mv.hwnd, timerSpin, 140, 0)
		loadView(true)
	}
}

// olderArrived is called on the window thread when a page ended.
func olderArrived(thread int64, res uintptr) {
	if older.busy == thread {
		older.busy = 0
		pKillTimer.Call(mv.hwnd, timerSpin)
	}
	if older.done == nil {
		older.done = map[int64]bool{}
	}
	switch res {
	case 0:
		older.done[thread] = true
	case 2:
		older.failed = time.Now()
	case 3:
		if older.locked == nil {
			older.locked = map[int64]bool{}
		}
		older.locked[thread] = true
	}
	if mv.has && mv.thread.ID == thread {
		mv.limit += windowStep // the page just stored joins the window
		loadView(true)
	}
	panesOlder(thread)
}
