//go:build windows

package main

// The window of GIFs and stickers: opened from the button beside the microphone. Its first
// leaf shows the GIFs in fashion and, as words are typed, the ones found for them; its
// second the packs of stickers the account has in Messenger and what is in the chosen one.
// A click sends the GIF or the sticker to the conversation. Both are asked of Messenger
// through the connection; the small pictures in the window come straight from where they
// are kept.

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/store"
)

const (
	wmAppGif     = wmApp + 20 // an answer has come; wp is the question it answers
	wmAppGifPic  = wmApp + 21 // a small picture has come
	timerGifType = 33         // the words have stopped changing: search for them
	idGif        = 334
	idGifEdit    = 335

	tabGifs     = 0
	tabStickers = 1
)

var (
	hGif    uintptr
	gifFake = os.Getenv("MLITE_GIFS") // a file with a search answer, for looking at the window without a connection
	gifTab  = tabGifs                 // the leaf the window was last left on

	gp struct {
		hwnd, edit uintptr
		oldEdit    uintptr
		thread     store.Thread
		bank       *viewBank
		w, h       int32
		gifs       []meta.GIF
		stickers   []meta.Sticker
		packs      []meta.StickerPack
		pack       int
		cells      []rect // where each one is drawn, before scrolling
		total      int32
		scroll     int32
		hot        int // the GIF or sticker under the pointer
		hotTop     int // the tab (0, 1) or the pack (10 and on) under the pointer; -1 for none
		seq        int
		status     string
		pics       map[string]uintptr // scaled pictures by what they are of
		asked      map[string]bool
		mu         sync.Mutex
		answers    map[int]pickAnswer
		sem        chan struct{}
	}
)

type pickAnswer struct {
	kind     string // gifs, packs or stickers
	gifs     []meta.GIF
	packs    []meta.StickerPack
	stickers []meta.Sticker
	err      error
}

func gifPad() int32   { return dp(10) }
func gifTabs() rect   { return rect{gifPad(), dp(8), gp.w - gifPad(), dp(8) + dp(32)} }
func gifBand() rect   { return rect{gifPad(), dp(48), gp.w - gifPad(), dp(48) + dp(36)} }
func gifTop() int32   { return dp(48) + dp(36) + dp(10) }
func packSide() int32 { return dp(36) }

// gifOpen opens the window above a point of the screen: the upper left corner of its button.
func gifOpen(x, y int32) {
	t, ok := threadByID(hostThreadID())
	if !ok {
		return
	}
	if be.current() == nil && gifFake == "" {
		viewToast("Brak połączenia. GIF-y i naklejki są pobierane z Messengera.")
		return
	}
	gifClose()
	epickClose()
	gp.thread, gp.bank = t, curBank
	gp.w, gp.h = dp(360), dp(480)
	gp.gifs, gp.stickers, gp.packs, gp.pack = nil, nil, nil, 0
	gp.cells, gp.scroll, gp.total, gp.hot, gp.hotTop = nil, 0, 0, -1, -1
	gp.pics, gp.asked, gp.answers = map[string]uintptr{}, map[string]bool{}, map[int]pickAnswer{}
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
	gp.hwnd = createWindow(wsExTopmost|wsExToolWindow, "MliteGif", "GIF-y i naklejki", wsPopup|wsClipChildren, 0, 0)
	if gp.hwnd == 0 {
		return
	}
	r := dp(12)
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(gp.w+1), uintptr(gp.h+1), uintptr(2*r), uintptr(2*r))
	pSetWindowRgn.Call(gp.hwnd, rgn, 0)
	gp.edit = createWindow(0, "EDIT", "", wsChild|wsTabStop|esAutoHScroll, gp.hwnd, idGifEdit)
	sendMsg(gp.edit, wmSetFont, fnt.body, 1)
	sendMsg(gp.edit, emSetCueBanner, 1, uintptr(unsafe.Pointer(u16("Szukaj GIF-ów"))))
	gp.oldEdit = subclass(gp.edit, gifEditProc)
	eh, band := lay.editH, gifBand()
	moveTo(gp.edit, rect{band.left + dp(12), band.top + (dp(36)-eh)/2, band.right - dp(12), band.top + (dp(36)-eh)/2 + eh})
	pSetWindowPos.Call(gp.hwnd, 0, uintptr(px), uintptr(py), uintptr(gp.w), uintptr(gp.h), 0x0004)
	pShowWindow.Call(gp.hwnd, swShow)
	pSetForegroundWindow.Call(gp.hwnd)
	gifShowTab(gifTab)
}

