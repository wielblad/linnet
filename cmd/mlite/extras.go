//go:build windows

package main

// Four smaller pieces: pictures in formats the drawing library cannot read (WebP stickers),
// the preview beside a chat head, the list's name for screen readers, and pasting a
// picture or a file from the clipboard.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/rs/zerolog/log"
)

// ---- WebP and other formats through the Windows imaging component

var (
	ole32             = syscall.NewLazyDLL("ole32.dll")
	pCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	pCoCreateInstance = ole32.NewProc("CoCreateInstance")
	pCoUninitialize   = ole32.NewProc("CoUninitialize")
	wicLast           string // the step that failed last, for diagnosis
)

// toPNG rewrites a picture as PNG with the Windows imaging component, which reads WebP.
func toPNG(src, dst string) bool {
	// The WebP decoder refuses to start on the window's own thread, so the conversion runs
	// on a thread of its own and the caller waits for it.
	done := make(chan bool, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		pCoInitializeEx.Call(0, 0) // multithreaded
		defer pCoUninitialize.Call()
		done <- convertToPNG(src, dst)
	}()
	return <-done
}

func convertToPNG(src, dst string) bool {
	clsid := syscall.GUID{Data1: 0xcacaf262, Data2: 0x9370, Data3: 0x4615, Data4: [8]byte{0xa1, 0x3b, 0x9f, 0x55, 0x39, 0xda, 0x4c, 0x0a}}
	iid := syscall.GUID{Data1: 0xec5ec8a9, Data2: 0xc395, Data3: 0x4314, Data4: [8]byte{0x9c, 0x77, 0x54, 0xd7, 0xa9, 0x35, 0xff, 0x70}}
	png := syscall.GUID{Data1: 0x1b7cfaf4, Data2: 0x713f, Data3: 0x473c, Data4: [8]byte{0xbb, 0xcd, 0x61, 0x37, 0x42, 0x5f, 0xae, 0xaf}}
	var factory, decoder, frame, stream, encoder, target uintptr
	release := func(p uintptr) {
		if p != 0 {
			com(p, 2)
		}
	}
	if hr, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&factory))); hr != 0 || factory == 0 {
		wicLast = fmt.Sprintf("factory %08X", uint32(hr))
		return false
	}
	defer release(factory)
	// CreateDecoderFromFilename
	if hr := com(factory, 3, uintptr(unsafe.Pointer(u16(src))), 0, 0x80000000, 0, uintptr(unsafe.Pointer(&decoder))); hr != 0 || decoder == 0 {
		wicLast = fmt.Sprintf("decoder %08X", uint32(hr))
		return false
	}
	defer release(decoder)
	if hr := com(decoder, 13, 0, uintptr(unsafe.Pointer(&frame))); hr != 0 || frame == 0 { // GetFrame
		wicLast = fmt.Sprintf("frame %08X", uint32(hr))
		return false
	}
	defer release(frame)
	if hr := com(factory, 14, uintptr(unsafe.Pointer(&stream))); hr != 0 || stream == 0 { // CreateStream
		wicLast = fmt.Sprintf("stream %08X", uint32(hr))
		return false
	}
	defer release(stream)
	tmp := dst + ".tmp"
	if hr := com(stream, 15, uintptr(unsafe.Pointer(u16(tmp))), 0x40000000); hr != 0 { // InitializeFromFilename
		wicLast = fmt.Sprintf("stream file %08X", uint32(hr))
		return false
	}
	ok := false
	if hr := com(factory, 8, uintptr(unsafe.Pointer(&png)), 0, uintptr(unsafe.Pointer(&encoder))); hr == 0 && encoder != 0 { // CreateEncoder
		if com(encoder, 3, stream, 2) == 0 && com(encoder, 10, uintptr(unsafe.Pointer(&target)), 0) == 0 && target != 0 { // Initialize, CreateNewFrame
			ok = com(target, 3, 0) == 0 && com(target, 11, frame, 0) == 0 && com(target, 12) == 0 && com(encoder, 11) == 0 // Initialize, WriteSource, Commit, Commit
			release(target)
		}
		release(encoder)
	}
	com(stream, 2)
	stream = 0
	if !ok {
		wicLast = "encode"
		os.Remove(tmp)
		return false
	}
	return os.Rename(tmp, dst) == nil
}

