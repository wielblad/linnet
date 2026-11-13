//go:build windows

package main

// Chat heads: a pinned conversation shown as a small circle that stays on top of other
// windows, with the unread count and, when allowed, a short look at the newest message.

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"lazyfmess/internal/store"
)

var (
	pCreateDIBSection     = gdi32.NewProc("CreateDIBSection")
	pUpdateLayeredWindow  = user32.NewProc("UpdateLayeredWindow")
	pSetWindowPos         = user32.NewProc("SetWindowPos")
	pGetWindowRect        = user32.NewProc("GetWindowRect")
	pSetCapture           = user32.NewProc("SetCapture")
	pReleaseCapture       = user32.NewProc("ReleaseCapture")
	pSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
	pGdipFillEllipseA     = pGdipFillEllipseI
)

const (
	wsPopup         = 0x80000000
	wsExLayered     = 0x00080000
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExNoActivate  = 0x08000000
	wsExTransparent = 0x00000020
	swShowNA        = 8
	swpQuiet        = 0x0001 | 0x0004 | 0x0010 // keep size and order, do not activate

	idHeadOpen  = 430
	idHeadUnpin = 431
	idRowPin    = 432
	idRowUnpin  = 433
)

type headCfg struct {
	ID int64 `json:"id"`
	X  int32 `json:"x"`
	Y  int32 `json:"y"`
}

type head struct {
	id               int64
	hwnd, peek       uintptr
	key              uint64
	unread           int
	down, dragging   bool
	startX, startY   int32 // pointer when the button went down
	originX, originY int32 // window position at that moment
}

type bitmapInfoHeader struct {
	size          uint32
	width, height int32
	planes, bits  uint16
	compression   uint32
	sizeImage     uint32
	xppm, yppm    int32
	used, needed  uint32
}

var (
	heads     = map[int64]*head{}
	headByWnd = map[uintptr]*head{}
	unreadBy  = map[int64]int{}
)

func headSize() int32 { return dp(64) }

func isPinned(id int64) bool {
	for _, h := range cfg.Heads {
		if h.ID == id {
			return true
		}
	}
	return false
}

func pinHead(id int64) {
	if isPinned(id) {
		return
	}
	var work rect
	pSystemParametersInfo.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0) // work area
	x := work.right - headSize() - dp(16)
	y := work.top + dp(160) + int32(len(cfg.Heads))*(headSize()+dp(8))
	cfg.Heads = append(cfg.Heads, headCfg{ID: id, X: x, Y: y})
	saveConfig()
	headsSync(false)
}

func unpinHead(id int64) {
	for i, h := range cfg.Heads {
		if h.ID == id {
			cfg.Heads = append(cfg.Heads[:i], cfg.Heads[i+1:]...)
			break
		}
	}
	saveConfig()
	headsSync(false)
}

// layer puts a drawing with transparency on a layered window.
func layer(hwnd uintptr, w, h int32, draw func(g gfx)) {
	screen, _, _ := pGetDC.Call(0)
	mem, _, _ := pCreateCompatibleDC.Call(screen)
	bi := bitmapInfoHeader{width: w, height: -h, planes: 1, bits: 32}
	bi.size = uint32(unsafe.Sizeof(bi))
	var bits uintptr
	dib, _, _ := pCreateDIBSection.Call(screen, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib != 0 {
		old, _, _ := pSelectObject.Call(mem, dib)
		g := newGfx(mem)
		pGdipSetTextHint.Call(uintptr(g), 4)
		draw(g)
		g.free()
		size := [2]int32{w, h}
		var src point
		blend := [4]byte{0, 0, 255, 1} // source over, per-pixel alpha
		pUpdateLayeredWindow.Call(hwnd, screen, 0, uintptr(unsafe.Pointer(&size)), mem, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), 2)
		pSelectObject.Call(mem, old)
		pDeleteObject.Call(dib)
	}
	pDeleteDC.Call(mem)
	pReleaseDC.Call(0, screen)
}

func (g gfx) ellipseA(x, y, w, h int32, argb uint32) {
	var br uintptr
	pGdipCreateSolidFill.Call(uintptr(argb), uintptr(unsafe.Pointer(&br)))
	pGdipFillEllipseA.Call(uintptr(g), br, iarg(x), iarg(y), iarg(w), iarg(h))
	pGdipDeleteBrush.Call(br)
}

