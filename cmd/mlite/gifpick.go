//go:build windows

package main

// The window of GIFs: opened from the button beside the microphone, it shows the GIFs in
// fashion and, as words are typed, the ones found for them. A click sends the GIF to the
// conversation. The search is Messenger's own, asked through the connection; the small
// still pictures in the window come straight from the service the GIFs live on.

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/store"
)

const (
	wmAppGif     = wmApp + 20 // an answer to a search has come; wp is the search it answers
	wmAppGifPic  = wmApp + 21 // a small picture has come
	timerGifType = 33         // the words have stopped changing: search for them
	idGif        = 334
	idGifEdit    = 335
	gifCols      = 2
)

var (
	hGif    uintptr
	gifFake = os.Getenv("MLITE_GIFS") // a file with a search answer, for looking at the window without a connection

	gp struct {
		hwnd, edit uintptr
		oldEdit    uintptr
		thread     store.Thread
		bank       *viewBank
		w, h       int32
		items      []meta.GIF
		cells      []rect // where each one is drawn, before scrolling
		total      int32
		scroll     int32
		hot        int
		seq        int
		status     string
		pics       map[string]uintptr // scaled pictures by GIF
		asked      map[string]bool
		mu         sync.Mutex
		answers    map[int]gifAnswer
		sem        chan struct{}
	}
)

type gifAnswer struct {
	items []meta.GIF
	err   error
}

func gifPad() int32 { return dp(10) }
func gifTop() int32 { return dp(12) + dp(36) + dp(10) }

// gifOpen opens the window above a point of the screen: the upper left corner of its button.
func gifOpen(x, y int32) {
	t, ok := threadByID(hostThreadID())
	if !ok {
		return
	}
	if be.current() == nil && gifFake == "" {
		viewToast("Brak połączenia. GIF-y są wyszukiwane przez Messengera.")
		return
	}
	gifClose()
	epickClose()
	gp.thread, gp.bank = t, curBank
	gp.w, gp.h = dp(360), dp(460)
	gp.items, gp.cells, gp.scroll, gp.total, gp.hot = nil, nil, 0, 0, -1
	gp.pics, gp.asked, gp.answers = map[string]uintptr{}, map[string]bool{}, map[int]gifAnswer{}
	if gp.sem == nil {
		gp.sem = make(chan struct{}, 4)
	}
	work := workAt(rect{x, y, x + 1, y + 1})
	px, py := x, y-gp.h-dp(8)
	if px+gp.w > work.right-dp(8) {
		px = work.right - dp(8) - gp.w
	}
	if px < work.left+dp(8) {
		px = work.left + dp(8)
	}
	if py < work.top+dp(8) {
		py = work.top + dp(8)
	}
	gp.hwnd = createWindow(wsExTopmost|wsExToolWindow, "MliteGif", "GIF-y", wsPopup|wsClipChildren, 0, 0)
	if gp.hwnd == 0 {
		return
	}
	r := dp(12)
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(gp.w+1), uintptr(gp.h+1), uintptr(2*r), uintptr(2*r))
	pSetWindowRgn.Call(gp.hwnd, rgn, 0)
	gp.edit = createWindow(0, "EDIT", "", wsChild|wsVisible|wsTabStop|esAutoHScroll, gp.hwnd, idGifEdit)
	sendMsg(gp.edit, wmSetFont, fnt.body, 1)
	sendMsg(gp.edit, emSetCueBanner, 1, uintptr(unsafe.Pointer(u16("Szukaj GIF-ów"))))
	gp.oldEdit = subclass(gp.edit, gifEditProc)
	eh := lay.editH
	moveTo(gp.edit, rect{gifPad() + dp(12), dp(12) + (dp(36)-eh)/2, gp.w - gifPad() - dp(12), dp(12) + (dp(36)-eh)/2 + eh})
	pSetWindowPos.Call(gp.hwnd, 0, uintptr(px), uintptr(py), uintptr(gp.w), uintptr(gp.h), 0x0004)
	pShowWindow.Call(gp.hwnd, swShow)
	pSetForegroundWindow.Call(gp.hwnd)
	pSetFocus.Call(gp.edit)
	gifSearch("")
}

func gifClose() {
	if gp.hwnd == 0 {
		return
	}
	h := gp.hwnd
	gp.hwnd, gp.edit = 0, 0
	gp.seq++ // answers still on their way are for nobody
	for id, img := range gp.pics {
		if img != 0 {
			pGdipDisposeImage.Call(img)
		}
		delete(gp.pics, id)
	}
	pDestroyWindow.Call(h)
}

