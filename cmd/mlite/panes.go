//go:build windows

package main

// Extra conversations open beside the heads. "Kolumny" and "Dok" show several conversations
// at once: the first is the panel of the overlay itself, with everything the window can do,
// and each further one is a pane of its own: a header, a view of the conversation and a
// field to write in. A pane reads, scrolls, loads older messages and sends text; replying to
// a message, attachments and the large picture stay with the first conversation.
//
// Columns stand side by side with no gap, the same height, growing away from the head. The
// dock is a row of equal panes along the bottom of the screen, each of which can be folded
// down to its header.

import (
	"strings"
	"unsafe"

	"lazyfmess/internal/store"
)

const (
	headModeCols = "cols"
	headModeDock = "dock"

	maxPanes = 2 // beside the panel of the overlay: three conversations in all
)

type pane struct {
	id                int64
	hwnd, view, input uintptr
	bank              *viewBank
	thread            store.Thread
	min               bool // folded down to its header (the dock)
	used              int
	w, h              int32
	closeR, minR      rect
	sendR, fieldR     rect
	active            bool
}

var (
	panes     []*pane
	paneClock int
)

func multiMode() bool { return cfg.HeadMode == headModeCols || cfg.HeadMode == headModeDock }

func paneFor(id int64) *pane {
	for _, p := range panes {
		if p.id == id {
			return p
		}
	}
	return nil
}

// paneOf finds the pane a window belongs to: the pane itself, its view or its field.
func paneOf(hwnd uintptr) *pane {
	for _, p := range panes {
		if hwnd == p.hwnd || hwnd == p.view || hwnd == p.input {
			return p
		}
	}
	return nil
}

// paneHeaderH is the height of the header of the panel, so that a row of conversations
// has one line along its top.
func paneHeaderH() int32 {
	if h := dp(18) + sp(38); h > dp(56) {
		return h
	}
	return dp(56)
}

// paneOpen opens a pane for a conversation, making room first when there are too many.
func paneOpen(id int64) {
	t, ok := threadByID(id)
	if !ok {
		return
	}
	if len(panes) >= maxPanes {
		oldest := panes[0]
		for _, p := range panes {
			if p.used < oldest.used {
				oldest = p
			}
		}
		paneClose(oldest)
	}
	p := &pane{id: id, thread: t, bank: &viewBank{}}
	p.hwnd = createWindow(wsExToolWindow|wsExTopmost, "MlitePane", threadTitle(t), wsPopup|wsThickFrame|wsClipChildren, 0, 0)
	if p.hwnd == 0 {
		return
	}
	panes = append(panes, p)
	b := p.bank
	b.mv.hover, b.mv.hotAct = -1, -1
	pendingBank = b
	p.view = createWindow(0, "MliteView", "Wiadomości", wsChild|wsVisible|wsTabStop, p.hwnd, 0)
	pendingBank = nil
	bankByView[p.view] = b
	b.mv.hwnd = p.view
	b.mv.sc = scroller{hwnd: p.view, pos: &b.mv.scroll, total: func() int32 { return b.mv.total }, page: func() int32 { return b.mv.h }, changed: viewScrolled}
	b.mv.thread, b.mv.has = t, true
	b.mv.hasNew, b.mv.newAfter = t.Unread(), t.LastRead
	p.input = createWindow(0, "EDIT", "", wsChild|wsVisible|wsTabStop|esAutoHScroll, p.hwnd, 1)
	sendMsg(p.input, wmSetFont, fnt.body, 1)
	sendMsg(p.input, emSetCueBanner, 1, uintptr(unsafe.Pointer(u16("Napisz wiadomość…"))))
	paneFrame(p)
	panesPlace()
	pShowWindow.Call(p.hwnd, swShow)
	paneLoad(p, true)
	be.openThread(t, true)
	paneFront(p)
	headsSync(false)
}