// gifShowTab turns the window to one of its leaves and asks for what belongs on it.
func gifShowTab(tab int) {
	gifTab = tab
	gp.cells, gp.scroll, gp.total, gp.hot = nil, 0, 0, -1
	show(gp.edit, tab == tabGifs)
	if tab == tabGifs {
		pSetFocus.Call(gp.edit)
		pickAsk("gifs", getText(gp.edit))
		return
	}
	pSetFocus.Call(gp.hwnd)
	if len(gp.packs) == 0 {
		pickAsk("packs", "")
		return
	}
	pickAsk("stickers", gp.packs[gp.pack].ID)
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

// pickAsk asks Messenger for GIFs, for the packs of stickers or for the stickers of a
// pack, away from the window.
func pickAsk(kind, arg string) {
	gp.seq++
	seq, win := gp.seq, gp.hwnd
	gp.status = "Wczytuję…"
	invalidate(gp.hwnd)
	client := be.current()
	go func() {
		a := pickAnswer{kind: kind}
		switch {
		case gifFake != "": // for looking at the window only: answers kept in files
			name := map[string]string{"gifs": filepath.Base(gifFake), "packs": "packs.json", "stickers": "pack.json"}[kind]
			body, err := os.ReadFile(filepath.Join(filepath.Dir(gifFake), name))
			a.err = err
			if err == nil {
				switch kind {
				case "gifs":
					a.gifs, a.err = meta.ParseGIFs(body)
				case "packs":
					a.packs, a.err = meta.ParseStickerPacks(body)
				default:
					a.stickers, a.err = meta.ParseStickers(body)
				}
			}
		case client == nil:
			a.err = fmt.Errorf("no connection")
		case kind == "gifs":
			a.gifs, a.err = client.SearchGIFs(be.ctx, arg)
		case kind == "packs":
			a.packs, a.err = client.StickerPacks(be.ctx)
		default:
			a.stickers, a.err = client.Stickers(be.ctx, arg)
		}
		gp.mu.Lock()
		gp.answers[seq] = a
		gp.mu.Unlock()
		postMsg(win, wmAppGif, uintptr(seq), 0)
	}()
}

// pickArrived takes in an answer, unless something else has been asked since.
func pickArrived(seq int) {
	gp.mu.Lock()
	a, ok := gp.answers[seq]
	delete(gp.answers, seq)
	gp.mu.Unlock()
	if !ok || seq != gp.seq {
		return
	}
	gp.status = ""
	if a.err != nil {
		gp.status = "Nie udało się wczytać. Spróbuj ponownie."
		if typingDebug {
			viewToast(a.err.Error())
		}
		invalidate(gp.hwnd)
		return
	}
	switch a.kind {
	case "gifs":
		gp.gifs = a.gifs
		if len(a.gifs) == 0 {
			gp.status = "Nic nie znaleziono."
		}
	case "packs":
		gp.packs, gp.pack = a.packs, 0
		if len(a.packs) == 0 {
			gp.status = "Nie masz jeszcze paczek naklejek w Messengerze."
			break
		}
		pickAsk("stickers", a.packs[0].ID) // the first pack at once
		return
	default:
		gp.stickers = a.stickers
		if len(a.stickers) == 0 {
			gp.status = "Ta paczka jest pusta."
		}
	}
	gp.scroll, gp.hot = 0, -1
	gifLay()
	invalidate(gp.hwnd)
}

// gifLay places what the leaf shows: GIFs in two columns, each under the shorter so far;
// stickers four in a row.
func gifLay() {
	gp.cells, gp.total = gp.cells[:0], 0
	if gifTab == tabStickers {
		const cols = 4
		cw := (gp.w - gifPad()*(cols+1)) / cols
		for i := range gp.stickers {
			x := gifPad() + int32(i%cols)*(cw+gifPad())
			y := int32(i/cols) * (cw + gifPad())
			gp.cells = append(gp.cells, rect{x, y, x + cw, y + cw})
			gp.total = y + cw + gifPad()
		}
		return
	}
	const cols = 2
	cw := (gp.w - gifPad()*(cols+1)) / cols
	var colY [cols]int32
	for _, g := range gp.gifs {
		col := 0
		if colY[1] < colY[0] {
			col = 1
		}
		h := cw * int32(g.Height) / int32(g.Width)
		if h > cw*2 {
			h = cw * 2
		}
		x := gifPad() + int32(col)*(cw+gifPad())
		gp.cells = append(gp.cells, rect{x, colY[col], x + cw, colY[col] + h})
		colY[col] += h + gifPad()
	}
	gp.total = max(colY[0], colY[1])
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

// pickPath is where the small picture kept under a name lives on the disk.
func pickPath(kind, name string) string {
	h := fnv.New64a()
	h.Write([]byte(name))
	return filepath.Join(dataDir, "media", fmt.Sprintf("%s-%016x.img", kind, h.Sum64()))
}

// pickPicture gives the scaled picture kept under a key, asking for its file when it is
// not here yet. cover cuts the picture to the box; without it the picture is fitted whole.
func pickPicture(key, path, from string, w, h, radius int32, cover bool) uintptr {
	if img, ok := gp.pics[key]; ok {
		return img
	}
	if _, err := os.Stat(path); err == nil {
		shown, _, _ := drawable(path) // a WebP picture becomes a PNG copy the window can draw
		img := thumbOf(shown, w, h, radius, cover)
		gp.pics[key] = img
		return img
	}
	if !gp.asked[key] && from != "" {
		gp.asked[key] = true
		win := gp.hwnd
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
	if y < gifTop() {
		return -1
	}
	y += gp.scroll - gifTop()
	for i, c := range gp.cells {
		if x >= c.left && x < c.right && y >= c.top && y < c.bottom {
			return i
		}
	}
	return -1
}

// tabRect is one of the two tabs; packRect one of the packs of the sticker leaf.
func tabRect(i int) rect {
	t := gifTabs()
	half := (t.right - t.left) / 2
	return rect{t.left + int32(i)*half, t.top, t.left + int32(i+1)*half, t.bottom}
}

func packRect(i int) rect {
	b := gifBand()
	x := b.left + int32(i)*(packSide()+dp(6))
	return rect{x, b.top, x + packSide(), b.top + packSide()}
}

// packsShown is how many packs fit in the row.
func packsShown() int {
	b := gifBand()
	n := int((b.right - b.left + dp(6)) / (packSide() + dp(6)))
	return min(n, len(gp.packs))
}

// gifTopAt tells what of the upper part is at a point: a tab (0, 1), a pack (10 and on), -1.
func gifTopAt(x, y int32) int {
	in := func(r rect) bool { return x >= r.left && x < r.right && y >= r.top && y < r.bottom }
	for i := 0; i < 2; i++ {
		if in(tabRect(i)) {
			return i
		}
	}
	if gifTab == tabStickers {
		for i := 0; i < packsShown(); i++ {
			if in(packRect(i)) {
				return 10 + i
			}
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
	case wmKeyDown:
		if wp == vkEscape {
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
			if gifTab == tabGifs {
				pickAsk("gifs", getText(gp.edit))
			}
		}
		return 0
	case wmAppGif:
		pickArrived(int(wp))
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
		if hot, top := gifAt(x, y), gifTopAt(x, y); hot != gp.hot || top != gp.hotTop {
			gp.hot, gp.hotTop = hot, top
			invalidate(hwnd)
		}
		return 0
	case wmLButtonUp:
		x, y := lpPoint(lp)
		switch top := gifTopAt(x, y); {
		case top == tabGifs || top == tabStickers:
			if top != gifTab {
				gifShowTab(top)
			}
			return 0
		case top >= 10 && top-10 < len(gp.packs):
			if gp.pack != top-10 {
				gp.pack = top - 10
				gp.stickers, gp.cells, gp.scroll, gp.hot = nil, nil, 0, -1
				pickAsk("stickers", gp.packs[gp.pack].ID)
			}
			return 0
		}
		i, t := gifAt(x, y), gp.thread
		switch {
		case i < 0:
		case gifTab == tabGifs && i < len(gp.gifs):
			g := gp.gifs[i]
			gifClose()
			if be.sendGIF(t, g) {
				viewToast("Wysyłam GIF-a…")
			}
		case gifTab == tabStickers && i < len(gp.stickers):
			st := gp.stickers[i]
			gifClose()
			if be.sendSticker(t, st) {
				viewToast("Wysyłam naklejkę…")
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
	// what scrolls first, so that the upper part covers what goes under it
	for i, cell := range gp.cells {
		y := cell.top + top - gp.scroll
		w, h := cell.right-cell.left, cell.bottom-cell.top
		if y > gp.h || y+h < top {
			continue
		}
		var img uintptr
		if gifTab == tabGifs && i < len(gp.gifs) {
			gif := gp.gifs[i]
			img = pickPicture("g"+gif.ID, pickPath("gifp", gif.URL), gifStillURL(gif), w, h, dp(8), true)
		} else if gifTab == tabStickers && i < len(gp.stickers) {
			st := gp.stickers[i]
			id := strconv.FormatInt(st.ID, 10)
			if i == gp.hot {
				g.round(cell.left-dp(4), y-dp(4), w+dp(8), h+dp(8), dp(8), dp(8), dp(8), dp(8), mix(pal.fg, pal.layer, 9))
			}
			img = pickPicture("s"+id, pickPath("stk", id), st.Image, w, h, 1, false)
		}
		switch {
		case img != 0:
			g.image(img, cell.left, y, w, h)
		case gifTab == tabGifs:
			g.round(cell.left, y, w, h, dp(8), dp(8), dp(8), dp(8), mix(pal.fg, pal.layer, 7))
		}
		if gifTab == tabGifs && i == gp.hot { // a ring in the accent around the one under the pointer
			t := dp(3)
			g.round(cell.left-t, y-t, w+2*t, t, 1, 1, 1, 1, pal.accent)
			g.round(cell.left-t, y+h, w+2*t, t, 1, 1, 1, 1, pal.accent)
			g.round(cell.left-t, y-t, t, h+2*t, 1, 1, 1, 1, pal.accent)
			g.round(cell.left+w, y-t, t, h+2*t, 1, 1, 1, 1, pal.accent)
		}
	}
	g.round(1, 1, gp.w-2, top-dp(2), r-1, r-1, 1, 1, pal.card)
	// the two tabs: the chosen one filled
	tabs := gifTabs()
	g.round(tabs.left, tabs.top, tabs.right-tabs.left, tabs.bottom-tabs.top, dp(8), dp(8), dp(8), dp(8), mix(pal.fg, pal.card, 7))
	for i := 0; i < 2; i++ {
		t := tabRect(i)
		switch {
		case i == gifTab:
			g.round(t.left+dp(2), t.top+dp(2), t.right-t.left-dp(4), t.bottom-t.top-dp(4), dp(6), dp(6), dp(6), dp(6), pal.accent)
		case i == gp.hotTop:
			g.round(t.left+dp(2), t.top+dp(2), t.right-t.left-dp(4), t.bottom-t.top-dp(4), dp(6), dp(6), dp(6), dp(6), mix(pal.fg, pal.card, 14))
		}
	}
	band := gifBand()
	if gifTab == tabGifs { // the frame of the field
		g.round(band.left, band.top, band.right-band.left, band.bottom-band.top, dp(8), dp(8), dp(8), dp(8), pal.accent)
		g.round(band.left+2, band.top+2, band.right-band.left-4, band.bottom-band.top-4, dp(6), dp(6), dp(6), dp(6), pal.card)
	} else { // the packs
		for i := 0; i < packsShown(); i++ {
			p, pr := gp.packs[i], packRect(i)
			switch {
			case i == gp.pack:
				g.round(pr.left-dp(2), pr.top-dp(2), packSide()+dp(4), packSide()+dp(4), dp(8), dp(8), dp(8), dp(8), pal.accent)
				g.round(pr.left, pr.top, packSide(), packSide(), dp(6), dp(6), dp(6), dp(6), pal.card)
			case gp.hotTop == 10+i:
				g.round(pr.left-dp(2), pr.top-dp(2), packSide()+dp(4), packSide()+dp(4), dp(8), dp(8), dp(8), dp(8), mix(pal.fg, pal.card, 12))
			}
			if img := pickPicture("p"+p.ID, pickPath("stkp", p.ID), p.Thumb, packSide()-dp(6), packSide()-dp(6), 1, false); img != 0 {
				g.image(img, pr.left+dp(3), pr.top+dp(3), packSide()-dp(6), packSide()-dp(6))
			}
		}
	}
	g.free()
	c.fill(rect{1, top - dp(2), gp.w - 1, top - dp(1)}, pal.line)
	c.font(fnt.smallSemi)
	for i, label := range []string{"GIF-y", "Naklejki"} {
		ink := pal.fg
		if i == gifTab {
			ink = pal.onAccent
		}
		c.text(label, tabRect(i), ink, dtCenter|dtVCenter|dtSingleLine)
	}
	if gp.status != "" {
		c.font(fnt.small)
		c.text(gp.status, rect{gifPad(), top, gp.w - gifPad(), top + dp(60)}, pal.fg2, dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
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

// sendSticker sends a sticker and notes it locally, so that the conversation shows it at
// once. An encrypted chat is sent the picture itself, fetched first.
func (b *backend) sendSticker(thread store.Thread, st meta.Sticker) bool {
	client := b.current()
	if client == nil {
		b.notice("Brak połączenia. Naklejka nie została wysłana.")
		return false
	}
	go func() {
		path := filepath.Join(dataDir, "outbox", "naklejka-"+time.Now().Format("20060102-150405.000")+".png")
		if err := fetchTo(st.Image, path, 4<<20); err != nil {
			b.notice("Nie udało się pobrać naklejki.")
			return
		}
		defer os.Remove(path)
		result, err := client.SendSticker(b.ctx, thread, st, path)
		if err != nil {
			b.notice("Nie udało się wysłać naklejki: " + err.Error())
			return
		}
		at := result.Timestamp
		if at.IsZero() {
			at = time.Now()
		}
		if result.MessageID != "" {
			sent := store.Message{ID: result.MessageID, ThreadID: thread.ID, SenderID: client.AccountID(), Timestamp: at,
				MediaKind: "image", MediaMime: "image/png", MediaName: "sticker", MediaURL: st.Image}
			if data, err := os.ReadFile(path); err == nil {
				dst := mediaPath(sent)
				if os.MkdirAll(filepath.Dir(dst), 0o700) == nil {
					_ = os.WriteFile(dst, data, 0o600)
				}
			}
			_ = b.db.UpsertMessage(b.ctx, sent)
			_ = b.db.SetLastRead(b.ctx, thread.ID, at)
		}
		b.requestRefresh()
	}()
	return true
}