// paintHead draws one head: soft shadow, picture or initials, unread badge.
func paintHead(g gfx, id int64, title string, unread int) {
	s := headSize()
	d := dp(52)
	ax, ay := (s-d)/2, s-d-dp(4)
	g.ellipseA(ax-1, ay+dp(1), d+2, d+dp(3), 0x30000000)
	g.ellipseA(ax, ay+dp(1), d, d+dp(1), 0x40000000)
	if !g.avatar(id, ax, ay, d) {
		g.label(initials(title), ax, ay, d, d, dp(19), true, pal.fg)
	}
	if unread > 0 {
		text := countLabel(unread)
		bh := dp(22)
		bw := bh + int32(len(text)-1)*dp(7)
		bx := s - bw - dp(1)
		g.pill(bx-dp(2), 0, bw+dp(4), bh+dp(4), 0xFFFFFF)
		g.pill(bx, dp(2), bw, bh, pal.accent)
		g.label(text, bx, dp(1), bw, bh, dp(13), true, pal.onAccent)
	}
}

// headsSync creates, removes and redraws the heads so they match the pinned list and the unread counts.
// announce allows the short look at a new message.
func headsSync(announce bool) {
	want := map[int64]bool{}
	for _, hc := range cfg.Heads {
		want[hc.ID] = true
		h := heads[hc.ID]
		if h == nil {
			hwnd := createWindow(wsExLayered|wsExTopmost|wsExToolWindow|wsExNoActivate, "MliteHead", "Rozmowa", wsPopup, 0, 0)
			if hwnd == 0 {
				continue
			}
			h = &head{id: hc.ID, hwnd: hwnd, unread: unreadBy[hc.ID]}
			heads[hc.ID], headByWnd[hwnd] = h, h
			pSetWindowPos.Call(hwnd, 0, uintptr(hc.X), uintptr(hc.Y), uintptr(headSize()), uintptr(headSize()), 0x0004|0x0010)
		}
		t, known := threadByID(hc.ID)
		title := "?"
		if known {
			title = threadTitle(t)
		}
		unread := unreadBy[hc.ID]
		sum := fnv.New64a()
		fmt.Fprintf(sum, "%s|%d|%v|%06x|%d|%v", title, unread, avatarImage(hc.ID, dp(52)) != 0, pal.accent, dpi, pal.dark)
		if key := sum.Sum64(); key != h.key {
			h.key = key
			layer(h.hwnd, headSize(), headSize(), func(g gfx) { paintHead(g, hc.ID, title, unread) })
			pShowWindow.Call(h.hwnd, swShowNA)
		}
		if announce && known && unread > h.unread && cfg.HeadPreview && cfg.Body && !cfg.DND && !quietThread(h.id) && !conversationShown(h.id) {
			showPeek(h, title, oneLine(previewText(t.LastPreview), 260), firstURL(t.LastPreview))
		}
		if unread == 0 {
			hidePeek(h)
		}
		h.unread = unread
	}
	for id, h := range heads {
		if !want[id] {
			hidePeek(h)
			pDestroyWindow.Call(h.hwnd)
			delete(headByWnd, h.hwnd)
			delete(heads, id)
		}
	}
}

// headsRedraw forces every head to be drawn again, after a picture arrived or the theme changed.
func headsRedraw() {
	for _, h := range heads {
		h.key = 0
	}
	headsSync(false)
}

func headsClose() {
	for id, h := range heads {
		hidePeek(h)
		pDestroyWindow.Call(h.hwnd)
		delete(headByWnd, h.hwnd)
		delete(heads, id)
	}
}

