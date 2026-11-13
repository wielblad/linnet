//go:build windows

package main

// Notifications as Windows toasts with a box to answer in: used when the program is
// installed (toasts belong to its Start Menu shortcut); otherwise the plain notification
// from the notification area stays.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/rs/zerolog/log"

	"lazyfmess/internal/store"
	"lazyfmess/internal/toast"
)

const (
	wmAppToast  = wmApp + 11
	wmAppUpdate = wmApp + 12
)

var (
	toastOn    bool
	toastQueue = make(chan toast.Activation, 16)
)

// toastStart turns toasts on when the program runs installed.
func toastStart() {
	if _, err := os.Stat(shortcutPath()); err != nil {
		return
	}
	err := toast.Start(appID(), func(a toast.Activation) {
		select {
		case toastQueue <- a: // read on the window thread
		default:
		}
		postMsg(hMain, wmAppToast, 0, 0)
	})
	if err != nil {
		log.Warn().Err(err).Msg("toast notifications")
		return
	}
	toastOn = true
}

func toastTag(id int64) string { return strconv.FormatInt(id, 10) }

// toastAbout shows the toast of a conversation; false means the caller should fall back.
func toastAbout(t store.Thread, title, text string) bool {
	if !toastOn {
		return false
	}
	image := ""
	if bmp := notifyBitmap(t, 96); bmp != 0 {
		dir := filepath.Join(dataDir, "notify")
		os.MkdirAll(dir, 0o700)
		path := filepath.Join(dir, toastTag(t.ID)+".png")
		png := syscall.GUID{Data1: 0x557CF406, Data2: 0x1A04, Data3: 0x11D3, Data4: [8]byte{0x9A, 0x73, 0x00, 0x00, 0xF8, 0x1E, 0xF3, 0x2E}}
		if st, _, _ := pGdipSaveImageToFile.Call(bmp, uintptr(unsafe.Pointer(u16(path))), uintptr(unsafe.Pointer(&png)), 0); st == 0 {
			image = path
		}
		pGdipDisposeImage.Call(bmp)
	}
	n := toast.Notification{
		Title: title, Body: text, Image: image,
		Tag: toastTag(t.ID), Group: "rozmowy",
		Args:  "open:" + toastTag(t.ID),
		Reply: be.current() != nil || os.Getenv("MLITE_TOASTREPLY") != "", ReplyHint: "Napisz odpowiedź…", ReplyLabel: "Wyślij",
		ReplyArgs: "reply:" + toastTag(t.ID),
		Silent:    true, // the program plays the sound itself
	}
	if err := toast.Show(n); err != nil {
		log.Warn().Err(err).Msg("show toast")
		return false
	}
	return true
}

// toastGone takes back the toast of a conversation the user has now seen.
func toastGone(id int64) {
	if toastOn {
		toast.Remove(toastTag(id), "rozmowy")
	}
}

// toastArrived handles what the user did with toasts: a click opens the conversation, a
// reply is sent to it.
func toastArrived() {
	for {
		select {
		case a := <-toastQueue:
			verb, raw, _ := strings.Cut(a.Args, ":")
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				continue
			}
			refresh()
			t, ok := threadByID(id)
			if !ok {
				continue
			}
			switch {
			case verb == "reply" && strings.TrimSpace(a.Text) != "":
				if be.send(t, strings.TrimSpace(a.Text), store.Message{}) {
					toastGone(id)
				} else {
					balloon(appName, fmt.Sprintf("Nie udało się wysłać odpowiedzi do: %s", oneLine(threadTitle(t), 40)))
				}
			default:
				showMain()
				postMsg(hMain, wmAppOpen, 0, uintptr(id))
			}
			continue
		default:
		}
		return
	}
}

// toastQuit takes the program's toasts away when it ends: left behind they would stay in
// the notification centre and answer to nothing.
func toastQuit() {
	if toastOn {
		toastOn = false
		toast.Remove("", "rozmowy")
		toast.Stop()
	}
}
