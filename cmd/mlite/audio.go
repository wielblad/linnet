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

// paintWave draws a recording as bars of its loudness in a place of the conversation. What
// has played is in the full colour, the rest paler, and a slider stands where it is now.
// played is in thousandths of the length, or less than 0 for a recording that is not loaded.
func paintWave(c canvas, wave []uint8, r rect, full, pale uint32, played int64) {
	bar, step := dp(3), dp(5)
	w, h := r.right-r.left, r.bottom-r.top
	n := int(w / step)
	if n < 4 || len(wave) == 0 {
		return
	}
	mid, top := r.top+h/2, h-dp(2)
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
		if bh < dp(2) {
			bh = dp(2)
		}
		x := r.left + int32(i)*step
		col := pale
		if x+bar <= at {
			col = full
		}
		c.fill(rect{x, mid - bh/2, x + bar, mid - bh/2 + bh}, col)
	}
	if at >= 0 { // the slider
		c.fill(rect{at - 1, r.top, at + 1, r.bottom}, full)
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