// paneFrame gives the pane the edge the panel of the overlay has: rounded in the dock,
// square where columns stand shoulder to shoulder.
func paneFrame(p *pane) {
	round, border := uintptr(dwmRoundSmall), uintptr(cref(pal.line))
	if cfg.HeadMode == headModeCols {
		round = dwmRoundNone
	}
	if p.active {
		border = uintptr(cref(pal.accent))
	}
	pDwmSetWindowAttribute.Call(p.hwnd, dwmaCornerPreference, uintptr(unsafe.Pointer(&round)), 4)
	pDwmSetWindowAttribute.Call(p.hwnd, dwmaBorderColour, uintptr(unsafe.Pointer(&border)), 4)
}

func paneFront(p *pane) {
	if p.min {
		p.min = false
		panesPlace()
	}
	pSetForegroundWindow.Call(p.hwnd)
	pSetFocus.Call(p.input)
}

func paneClose(p *pane) {
	for i, q := range panes {
		if q == p {
			panes = append(panes[:i], panes[i+1:]...)
			break
		}
	}
	delete(bankByView, p.view)
	pDestroyWindow.Call(p.hwnd)
	panesPlace()
	headsSync(false)
}

// panesCloseAll puts every pane away: the way of showing conversations was changed.
func panesCloseAll() {
	for len(panes) > 0 {
		paneClose(panes[0])
	}
}

// paneLoad fills the view of a pane from the store.
func paneLoad(p *pane, force bool) {
	defer enterBank(p.bank)()
	loadView(force)
}

// panesRefresh runs when the store changed: every pane shows what is there now, and the one
// in front counts as read.
func panesRefresh() {
	if len(panes) == 0 {
		return
	}
	fg, _, _ := pGetForegroundWindow.Call()
	for _, p := range panes {
		t, ok := threadByID(p.id)
		if !ok {
			continue
		}
		if threadTitle(t) != threadTitle(p.thread) {
			invalidate(p.hwnd)
		}
		p.thread = t
		func() {
			defer enterBank(p.bank)()
			mv.thread = t
			loadView(false)
		}()
		if fg == p.hwnd && !p.min && t.Unread() {
			be.openThread(t, true)
		}
	}
}

// panesOlder is told that a page of older messages arrived for a conversation.
func panesOlder(thread int64) {
	for _, p := range panes {
		pKillTimer.Call(p.view, timerSpin)
		if p.id == thread {
			func() {
				defer enterBank(p.bank)()
				mv.limit += windowStep
				loadView(true)
			}()
		}
	}
}

// panesMedia is told that a picture or the card of a link arrived: the panes are laid out
// again, since either may have changed the height of a message.
func panesMedia() {
	for _, p := range panes {
		func() {
			defer enterBank(p.bank)()
			relayoutView()
		}()
		invalidate(p.view)
	}
}

func panesInvalidate() {
	for _, p := range panes {
		paneFrame(p)
		invalidate(p.hwnd)
		invalidate(p.view)
	}
}

// paneShown reports whether a conversation is being read in a pane right now.
func paneShown(id int64) bool {
	p := paneFor(id)
	if p == nil || p.min {
		return false
	}
	fg, _, _ := pGetForegroundWindow.Call()
	return fg == p.hwnd
}

