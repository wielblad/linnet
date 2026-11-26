//go:build windows

package main

// A GIF moves in the conversation, as it does in Messenger. Only the ones on the screen
// are kept moving: each has its file open and is drawn anew, frame by frame, at the pace
// the file asks for. One that has left the screen is let go within a moment.
//
// Encrypted chats carry a GIF as a film that says it is to be played like a GIF, with the
// real GIF as its file: such a message is a picture here, not a film.

import (
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"lazyfmess/internal/store"
)

const (
	timerGif     = 30
	gifFrameProp = 0x5100 // PropertyTagFrameDelay: hundredths of a second, one per frame
)

var (
	pGdipImageGetFrameCount     = gdiplus.NewProc("GdipImageGetFrameCount")
	pGdipImageSelectActiveFrame = gdiplus.NewProc("GdipImageSelectActiveFrame")
	pGdipGetPropertyItemSize    = gdiplus.NewProc("GdipGetPropertyItemSize")
	pGdipGetPropertyItem        = gdiplus.NewProc("GdipGetPropertyItem")
	gifTimeDimension            = syscall.GUID{Data1: 0x6aedbd6d, Data2: 0x3fb5, Data3: 0x418a, Data4: [8]byte{0x83, 0xa6, 0x7f, 0x45, 0x22, 0x9d, 0xc8, 0x72}}

	gifs     = map[string]*gifAnim{}
	gifStill = map[string]bool{} // files found to have one frame only: nothing to move
)

type gifAnim struct {
	src    uintptr
	delays []time.Duration
	idx    int
	due    time.Time
	seen   time.Time
	w, h   int32
}

// gifAsPicture makes a picture of the film an encrypted chat carries a GIF as.
func gifAsPicture(m *store.Message) {
	if m.MediaKind == "video" && m.MediaMime == "image/gif" {
		m.MediaKind = "image"
	}
}

// isGif reports whether a message is a GIF: said by its kind of file, or, for one sent from
// here and not yet heard back from Messenger, by the name of its file.
func isGif(m store.Message) bool {
	return m.MediaKind == "image" && (m.MediaMime == "image/gif" || strings.EqualFold(filepath.Ext(m.MediaName), ".gif"))
}

// gifSeen is a GIF being drawn at a size: it is kept moving from now on.
func gifSeen(m store.Message, w, h int32) {
	if !isGif(m) || gifStill[m.ID] {
		return
	}
	a := gifs[m.ID]
	if a == nil {
		path := md.files[m.ID]
		if path == "" || len(gifs) >= 6 { // a few at a time are enough to look at
			return
		}
		var src uintptr
		if st, _, _ := pGdipLoadImageFromFile.Call(uintptr(unsafe.Pointer(u16(path))), uintptr(unsafe.Pointer(&src))); st != 0 || src == 0 {
			gifStill[m.ID] = true
			return
		}
		var n uint32
		pGdipImageGetFrameCount.Call(src, uintptr(unsafe.Pointer(&gifTimeDimension)), uintptr(unsafe.Pointer(&n)))
		if n < 2 {
			pGdipDisposeImage.Call(src)
			gifStill[m.ID] = true
			return
		}
		a = &gifAnim{src: src, delays: gifDelays(src, int(n))}
		a.due = time.Now().Add(a.delays[0])
		gifs[m.ID] = a
		pSetTimer.Call(hMain, timerGif, 20, 0)
	}
	a.w, a.h, a.seen = w, h, time.Now()
}

// gifDelays reads how long each frame of a GIF stays. A frame that asks for no time at
// all is given a tenth of a second, as browsers do.
func gifDelays(src uintptr, n int) []time.Duration {
	out := make([]time.Duration, n)
	for i := range out {
		out[i] = 100 * time.Millisecond
	}
	var size uint32
	if st, _, _ := pGdipGetPropertyItemSize.Call(src, gifFrameProp, uintptr(unsafe.Pointer(&size))); st != 0 || size < 24 {
		return out
	}
	buf := make([]byte, size)
	if st, _, _ := pGdipGetPropertyItem.Call(src, gifFrameProp, uintptr(size), uintptr(unsafe.Pointer(&buf[0]))); st != 0 {
		return out
	}
	length := *(*uint32)(unsafe.Pointer(&buf[4]))
	value := *(*uintptr)(unsafe.Pointer(&buf[16]))
	if value == 0 || int(length) < 4*n {
		return out
	}
	for i, v := range unsafe.Slice((*uint32)(unsafe.Pointer(value)), n) {
		if v >= 2 {
			out[i] = time.Duration(v) * 10 * time.Millisecond
		}
	}
	return out
}

// gifTick moves the GIFs that are due on, and lets go of those no longer on the screen.
func gifTick() {
	now := time.Now()
	moved := false
	for id, a := range gifs {
		if now.Sub(a.seen) > 700*time.Millisecond {
			pGdipDisposeImage.Call(a.src)
			delete(gifs, id)
			continue
		}
		if now.Before(a.due) {
			continue
		}
		a.idx = (a.idx + 1) % len(a.delays)
		a.due = now.Add(a.delays[a.idx])
		pGdipImageSelectActiveFrame.Call(a.src, uintptr(unsafe.Pointer(&gifTimeDimension)), uintptr(a.idx))
		img := thumbFrom(a.src, a.w, a.h, dp(10), false, 6) // a little coarser while it moves
		if img == 0 {
			continue
		}
		if t, ok := md.thumbs[id]; ok && t.img != 0 {
			pGdipDisposeImage.Call(t.img)
		}
		md.thumbs[id] = thumb{img, a.w, a.h}
		moved = true
	}
	if len(gifs) == 0 {
		pKillTimer.Call(hMain, timerGif)
	}
	if moved {
		audioRedraw() // every view of a conversation is drawn again
	}
}
