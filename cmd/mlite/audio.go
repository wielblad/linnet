//go:build windows

package main

// A voice recording is played in the conversation itself: its line has a play sign, and
// while it plays, a bar that fills and the time. One recording plays at a time. The sound
// goes through the simple player of Windows' Media Foundation, which knows the forms
// Messenger sends recordings in (AAC in an MP4 file, MP3, WAV); a file it cannot open is
// handed to the program Windows has for it, as before.

import (
	"fmt"
	"syscall"
	"unsafe"

	"lazyfmess/internal/store"
)

const (
	timerAudio = 29
	gPause     = ""

	// IMFPMediaPlayer, after the three of IUnknown (mfplay.h)
	mfpPlay        = 3
	mfpPause       = 4
	mfpSetPosition = 7
	mfpGetPosition = 8
	mfpGetDuration = 9
	mfpGetState    = 13
	mfpShutdown    = 38

	mfpStatePlaying = 2
	mfpStatePaused  = 3
	vtI8            = 20
)

var (
	mfplayDLL             = syscall.NewLazyDLL("mfplay.dll")
	pMFPCreateMediaPlayer = mfplayDLL.NewProc("MFPCreateMediaPlayer")

	audio struct {
		player   uintptr
		id       string // the message whose recording is loaded
		playing  bool
		started  bool  // it has been heard playing at least once: a stop after that is its end
		pos, dur int64 // milliseconds
	}
)

type propVariant struct {
	vt  uint16
	_   [6]byte
	val int64
	_   int64
}

// isAudio reports whether a message is a recording to be played in place.
func isAudio(m store.Message) bool {
	return (m.MediaKind == "audio" || m.MediaKind == "voice") && !m.Unsent
}

func audioActive(m store.Message) bool { return audio.player != 0 && audio.id == m.ID }

// audioToggle plays the recording of a message, or pauses and resumes it when it is the
// one already loaded. path is its file on the disk.
func audioToggle(m store.Message, path string) {
	if audioActive(m) {
		if audio.playing {
			com(audio.player, mfpPause)
		} else {
			com(audio.player, mfpPlay)
		}
		audio.playing = !audio.playing
		audioRedraw()
		return
	}
	audioStop()
	pCoInitializeEx.Call(0, 2) // the player wants an apartment; harmless when there is one
	var player uintptr
	hr, _, _ := pMFPCreateMediaPlayer.Call(uintptr(unsafe.Pointer(u16(path))), 1, 0, 0, 0, uintptr(unsafe.Pointer(&player)))
	if int32(hr) < 0 || player == 0 {
		// a form Windows' player does not know: the program set for such files gets it
		safeOpen(path)
		return
	}
	audio.player, audio.id, audio.playing, audio.started = player, m.ID, true, false
	audio.pos, audio.dur = 0, m.MediaDurationMs
	pSetTimer.Call(hMain, timerAudio, 100, 0)
	audioRedraw()
}

// audioStop lets the loaded recording go.
func audioStop() {
	if audio.player == 0 {
		return
	}
	com(audio.player, mfpShutdown)
	com(audio.player, 2) // Release
	audio.player, audio.id, audio.playing = 0, "", false
	pKillTimer.Call(hMain, timerAudio)
	audioRedraw()
}

// audioTick follows the recording as it plays and notices its end.
func audioTick() {
	if audio.player == 0 {
		pKillTimer.Call(hMain, timerAudio)
		return
	}
	var zero [16]byte // MFP_POSITIONTYPE_100NS
	var v propVariant
	if int32(com(audio.player, mfpGetDuration, uintptr(unsafe.Pointer(&zero)), uintptr(unsafe.Pointer(&v)))) >= 0 && v.vt == vtI8 && v.val > 0 {
		audio.dur = v.val / 10000
	}
	v = propVariant{}
	if int32(com(audio.player, mfpGetPosition, uintptr(unsafe.Pointer(&zero)), uintptr(unsafe.Pointer(&v)))) >= 0 && v.vt == vtI8 {
		audio.pos = v.val / 10000
	}
	var state uint32
	com(audio.player, mfpGetState, uintptr(unsafe.Pointer(&state)))
	switch {
	case state == mfpStatePlaying:
		audio.started = true
	case audio.started && audio.playing && state != mfpStatePaused: // it has played to its end
		audioStop()
		return
	}
	audioRedraw()
}

// audioSeek moves the loaded recording to a part of its length, 0 to 1000.
func audioSeek(permille int64) {
	if audio.player == 0 || audio.dur <= 0 {
		return
	}
	var zero [16]byte
	v := propVariant{vt: vtI8, val: audio.dur * min(max(permille, 0), 1000) / 1000 * 10000}
	com(audio.player, mfpSetPosition, uintptr(unsafe.Pointer(&zero)), uintptr(unsafe.Pointer(&v)))
	audio.pos = v.val / 10000
	audioRedraw()
}

// audioRedraw shows the state of the player in every view of a conversation.
func audioRedraw() {
	typingBanks(func() {
		if mv.hwnd != 0 {
			invalidate(mv.hwnd)
		}
	})
}