// showPeek shows the newest message beside the head for a few seconds.
func showPeek(h *head, title, text, link string) {
	if text == "" {
		text = "Nowa wiadomość"
	}
	hidePeek(h) // made anew each time: its size and whether it takes clicks depend on the message
	p := &peekData{title: title, text: emojify(text), link: link}
	p.measure()
	w, ht := p.w, p.h
	ex := uint32(wsExLayered | wsExTopmost | wsExToolWindow | wsExNoActivate)
	if link == "" {
		ex |= wsExTransparent // nothing to press: clicks go through to what is underneath
	}
	h.peek = createWindow(ex, "MlitePeek", "Nowa wiadomość", wsPopup, 0, 0)
	if h.peek == 0 {
		return
	}
	pSetLayeredAttrs.Call(h.peek, 0, 255, 2) // opaque; layered only so that clicks can pass through
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(ht+1), uintptr(dp(24)), uintptr(dp(24)))
	pSetWindowRgn.Call(h.peek, rgn, 0)
	peeks[h.peek] = p
	var r, work rect
	pGetWindowRect.Call(h.hwnd, uintptr(unsafe.Pointer(&r)))
	pSystemParametersInfo.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0)
	x := r.left - w - dp(8)
	if (r.left+r.right)/2 < (work.left+work.right)/2 {
		x = r.right + dp(8)
	}
	y := r.top + (headSize()-ht)/2
	if y+ht > work.bottom-dp(8) {
		y = work.bottom - dp(8) - ht
	}
	if y < work.top+dp(8) {
		y = work.top + dp(8)
	}
	pSetWindowPos.Call(h.peek, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), 0x0004|0x0010)
	pShowWindow.Call(h.peek, swShowNA)
	invalidate(h.peek)
	stay := uintptr(7000)
	if link != "" {
		stay = 10000 // long enough to reach for the link
	}
	pSetTimer.Call(h.hwnd, 1, stay, 0)
}

func hidePeek(h *head) {
	if h.peek != 0 {
		delete(peeks, h.peek)
		pDestroyWindow.Call(h.peek)
		h.peek = 0
	}
}

// compact remembers that the main window is shown small beside a head, and where it was before.
var compact struct {
	on      bool
	restore rect
	forID   int64
}

// openHead shows the conversation in a small window beside the head; a second click puts it away.
func openHead(h *head) {
	t, ok := threadByID(h.id)
	if !ok {
		showMain()
		return
	}
	hidePeek(h)
	if vis, _, _ := pIsWindowVisible.Call(hMain); compact.on && compact.forID == h.id && vis != 0 {
		closeWindow()
		return
	}
	if !compact.on {
		pGetWindowRect.Call(hMain, uintptr(unsafe.Pointer(&compact.restore)))
		compact.on = true
	}
	compact.forID = h.id
	w, ht := dp(380), dp(580)
	var hr, work rect
	pGetWindowRect.Call(h.hwnd, uintptr(unsafe.Pointer(&hr)))
	pSystemParametersInfo.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0)
	x := hr.left - w - dp(10)
	if (hr.left+hr.right)/2 < (work.left+work.right)/2 {
		x = hr.right + dp(10)
	}
	y := hr.top - dp(120)
	if y+ht > work.bottom-dp(8) {
		y = work.bottom - dp(8) - ht
	}
	if y < work.top+dp(8) {
		y = work.top + dp(8)
	}
	if r, _, _ := pIsIconic.Call(hMain); r != 0 {
		pShowWindow.Call(hMain, swRestore)
	}
	pSetWindowPos.Call(hMain, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), 0x0004|0x0040)
	pSetForegroundWindow.Call(hMain)
	refresh()
	selectThread(t, true)
}

func headProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	h := headByWnd[hwnd]
	if h == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
		return r
	}
	switch msg {
	case wmLButtonDown:
		var pt point
		var r rect
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		h.down, h.dragging = true, false
		h.startX, h.startY, h.originX, h.originY = pt.x, pt.y, r.left, r.top
		pSetCapture.Call(hwnd)
		return 0
	case wmMouseMove:
		if !h.down {
			return 0
		}
		var pt point
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		dx, dy := pt.x-h.startX, pt.y-h.startY
		if !h.dragging && (dx > dp(4) || dx < -dp(4) || dy > dp(4) || dy < -dp(4)) {
			h.dragging = true
			hidePeek(h)
		}
		if h.dragging {
			pSetWindowPos.Call(hwnd, 0, uintptr(h.originX+dx), uintptr(h.originY+dy), 0, 0, swpQuiet)
		}
		return 0
	case wmLButtonUp:
		if !h.down {
			return 0
		}
		h.down = false
		pReleaseCapture.Call()
		if h.dragging {
			var r rect
			pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
			for i := range cfg.Heads {
				if cfg.Heads[i].ID == h.id {
					cfg.Heads[i].X, cfg.Heads[i].Y = r.left, r.top
				}
			}
			saveConfig()
		} else {
			openHead(h)
		}
		return 0
	case wmRButtonUp:
		menu, _, _ := pCreatePopupMenu.Call()
		pAppendMenuW.Call(menu, mfString, idHeadOpen, uintptr(unsafe.Pointer(u16("Otwórz rozmowę"))))
		pAppendMenuW.Call(menu, mfString, idHeadUnpin, uintptr(unsafe.Pointer(u16("Odepnij główkę"))))
		var pt point
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		pSetForegroundWindow.Call(hMain)
		cmd, _, _ := pTrackPopupMenu.Call(menu, tpmRightBtn|tpmReturnCmd|tpmNoNotify, uintptr(pt.x), uintptr(pt.y), 0, hMain, 0)
		pDestroyMenu.Call(menu)
		switch cmd {
		case idHeadOpen:
			openHead(h)
		case idHeadUnpin:
			unpinHead(h.id)
		}
		return 0
	case wmTimer:
		pKillTimer.Call(hwnd, wp)
		hidePeek(h)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func peekProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		paintPeek(hwnd, hdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0021: // WM_MOUSEACTIVATE: a click here never takes the keyboard from another program
		return 3
	case wmMouseMove:
		if p := peeks[hwnd]; p != nil && p.link != "" {
			x, y := lpPoint(lp)
			r := p.linkRect()
			hot := x >= r.left && x < r.right && y >= r.top && y < r.bottom
			if hot != p.hot {
				p.hot = hot
				invalidate(hwnd)
				if hot {
					trackLeave(hwnd)
				}
			}
		}
		return 0
	case wmMouseLeave:
		if p := peeks[hwnd]; p != nil && p.hot {
			p.hot = false
			invalidate(hwnd)
		}
		return 0
	case wmSetCursor:
		if p := peeks[hwnd]; p != nil && p.hot {
			cur, _, _ := pLoadCursorW.Call(0, idcHand)
			pSetCursor.Call(cur)
			return 1
		}
	case wmLButtonUp:
		if p := peeks[hwnd]; p != nil && p.link != "" {
			x, y := lpPoint(lp)
			if r := p.linkRect(); x >= r.left && x < r.right && y >= r.top && y < r.bottom {
				peekOpen(hwnd)
			}
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

// rowMenu is the menu of a conversation in the list.
func rowMenu(i int, lp uintptr) {
	if i < 0 || i >= len(rows) || rows[i].person != nil {
		return
	}
	t := rows[i].t
	menu, _, _ := pCreatePopupMenu.Call()
	if isPinned(t.ID) {
		pAppendMenuW.Call(menu, mfString, idRowUnpin, uintptr(unsafe.Pointer(u16("Odepnij główkę"))))
	} else {
		pAppendMenuW.Call(menu, mfString, idRowPin, uintptr(unsafe.Pointer(u16("Przypnij jako główkę na pulpicie"))))
	}
	pt := point{int32(int16(lp)), int32(int16(lp >> 16))}
	pClientToScreen.Call(hList, uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(hMain)
	cmd, _, _ := pTrackPopupMenu.Call(menu, tpmRightBtn|tpmReturnCmd|tpmNoNotify, uintptr(pt.x), uintptr(pt.y), 0, hMain, 0)
	pDestroyMenu.Call(menu)
	rowAction(int(cmd), t)
}

func rowAction(cmd int, t store.Thread) {
	switch cmd {
	case idRowPin:
		pinHead(t.ID)
	case idRowUnpin:
		unpinHead(t.ID)
	}
}

// dumpHeads saves pictures of the heads as PNG files, to look at them without the desktop.
func dumpHeads(dir string) {
	png := syscall.GUID{Data1: 0x557CF406, Data2: 0x1A04, Data3: 0x11D3, Data4: [8]byte{0x9A, 0x73, 0x00, 0x00, 0xF8, 0x1E, 0xF3, 0x2E}}
	os.MkdirAll(dir, 0o700)
	for _, hc := range cfg.Heads {
		t, _ := threadByID(hc.ID)
		bmp, g := newBitmap(headSize(), headSize())
		if bmp == 0 {
			continue
		}
		paintHead(g, hc.ID, threadTitle(t), unreadBy[hc.ID])
		g.free()
		name := filepath.Join(dir, fmt.Sprintf("head-%d.png", hc.ID))
		pGdipSaveImageToFile.Call(bmp, uintptr(unsafe.Pointer(u16(name))), uintptr(unsafe.Pointer(&png)), 0)
		pGdipDisposeImage.Call(bmp)
	}
}