// gifEditProc lets Escape close the window from its field.
func gifEditProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if msg == wmKeyDown && wp == vkEscape {
		gifClose()
		return 0
	}
	r, _, _ := pCallWindowProcW.Call(gp.oldEdit, hwnd, uintptr(msg), wp, lp)
	return r
}

// gifSearch asks for the GIFs of some words, away from the window.
func gifSearch(words string) {
	gp.seq++
	seq, win := gp.seq, gp.hwnd
	gp.status = "Szukam…"
	invalidate(gp.hwnd)
	client := be.current()
	go func() {
		var a gifAnswer
		if gifFake != "" { // for looking at the window only: an answer kept in a file
			if body, err := os.ReadFile(gifFake); err == nil {
				a.items, a.err = meta.ParseGIFs(body)
			} else {
				a.err = err
			}
		} else if client == nil {
			a.err = fmt.Errorf("no connection")
		} else {
			a.items, a.err = client.SearchGIFs(be.ctx, words)
		}
		gp.mu.Lock()
		gp.answers[seq] = a
		gp.mu.Unlock()
		postMsg(win, wmAppGif, uintptr(seq), 0)
	}()
}

// gifArrived takes in the answer to a search, unless a later one has been asked since.
func gifArrived(seq int) {
	gp.mu.Lock()
	a, ok := gp.answers[seq]
	delete(gp.answers, seq)
	gp.mu.Unlock()
	if !ok || seq != gp.seq {
		return
	}
	gp.items, gp.scroll, gp.hot = a.items, 0, -1
	switch {
	case a.err != nil:
		gp.status = "Nie udało się wyszukać GIF-ów."
		if typingDebug {
			viewToast(a.err.Error())
		}
	case len(a.items) == 0:
		gp.status = "Nic nie znaleziono."
	default:
		gp.status = ""
	}
	gifLay()
	invalidate(gp.hwnd)
}

// gifLay places the GIFs in columns, each under the shortest so far.
func gifLay() {
	cw := (gp.w - gifPad()*(gifCols+1)) / gifCols
	var colY [gifCols]int32
	gp.cells = gp.cells[:0]
	for _, g := range gp.items {
		col := 0
		for c := 1; c < gifCols; c++ {
			if colY[c] < colY[col] {
				col = c
			}
		}
		h := cw * int32(g.Height) / int32(g.Width)
		if h > cw*2 {
			h = cw * 2
		}
		x := gifPad() + int32(col)*(cw+gifPad())
		gp.cells = append(gp.cells, rect{x, colY[col], x + cw, colY[col] + h})
		colY[col] += h + gifPad()
	}
	gp.total = 0
	for _, y := range colY {
		if y > gp.total {
			gp.total = y
		}
	}
}

// gifStillURL is the address of a small still picture of a GIF, where its service has one.
func gifStillURL(g meta.GIF) string {
	u, err := url.Parse(g.URL)
	if err == nil && strings.HasSuffix(u.Hostname(), ".giphy.com") && strings.HasSuffix(u.Path, "/giphy.gif") {
		u.Path = strings.TrimSuffix(u.Path, "giphy.gif") + "200w_s.gif"
		return u.String()
	}
	return g.URL
}

func gifStillPath(g meta.GIF) string {
	h := fnv.New64a()
	h.Write([]byte(g.URL))
	return filepath.Join(dataDir, "media", fmt.Sprintf("gifp-%016x.gif", h.Sum64()))
}

// gifPicture gives the scaled picture of a GIF, asking for its file when it is not here.
func gifPicture(g meta.GIF, w, h int32) uintptr {
	if img, ok := gp.pics[g.ID]; ok {
		return img
	}
	path := gifStillPath(g)
	if _, err := os.Stat(path); err == nil {
		img := thumbOf(path, w, h, dp(8), true)
		gp.pics[g.ID] = img
		return img
	}
	if !gp.asked[g.ID] {
		gp.asked[g.ID] = true
		win, from := gp.hwnd, gifStillURL(g)
		go func() {
			gp.sem <- struct{}{}
			err := fetchTo(from, path, 8<<20)
			<-gp.sem
			if err == nil {
				postMsg(win, wmAppGifPic, 0, 0)
			}
		}()
	}
	return 0
}

func gifAt(x, y int32) int {
	y += gp.scroll - gifTop()
	if y < 0 {
		return -1
	}
	for i, c := range gp.cells {
		if x >= c.left && x < c.right && y >= c.top && y < c.bottom {
			return i
		}
	}
	return -1
}

func gifClamp() {
	if max := gp.total - (gp.h - gifTop()); gp.scroll > max {
		gp.scroll = max
	}
	if gp.scroll < 0 {
		gp.scroll = 0
	}
}

func gifProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if hwnd != gp.hwnd {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
		return r
	}
	if gp.bank != nil && gp.bank != curBank {
		defer enterBank(gp.bank)()
	}
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		gifPaint(hdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmActivate:
		if wp&0xFFFF == 0 { // another window was chosen: this one goes
			gifClose()
		}
		return 0
	case wmCtlColorEdit:
		pSetTextColor.Call(wp, cref(pal.fg))
		pSetBkColor.Call(wp, cref(pal.card))
		return brCard
	case wmCommand:
		if (wp>>16)&0xFFFF == enChange {
			pSetTimer.Call(hwnd, timerGifType, 400, 0)
		}
		return 0
	case wmTimer:
		if wp == timerGifType {
			pKillTimer.Call(hwnd, timerGifType)
			gifSearch(getText(gp.edit))
		}
		return 0
	case wmAppGif:
		gifArrived(int(wp))
		return 0
	case wmAppGifPic:
		invalidate(hwnd)
		return 0
	case wmMouseWheel:
		gp.scroll -= int32(int16(wp>>16)) * dp(90) / 120
		gifClamp()
		invalidate(hwnd)
		return 0
	case wmMouseMove:
		x, y := lpPoint(lp)
		if hot := gifAt(x, y); hot != gp.hot {
			gp.hot = hot
			invalidate(hwnd)
		}
		return 0
	case wmLButtonUp:
		x, y := lpPoint(lp)
		if i := gifAt(x, y); i >= 0 && i < len(gp.items) {
			g, t := gp.items[i], gp.thread
			gifClose()
			if be.sendGIF(t, g) {
				viewToast("Wysyłam GIF-a…")
			}
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func gifPaint(hdc uintptr) {
	buf := newBuffer(hdc, gp.w, gp.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, gp.w, gp.h}, pal.edge)
	g := newGfx(buf.hdc)
	r := dp(12)
	g.round(1, 1, gp.w-2, gp.h-2, r-1, r-1, r-1, r-1, pal.layer)
	top := gifTop()
	// the GIFs first, so that the bar with the field covers what scrolls under it
	for i, cell := range gp.cells {
		y := cell.top + top - gp.scroll
		if y > gp.h || y+(cell.bottom-cell.top) < top {
			continue
		}
		w, h := cell.right-cell.left, cell.bottom-cell.top
		if img := gifPicture(gp.items[i], w, h); img != 0 {
			g.image(img, cell.left, y, w, h)
		} else {
			g.round(cell.left, y, w, h, dp(8), dp(8), dp(8), dp(8), mix(pal.fg, pal.layer, 7))
		}
		if i == gp.hot { // a ring in the accent around the one under the pointer
			g.round(cell.left-dp(3), y-dp(3), w+dp(6), dp(3), dp(1), dp(1), dp(1), dp(1), pal.accent)
			g.round(cell.left-dp(3), y+h, w+dp(6), dp(3), dp(1), dp(1), dp(1), dp(1), pal.accent)
			g.round(cell.left-dp(3), y-dp(3), dp(3), h+dp(6), dp(1), dp(1), dp(1), dp(1), pal.accent)
			g.round(cell.left+w, y-dp(3), dp(3), h+dp(6), dp(1), dp(1), dp(1), dp(1), pal.accent)
		}
	}
	g.round(1, 1, gp.w-2, top-dp(2), r-1, r-1, 1, 1, pal.card)
	f := rect{gifPad(), dp(12), gp.w - gifPad(), dp(12) + dp(36)}
	g.round(f.left, f.top, f.right-f.left, f.bottom-f.top, dp(8), dp(8), dp(8), dp(8), pal.accent)
	g.round(f.left+2, f.top+2, f.right-f.left-4, f.bottom-f.top-4, dp(6), dp(6), dp(6), dp(6), pal.card)
	g.free()
	c.fill(rect{1, top - dp(2), gp.w - 1, top - dp(1)}, pal.line)
	if gp.status != "" {
		c.font(fnt.small)
		c.text(gp.status, rect{0, top, gp.w, top + dp(60)}, pal.fg2, dtCenter|dtVCenter|dtSingleLine)
	}
	buf.flush(0, 0)
}

// sendGIF fetches a GIF and sends it to the conversation as its file: the way that is
// known to work in both kinds of chat.
func (b *backend) sendGIF(thread store.Thread, g meta.GIF) bool {
	if b.current() == nil {
		b.notice("Brak połączenia. GIF nie został wysłany.")
		return false
	}
	go func() {
		path := filepath.Join(dataDir, "outbox", "gif-"+time.Now().Format("20060102-150405")+".gif")
		if err := fetchTo(g.URL, path, 24<<20); err != nil {
			b.notice("Nie udało się pobrać GIF-a.")
			return
		}
		b.sendFile(thread, path)
	}()
	return true
}