// imageSize returns the pixel size of a picture file the drawing library can read.
func imageSize(path string) (int32, int32) {
	var src uintptr
	var w, h uint32
	if st, _, _ := pGdipLoadImageFromFile.Call(uintptr(unsafe.Pointer(u16(path))), uintptr(unsafe.Pointer(&src))); st == 0 && src != 0 {
		pGdipGetImageWidth.Call(src, uintptr(unsafe.Pointer(&w)))
		pGdipGetImageHeight.Call(src, uintptr(unsafe.Pointer(&h)))
		pGdipDisposeImage.Call(src)
	}
	return int32(w), int32(h)
}

// drawable returns a file the drawing library can show for a fetched picture: the file
// itself, or a PNG copy made beside it when the format (WebP) needs converting.
func drawable(path string) (string, int32, int32) {
	// WebP always goes through the copy: the drawing library may report its size and still not draw it.
	if strings.ToLower(filepath.Ext(path)) != ".webp" {
		if w, h := imageSize(path); w > 0 && h > 0 {
			return path, w, h
		}
	}
	copyPath := path + ".png"
	if _, err := os.Stat(copyPath); err != nil && !toPNG(path, copyPath) {
		log.Warn().Str("step", wicLast).Msg("picture conversion")
		return path, 0, 0
	}
	w, h := imageSize(copyPath)
	return copyPath, w, h
}

// ---- the preview beside a chat head: an ordinary small window, so emoji are in colour

type peekData struct {
	title, text, link string
	w, h, textH       int32
	hot               bool // the pointer is on "Otwórz link"
}

var (
	pSetLayeredAttrs = user32.NewProc("SetLayeredWindowAttributes")
	peeks            = map[uintptr]*peekData{} // by window
)

// peekMeasure sizes the bubble for its text: up to four lines of the message, and one more
// line, the one that opens the link, when the message carries one.
func (p *peekData) measure() {
	p.w = dp(340)
	lineH := fnt.lineH
	withFont(fnt.body, func(c canvas) { _, p.textH = c.measure(p.text, p.w-dp(32)) })
	if p.textH < lineH {
		p.textH = lineH
	}
	if p.textH > 4*lineH {
		p.textH = 4 * lineH
	}
	p.h = dp(14) + sp(22) + dp(2) + p.textH + dp(14)
	if p.link != "" {
		p.h += sp(32)
	}
}

// linkRect is the line that opens the link, in the bubble's own coordinates.
func (p *peekData) linkRect() rect {
	return rect{1, p.h - sp(32) - dp(4), p.w - 1, p.h - 1}
}

func paintPeek(hwnd, target uintptr) {
	p := peeks[hwnd]
	if p == nil {
		return
	}
	w, h := p.w, p.h
	buf := newBuffer(target, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.line)
	g := newGfx(buf.hdc)
	g.round(1, 1, w-2, h-2, dp(11), dp(11), dp(11), dp(11), pal.card)
	lr := p.linkRect()
	if p.link != "" {
		if p.hot {
			g.round(lr.left+dp(6), lr.top+dp(3), lr.right-lr.left-dp(12), lr.bottom-lr.top-dp(6), dp(8), dp(8), dp(8), dp(8), mix(pal.accent, pal.card, 14))
		}
		g.round(dp(16), lr.top, w-dp(32), 1, 0, 0, 0, 0, pal.line)
	}
	g.free()
	c.font(fnt.name)
	c.text(p.title, rect{dp(16), dp(14), w - dp(16), dp(14) + sp(22)}, pal.accentText, dtSingleLine|dtVCenter|dtEndEllipsis)
	c.font(fnt.body)
	ty := dp(14) + sp(22) + dp(2)
	c.text(p.text, rect{dp(16), ty, w - dp(16), ty + p.textH}, pal.fg, dtWordBreak|dtEndEllipsis)
	if p.link != "" {
		c.font(fnt.icon)
		c.text(gLink, rect{dp(16), lr.top, dp(38), lr.bottom}, pal.accentText, dtSingleLine|dtVCenter)
		c.font(fnt.name)
		c.text("Otwórz link · "+linkHost(p.link), rect{dp(42), lr.top, w - dp(16), lr.bottom}, pal.accentText, dtSingleLine|dtVCenter|dtEndEllipsis)
	}
	buf.flush(0, 0)
}

