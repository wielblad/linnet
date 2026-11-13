//go:build windows

package main

// The sound of a new message. The program plays it itself rather than leaving it to the
// notification, so that one switch in the settings rules it whichever kind of
// notification is shown.

import (
	"syscall"
	"unsafe"
)

var (
	winmm       = syscall.NewLazyDLL("winmm.dll")
	pPlaySoundW = winmm.NewProc("PlaySoundW")
	soundPlayed string // the Windows sound that played last, for checks
)

// playMessageSound plays the chosen sound for an incoming message.
func playMessageSound() bool {
	if !cfg.Sound {
		return false
	}
	return playTone(cfg.Tone)
}

// playTone plays one of the program's own sounds, or with "" the Windows sound for an
// incoming message, falling back to the general notification sound of the user's scheme.
func playTone(id string) bool {
	const async, noDefault, memory, alias = 0x0001, 0x0002, 0x0004, 0x00010000
	if wav := toneWAV(id); wav != nil {
		r, _, _ := pPlaySoundW.Call(uintptr(unsafe.Pointer(&wav[0])), 0, memory|async|noDefault)
		soundPlayed = id
		return r != 0
	}
	for _, name := range []string{"Notification.IM", "Notification.Default", "SystemNotification"} {
		if r, _, _ := pPlaySoundW.Call(uintptr(unsafe.Pointer(u16(name))), 0, alias|async|noDefault); r != 0 {
			soundPlayed = name
			return true
		}
	}
	return false
}