// panesPlace puts the panes in their places: after the panel of the overlay when it is up,
// each one further from the head than the one before.
func panesPlace() {
	if len(panes) == 0 {
		return
	}
	mainUp := false
	if vis, _, _ := pIsWindowVisible.Call(hMain); compact.on && vis != 0 {
		mainUp = true
	}
	h := heads[compact.forID] // where the row began, also once the panel is put away
	if h == nil {
		for _, p := range panes {
			if heads[p.id] != nil {
				h = heads[p.id]
				break
			}
		}
	}
	var hr, work rect
	pSystemParametersInfo.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0)
	if h != nil {
		pGetWindowRect.Call(h.hwnd, uintptr(unsafe.Pointer(&hr)))
	} else { // no head to stand by: the lower right corner of the screen
		hr = rect{work.right - headSize() - dp(16), work.bottom - headSize() - dp(120), work.right - dp(16), work.bottom - dp(120)}
	}
	x, y, w, ht := panelPlaceAt(hr)
	dir, gap := int32(-1), int32(0)
	if x > hr.left {
		dir = 1
	}
	if cfg.HeadMode == headModeDock {
		gap = dp(8)
	}
	slot := int32(0)
	if mainUp {
		slot = 1
	}
	for _, p := range panes {
		px, py, ph := x+dir*slot*(w+gap), y, ht
		if px+w > work.right-dp(4) {
			px = work.right - dp(4) - w
		}
		if px < work.left+dp(4) {
			px = work.left + dp(4)
		}
		if p.min {
			ph = paneHeaderH()
			py = y + ht - ph
		}
		pSetWindowPos.Call(p.hwnd, 0, uintptr(px), uintptr(py), uintptr(w), uintptr(ph), swpNoZOrder|swpNoActivate)
		slot++
	}
}

// paneSend sends what is in the field of a pane.
func paneSend(p *pane) {
	text := strings.TrimSpace(getText(p.input))
	if text == "" {
		return
	}
	if be.current() == nil {
		done := enterBank(p.bank)
		viewToast("Brak połączenia. Wiadomość nie została wysłana.")
		done()
		return
	}
	if be.send(p.thread, text, store.Message{}) {
		setText(p.input, "")
		invalidate(p.hwnd)
	}
}

func paneLayout(p *pane) {
	hh, comp := paneHeaderH(), dp(56)
	p.closeR = rect{p.w - dp(10) - dp(36), (hh - dp(36)) / 2, p.w - dp(10), (hh + dp(36)) / 2}
	p.minR = rect{}
	if cfg.HeadMode == headModeDock {
		p.minR = rect{p.closeR.left - dp(4) - dp(36), p.closeR.top, p.closeR.left - dp(4), p.closeR.bottom}
	}
	if p.min || p.h <= hh+comp {
		show(p.view, false)
		show(p.input, false)
		return
	}
	show(p.view, true)
	show(p.input, true)
	moveTo(p.view, rect{0, hh + 1, p.w, p.h - comp - 1})
	fh := dp(36)
	p.sendR = rect{p.w - dp(12) - dp(36), p.h - comp + (comp-dp(36))/2, p.w - dp(12), p.h - comp + (comp+dp(36))/2}
	p.fieldR = rect{dp(12), p.h - comp + (comp-fh)/2, p.sendR.left - dp(8), p.h - comp + (comp+fh)/2}
	f := p.fieldR
	moveTo(p.input, rect{f.left + dp(10), f.top + (fh-lay.editH)/2, f.right - dp(10), f.top + (fh-lay.editH)/2 + lay.editH})
}