// conversationShown reports whether the conversation is on screen in the program's window,
// whatever its size: then a bubble beside its head would only repeat what is already seen.
func conversationShown(id int64) bool {
	if paneShown(id) {
		return true
	}
	if id == 0 || selectedID != id || nav == navSettings || (lay.narrow && !lay.chat) || lg.active {
		return false
	}
	vis, _, _ := pIsWindowVisible.Call(hMain)
	min, _, _ := pIsIconic.Call(hMain)
	return vis != 0 && min == 0
}

// peekOpen opens the link of a bubble in the default browser.
func peekOpen(hwnd uintptr) {
	p := peeks[hwnd]
	if p == nil || p.link == "" {
		return
	}
	link := p.link
	pShowWindow.Call(hwnd, swHide)
	openURL(link)
}

// ---- screen readers: the list tells them which conversation the keyboard is on

var pNotifyWinEvent = user32.NewProc("NotifyWinEvent")

func rowLabel(i int) string {
	if i < 0 || i >= len(rows) {
		return "Rozmowy"
	}
	r := rows[i]
	label := r.title
	if r.unread > 0 {
		label += fmt.Sprintf(", nieprzeczytane: %d", r.unread)
	}
	if r.sub != "" {
		label += ", " + r.sub
	}
	if r.preview != "" && nav == navChats {
		label += ", " + r.preview
	}
	if r.when != "" && nav == navChats {
		label += ", " + r.when
	}
	return label
}

// listAnnounce names the list after the selected row and reports the change, which is what
// a screen reader reads when the selection moves.
func listAnnounce() {
	setText(hList, rowLabel(ls.sel))
	pNotifyWinEvent.Call(0x800C, hList, ^uintptr(3), 0) // name change of the client area
}

// ---- pasting a picture or a file

var (
	pIsClipboardFormat = user32.NewProc("IsClipboardFormatAvailable")
	pGetClipboardData  = user32.NewProc("GetClipboardData")
	pDragQueryFileW    = shell32.NewProc("DragQueryFileW")
	pGdipFromHBITMAP   = gdiplus.NewProc("GdipCreateBitmapFromHBITMAP")
)

// attachFile puts a file above the text field, ready to send.
func attachFile(path string) bool {
	if _, ok := threadByID(selectedID); !ok {
		return false
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return false
	}
	if compose.mode == cmEdit {
		setText(hInput, "")
	}
	if compose.mode == cmFile && compose.file != path {
		dropPasted(compose.file)
	}
	compose.mode, compose.file = cmFile, path
	inputChanged()
	layout()
	pSetFocus.Call(hInput)
	return true
}

// composePic is the small picture of the image waiting to be sent, kept between repaints.
var composePic struct {
	img  uintptr
	path string
	side int32
}

func composeThumbSide() int32 { return dp(56) }