// chipHeight is the height of the line that stands for an attachment: a recording has a
// taller one, for its button and its wave.
func chipHeight(m store.Message) int32 {
	if isAudio(m) {
		return dp(36)
	}
	return fnt.lineH
}

// paintAudioRow draws a recording in its bubble, from x0 to x1 with its top at ty: a round
// button to play or pause it, its wave with the slider, and its time. col is the colour of
// links on that bubble. The button is drawn, not written with a sign of a font, so that
// play and pause have the same weight and sit in its middle.
func paintAudioRow(c canvas, it *vitem, x0, x1, ty int32, col uint32) {
	h := chipHeight(it.m)
	under, face, sign := pal.bubble, pal.accent, pal.onAccent
	if it.me { // on one's own bubble, which is the accent itself: the button is light
		under, face, sign = pal.own, pal.ownFg, pal.own
	}
	d := dp(32)
	bx, by := x0, ty+(h-d)/2
	g := newGfx(c.hdc)
	g.ellipse(bx, by, d, d, face)
	cx, cy := bx+d/2, by+d/2
	if audioActive(it.m) && audio.playing { // pause: two bars with round ends
		bw, bh, gap := dp(4), dp(12), dp(4)
		g.round(cx-gap/2-bw, cy-bh/2, bw, bh, bw/2, bw/2, bw/2, bw/2, sign)
		g.round(cx+gap/2, cy-bh/2, bw, bh, bw/2, bw/2, bw/2, bw/2, sign)
	} else { // play: a triangle, a little to the right so that it looks centred
		g.poly([]point{{cx - dp(4), cy - dp(7)}, {cx - dp(4), cy + dp(7)}, {cx + dp(7), cy}}, sign)
	}
	g.free()

	it.audL, it.audR = 0, 0
	if p, ok := mediaFile(it.m); ok { // measured once the file is here
		waveKick(it.m.ID, p)
	}
	wave := waveOf(it.m.ID)
	active := audioActive(it.m)
	times, played := clock(it.m.MediaDurationMs), int64(-1)
	if active {
		times = audioTimes()
		if audio.dur > 0 {
			played = min(audio.pos, audio.dur) * 1000 / audio.dur
		}
	}
	c.font(fnt.prev)
	left := x0 + d + dp(10)
	if wave == nil && !active { // nothing to draw of it yet: said in words
		c.text(it.chip, rect{left, ty, x1, ty + h}, col, dtSingleLine|dtVCenter|dtEndEllipsis)
		return
	}
	tw := c.width(times)
	c.text(times, rect{x1 - tw, ty, x1, ty + h}, col, dtSingleLine|dtVCenter)
	it.audL, it.audR = left, x1-tw-dp(10)
	pale := mix(col, under, 38)
	if wave != nil {
		paintWave(c, wave, rect{it.audL, ty + dp(4), it.audR, ty + h - dp(4)}, col, pale, played)
		return
	}
	// a file whose wave could not be measured: a bar that fills
	if w := it.audR - it.audL; w > dp(20) {
		mid := ty + h/2
		g := newGfx(c.hdc)
		g.pill(it.audL, mid-dp(2), w, dp(4), pale)
		if played > 0 {
			g.pill(it.audL, mid-dp(2), max(int32(int64(w)*played/1000), dp(4)), dp(4), col)
		}
		g.free()
	}
}

// paintWave draws a recording as bars of its loudness in a place of the conversation. What
// has played is in the full colour, the rest paler, and a slider stands where it is now.
// played is in thousandths of the length, or less than 0 for a recording that is not loaded.
func paintWave(c canvas, wave []uint8, r rect, full, pale uint32, played int64) {
	bar, step := dp(4), dp(6)
	w, h := r.right-r.left, r.bottom-r.top
	n := int(w / step)
	if n < 4 || len(wave) == 0 {
		return
	}
	mid, top := r.top+h/2, h-dp(2)
	g := newGfx(c.hdc) // bars with round ends
	defer g.free()
	rad := bar / 2
	at := int32(-1)
	if played >= 0 {
		at = r.left + int32(int64(w)*played/1000)
	}
	for i := 0; i < n; i++ {
		from, to := i*len(wave)/n, (i+1)*len(wave)/n
		if to <= from {
			to = from + 1
		}
		v := uint8(0)
		for _, s := range wave[from:min(to, len(wave))] {
			if s > v {
				v = s
			}
		}
		bh := top * int32(v) / 255
		if bh < bar { // the quietest is a dot
			bh = bar
		}
		x := r.left + int32(i)*step
		col := pale
		if x+bar <= at {
			col = full
		}
		g.round(x, mid-bh/2, bar, bh, rad, rad, rad, rad, col)
	}
	if at >= 0 { // the slider
		g.round(at-1, r.top, dp(2), h, 1, 1, 1, 1, full)
	}
}

func clock(ms int64) string {
	s := ms / 1000
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// audioTimes is what stands at the end of a playing recording's line.
func audioTimes() string {
	if audio.dur > 0 {
		return clock(audio.pos) + " / " + clock(audio.dur)
	}
	return clock(audio.pos)
}
