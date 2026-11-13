//go:build windows

package main

// The Windows notification about a new message carries the picture of the conversation
// (the person, the members of a group, or initials) instead of the generic "information"
// sign, and stays quiet while Windows itself is in "do not disturb".

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"lazyfmess/internal/store"
)

const (
	niifUser      = 0x04
	niifLargeIcon = 0x20
	niifQuietTime = 0x80
	niifNoSound   = 0x10 // the program plays the message sound itself
)

var notifyPic uintptr // the icon of the last notification, kept until the next one replaces it

// notifyBitmap draws the conversation's avatar on a square of d pixels.
func notifyBitmap(t store.Thread, d int32) uintptr {
	bmp, g := newBitmap(d, d)
	if bmp == 0 {
		return 0
	}
	if !threadAvatar(g, t, 0, 0, d, pal.card) {
		g.label(initials(threadTitle(t)), 0, 0, d, d, d*38/100, true, pal.fg)
	}
	g.free()
	return bmp
}

// notifyAbout shows the notification for a conversation.
func notifyAbout(t store.Thread, title, text string) {
	if toastAbout(t, title, text) {
		return
	}
	var icon uintptr
	if bmp := notifyBitmap(t, 96); bmp != 0 {
		pGdipCreateHICON.Call(bmp, uintptr(unsafe.Pointer(&icon)))
		pGdipDisposeImage.Call(bmp)
	}
	n := tray
	n.uFlags = nifInfo
	n.dwInfoFlags = niifInfo | niifQuietTime | niifNoSound
	if icon != 0 {
		n.dwInfoFlags = niifUser | niifLargeIcon | niifQuietTime | niifNoSound
		n.hBalloonIcon = icon
	}
	copyU16(n.szInfoTitle[:], title)
	copyU16(n.szInfo[:], text)
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&n)))
	if notifyPic != 0 {
		pDestroyIcon.Call(notifyPic)
	}
	notifyPic = icon
}

// dumpNotifyIcons saves the notification pictures of the first conversations as PNG files,
// to look at them without waiting for a notification.
func dumpNotifyIcons(dir string) {
	png := syscall.GUID{Data1: 0x557CF406, Data2: 0x1A04, Data3: 0x11D3, Data4: [8]byte{0x9A, 0x73, 0x00, 0x00, 0xF8, 0x1E, 0xF3, 0x2E}}
	os.MkdirAll(dir, 0o700)
	for i, t := range threads {
		if i >= 8 {
			break
		}
		if bmp := notifyBitmap(t, 96); bmp != 0 {
			name := filepath.Join(dir, "powiadomienie-"+string(rune('1'+i))+".png")
			pGdipSaveImageToFile.Call(bmp, uintptr(unsafe.Pointer(u16(name))), uintptr(unsafe.Pointer(&png)), 0)
			pGdipDisposeImage.Call(bmp)
		}
	}
}