// composeThumb returns the preview of the attachment above the text field when it is a
// picture the drawing library reads; 0 for other files (the bar then shows text only).
func composeThumb() uintptr {
	side := composeThumbSide()
	if compose.mode != cmFile || fileKind(compose.file) != "image" {
		composeThumbDrop()
		return 0
	}
	if composePic.path == compose.file && composePic.side == side {
		return composePic.img
	}
	composeThumbDrop()
	composePic.path, composePic.side = compose.file, side
	src := compose.file
	if strings.ToLower(filepath.Ext(src)) == ".webp" {
		// The drawing library does not read WebP: a PNG copy is made in the program's own
		// folder (never beside the user's file) and removed once the small picture exists.
		dir := filepath.Join(dataDir, "outbox")
		os.MkdirAll(dir, 0o700)
		copyPath := filepath.Join(dir, "podglad.png")
		if !toPNG(src, copyPath) {
			log.Warn().Str("step", wicLast).Msg("attachment preview")
			return 0
		}
		defer os.Remove(copyPath)
		src = copyPath
	}
	composePic.img = thumbOf(src, side, side, dp(6), true)
	return composePic.img
}

func composeThumbDrop() {
	if composePic.img != 0 {
		pGdipDisposeImage.Call(composePic.img)
	}
	composePic.img, composePic.path, composePic.side = 0, "", 0
}

// dropPasted removes a pasted picture once it is sent or given up. Files the user picked
// from their own folders are never touched: only the program's own outbox is.
func dropPasted(path string) {
	if path != "" && filepath.Dir(path) == filepath.Join(dataDir, "outbox") {
		os.Remove(path)
	}
}

// sweepOutbox clears pasted pictures left from earlier runs (a send that failed, a crash).
func sweepOutbox() {
	dir := filepath.Join(dataDir, "outbox")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// pasteAttachment handles Ctrl+V when the clipboard holds a file or a picture rather than text.
func pasteAttachment() bool {
	const cfBitmap, cfFiles = 2, 15
	files, _, _ := pIsClipboardFormat.Call(cfFiles)
	bitmap, _, _ := pIsClipboardFormat.Call(cfBitmap)
	if files == 0 && bitmap == 0 {
		return false
	}
	// Another program may hold the clipboard for a moment (clipboard history, the program
	// that has just copied), so opening it is tried a few times.
	opened := false
	for try := 0; try < 10 && !opened; try++ {
		if r, _, _ := pOpenClipboard.Call(hMain); r != 0 {
			opened = true
		} else {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !opened {
		log.Warn().Msg("paste: the clipboard is held by another program")
		be.notice("Nie udało się wkleić: schowek jest zajęty przez inny program. Spróbuj ponownie.")
		return true
	}
	path, step := "", ""
	if files != 0 {
		if h, _, _ := pGetClipboardData.Call(cfFiles); h != 0 {
			buf := make([]uint16, 1024)
			if n, _, _ := pDragQueryFileW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); n > 0 {
				path = syscall.UTF16ToString(buf)
			}
		}
	} else if h, _, _ := pGetClipboardData.Call(cfBitmap); h == 0 {
		step = "no bitmap"
	} else {
		var bmp uintptr
		st, _, _ := pGdipFromHBITMAP.Call(h, 0, uintptr(unsafe.Pointer(&bmp)))
		if bmp == 0 {
			step = fmt.Sprintf("bitmap %d", st)
		} else {
			dir := filepath.Join(dataDir, "outbox")
			os.MkdirAll(dir, 0o700)
			name := filepath.Join(dir, "wklejone-"+time.Now().Format("20060102-150405")+".png")
			png := syscall.GUID{Data1: 0x557CF406, Data2: 0x1A04, Data3: 0x11D3, Data4: [8]byte{0x9A, 0x73, 0x00, 0x00, 0xF8, 0x1E, 0xF3, 0x2E}}
			if st, _, _ := pGdipSaveImageToFile.Call(bmp, uintptr(unsafe.Pointer(u16(name))), uintptr(unsafe.Pointer(&png)), 0); st == 0 {
				path = name
			} else {
				step = fmt.Sprintf("save %d", st)
			}
			pGdipDisposeImage.Call(bmp)
		}
	}
	pCloseClipboard.Call()
	if step != "" {
		log.Warn().Str("step", step).Msg("paste: the picture could not be taken from the clipboard")
	}
	return path != "" && attachFile(path)
}