func panePaint(p *pane, target uintptr) {
	buf := newBuffer(target, p.w, p.h)
	c := canvas{buf.hdc}
	hh, comp := paneHeaderH(), dp(56)
	c.fill(rect{0, 0, p.w, p.h}, pal.layer)
	c.fill(rect{0, 0, p.w, hh}, pal.card)
	c.fill(rect{0, hh, p.w, hh + 1}, pal.line)
	t := p.thread
	g := newGfx(buf.hdc)
	face := threadAvatar(g, t, dp(12), (hh-dp(32))/2, dp(32), pal.card)
	if !p.min && p.h > hh+comp {
		c.fill(rect{0, p.h - comp, p.w, p.h}, pal.card)
		c.fill(rect{0, p.h - comp - 1, p.w, p.h - comp}, pal.line)
		f := p.fieldR
		focus, _, _ := pGetFocus.Call()
		edge, inset := mix(pal.fg2, pal.line, 65), int32(1)
		if focus == p.input {
			edge, inset = pal.accent, 2
		}
		rr := dp(8)
		g.round(f.left, f.top, f.right-f.left, f.bottom-f.top, rr, rr, rr, rr, edge)
		g.round(f.left+inset, f.top+inset, f.right-f.left-2*inset, f.bottom-f.top-2*inset, rr-inset, rr-inset, rr-inset, rr-inset, pal.card)
		s, fill := p.sendR, pal.accent
		if strings.TrimSpace(getText(p.input)) == "" {
			fill = mix(pal.fg, pal.card, 8)
		}
		g.round(s.left, s.top, s.right-s.left, s.bottom-s.top, rr, rr, rr, rr, fill)
	}
	g.free()
	if !face {
		c.font(fnt.avSm)
		c.text(initials(threadTitle(t)), rect{dp(12), (hh - dp(32)) / 2, dp(44), (hh + dp(32)) / 2}, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	}
	right := p.closeR.left - dp(8)
	if p.minR.right > p.minR.left {
		right = p.minR.left - dp(8)
		c.font(fnt.head)
		c.text("–", p.minR, pal.fg2, dtCenter|dtVCenter|dtSingleLine)
	}
	c.font(fnt.icon)
	c.text(gClose, p.closeR, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	sub := ""
	switch {
	case isGroup(t):
		sub = "Rozmowa grupowa"
	case isE2EE(t):
		sub = "Rozmowa szyfrowana"
	}
	x := dp(54)
	c.font(fnt.head)
	if sub == "" || p.min {
		c.text(threadTitle(t), rect{x, 0, right, hh}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
	} else {
		top := (hh - sp(38)) / 2
		c.text(threadTitle(t), rect{x, top, right, top + sp(20)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
		c.font(fnt.small)
		c.text(sub, rect{x, top + sp(20), right, top + sp(38)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
	}
	if !p.min && p.h > hh+comp {
		ink := pal.onAccent
		if strings.TrimSpace(getText(p.input)) == "" {
			ink = pal.fg2
		}
		c.font(fnt.icon)
		c.text(gSend, p.sendR, ink, dtCenter|dtVCenter|dtSingleLine)
	}
	buf.flush(0, 0)
}

func paneProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	p := paneOf(hwnd)
	if p == nil || p.hwnd != hwnd {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
		return r
	}
	in := func(r rect, x, y int32) bool { return r.right > r.left && x >= r.left && x < r.right && y >= r.top && y < r.bottom }
	switch msg {
	case wmNcCalcSize:
		if wp != 0 {
			return 0 // the whole pane is its content; the frame stays only for its edge
		}
	case wmNcHitTest:
		return htClient
	case wmNcPaint:
		return 0
	case wmNcActivate:
		return 1
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		panePaint(p, hdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSize:
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		p.w, p.h = rc.right, rc.bottom
		paneLayout(p)
		invalidate(hwnd)
		return 0
	case wmActivate:
		p.active = wp&0xFFFF != 0
		paneFrame(p)
		if p.active {
			paneClock++
			p.used = paneClock
			if !p.min {
				pSetFocus.Call(p.input)
				if t, ok := threadByID(p.id); ok && t.Unread() {
					be.openThread(t, true)
				}
			}
			headsSync(false)
		}
		return 0
	case wmLButtonUp:
		x, y := int32(int16(lp)), int32(int16(lp>>16))
		switch {
		case in(p.closeR, x, y):
			paneClose(p)
		case in(p.minR, x, y):
			p.min = !p.min
			panesPlace()
		case p.min && y < paneHeaderH(): // the folded pane opens again from its header
			paneFront(p)
		case in(p.sendR, x, y):
			paneSend(p)
		case in(p.fieldR, x, y):
			pSetFocus.Call(p.input)
		}
		return 0
	case wmCtlColorEdit:
		pSetTextColor.Call(wp, cref(pal.fg))
		pSetBkColor.Call(wp, cref(pal.card))
		return brCard
	case wmCommand:
		if code := (wp >> 16) & 0xFFFF; code == enSetFocus || code == enKillFocus || code == enChange {
			invalidate(hwnd)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
