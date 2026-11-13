//go:build windows

package main

// Extra conversations open beside the heads. "Kolumny" and "Dok" show several conversations
// at once, each in a pane of its own: a window with a header, a view of the conversation
// and a field to write in. The panes of the heads of one group stand in one row beside the
// group; a head on its own has a row of its own.
//
// A pane does what the panel does with a conversation. The code is the same code: the view,
// what the field is for (a reply, an edit, a file), the menu and the reactions of a message,
// the large picture and the questions all work on the bank of the view they were asked from
// and on its host, which is the window or a pane.
//
// Columns stand side by side with no gap, the same height, growing away from the head. The
// dock is a row of equal panes along the bottom of the screen, each of which can be folded
// down to its header.

import (
	"syscall"
	"unsafe"

	"lazyfmess/internal/store"
)

const (
	headModeCols = "cols"
	headModeDock = "dock"
)

type pane struct {
	id                int64
	hwnd, view, input uintptr
	bank              *viewBank
	thread            store.Thread
	min               bool // folded down to its header (the dock)
	used              int
	w, h              int32
	lines             int   // lines of text the field shows
	compH             int32 // height of everything under the view
	empty             bool  // nothing typed: the field shows its hint
	closeR, minR      rect
	sendR, fieldR     rect
	attachR, barR     rect
	appR              rect  // the button to the ordinary window of the program
	side              int32 // where its row stands: 1 to the right of the heads, -1 to the left
	last              bool  // the one furthest from the heads: its outer edge changes the size
	hot               int   // which of its buttons is under the pointer
	active            bool
}

const (
	pbtNone = iota
	pbtClose
	pbtMin
	pbtAttach
	pbtSend
	pbtBarClose
	pbtApp
)

var (
	panes       []*pane
	paneClock   int
	paneInputCB uintptr

	pGetAncestor = user32.NewProc("GetAncestor")
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

// paneHost finds the pane a window is somewhere inside of: also a question or a picture
// shown large in it.
func paneHost(hwnd uintptr) *pane {
	if len(panes) == 0 || hwnd == 0 {
		return nil
	}
	root, _, _ := pGetAncestor.Call(hwnd, 2) // GA_ROOT
	for _, p := range panes {
		if p.hwnd == root {
			return p
		}
	}
	return nil
}

// ---- the host of the conversation being handled: the window, or a pane

func hostWnd() uintptr {
	if p := curBank.pane; p != nil {
		return p.hwnd
	}
	return hMain
}

func hostInput() uintptr {
	if p := curBank.pane; p != nil {
		return p.input
	}
	return hInput
}

func hostThreadID() int64 {
	if p := curBank.pane; p != nil {
		return p.id
	}
	return selectedID
}

func hostLayout() {
	if p := curBank.pane; p != nil {
		paneLayout(p)
		invalidate(p.hwnd)
		return
	}
	layout()
}

func hostInputChanged() {
	if p := curBank.pane; p != nil {
		paneInputChanged(p)
		return
	}
	inputChanged()
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
	for len(panes) >= paneLimit() {
		paneClose(paneOldest())
	}
	p := &pane{id: id, thread: t, bank: &viewBank{}, lines: 1, empty: true}
	p.bank.pane = p
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
	p.input = createWindow(0, "EDIT", "", wsChild|wsVisible|wsTabStop|esMultiline|esAutoVScroll|esWantReturn, p.hwnd, 1)
	sendMsg(p.input, wmSetFont, fnt.body, 1)
	if paneInputCB == 0 {
		paneInputCB = syscall.NewCallback(paneInputProc)
	}
	pSetWindowLongPtrW.Call(p.input, ^uintptr(3), paneInputCB) // GWLP_WNDPROC
	paneFrame(p)
	panesPlace()
	pShowWindow.Call(p.hwnd, swShow)
	paneLoad(p, true)
	be.openThread(t, true)
	paneFront(p)
	headsSync(false)
}

// paneOldest is the pane that has gone unused the longest.
func paneOldest() *pane {
	oldest := panes[0]
	for _, p := range panes {
		if p.used < oldest.used {
			oldest = p
		}
	}
	return oldest
}

// paneInputProc draws the hint that fields of several lines do not offer themselves, the
// way the field of the window does.
func paneInputProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := pCallWindowProcW.Call(oldInputProc, hwnd, uintptr(msg), wp, lp)
	p := paneOf(hwnd)
	if p == nil || !p.empty {
		return r
	}
	if msg == wmPaint {
		var rc rect
		sendMsg(hwnd, emGetRect, 0, uintptr(unsafe.Pointer(&rc)))
		hdc, _, _ := pGetDC.Call(hwnd)
		old, _, _ := pSelectObject.Call(hdc, fnt.body)
		canvas{hdc}.gdiText("Napisz wiadomość…", rc, pal.fg2, dtSingleLine)
		pSelectObject.Call(hdc, old)
		pReleaseDC.Call(hwnd, hdc)
	}
	if msg == wmSetFocus || msg == wmKillFocus {
		pInvalidateRect.Call(hwnd, 0, 1)
	}
	return r
}

// paneInputChanged keeps the height of the field, its hint and the send button in step
// with what is typed.
func paneInputChanged(p *pane) {
	n, _, _ := pGetWindowTextLengthW.Call(p.input)
	if empty := n == 0; empty != p.empty {
		p.empty = empty
		pInvalidateRect.Call(p.input, 0, 1)
	}
	lines := int(sendMsg(p.input, emGetLineCount, 0, 0))
	if lines < 1 {
		lines = 1
	}
	if lines > maxInputLines {
		lines = maxInputLines
	}
	if lines != p.lines {
		p.lines = lines
		paneLayout(p)
		sendMsg(p.input, emLineScroll, 0, ^uintptr(999))
		sendMsg(p.input, emScrollCaret, 0, 0)
	}
	invalidate(p.hwnd)
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
	// what was open inside the pane goes first
	if pick.hwnd != 0 && pick.bank == p.bank {
		closePicker()
	}
	if vw.hwnd != 0 && vw.host == p.hwnd {
		closeViewer()
	}
	if dlg.hwnd != 0 && dlg.host == p.hwnd {
		closeDialog(false)
	}
	if p.bank.compose.mode == cmFile {
		dropPasted(p.bank.compose.file)
	}
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
		invalidate(p.input)
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

// panesPlace puts the panes in their places: a row for every group of heads, and for every
// head on its own, each pane further from the heads than the one before. A row stands on
// the side of its heads that has room and stays there while the heads are dragged.
func panesPlace() {
	if len(panes) == 0 {
		for k := range rowSide {
			delete(rowSide, k)
		}
		return
	}
	work := workArea()
	w, ht := panelSize()
	dock := cfg.HeadMode == headModeDock
	gap := int32(0)
	if dock {
		gap = dp(8)
	}
	rows, order := map[int64][]*pane{}, []int64{}
	for _, p := range panes {
		k := entityKey(p.id)
		if rows[k] == nil {
			order = append(order, k)
		}
		rows[k] = append(rows[k], p)
	}
	for k := range rowSide {
		if rows[k] == nil {
			delete(rowSide, k)
		}
	}
	var placed []rect // the panes of rows placed so far: no row stands on another
	for _, k := range order {
		row := rows[k]
		er, ok := entityRect(row[0].id)
		if !ok { // its head was unpinned: the lower right corner of the screen
			er = rect{work.right - headSize() - dp(16), work.bottom - headSize() - dp(120), work.right - dp(16), work.bottom - dp(120)}
		}
		need := int32(len(row))*(w+gap) - gap
		roomR, roomL := work.right-dp(4)-(er.right+dp(10)), er.left-dp(10)-(work.left+dp(4))
		side := rowSide[k]
		if side == 0 {
			side = -1
			if roomR > roomL {
				side = 1
			}
		}
		switch { // no room on that side any more, and room on the other
		case side > 0 && roomR < need && roomL >= need:
			side = -1
		case side < 0 && roomL < need && roomR >= need:
			side = 1
		}
		rowSide[k] = side
		y := er.top - dp(120)
		if dock {
			y = work.bottom - dp(8) - ht
		}
		if y+ht > work.bottom-dp(8) {
			y = work.bottom - dp(8) - ht
		}
		if y < work.top+dp(8) {
			y = work.top + dp(8)
		}
		for i, p := range row {
			px := er.right + dp(10) + int32(i)*(w+gap)
			if side < 0 {
				px = er.left - dp(10) - w - int32(i)*(w+gap)
			}
			// Heads that are not tied together still stand close to each other: a pane that
			// would land on the pane of another row goes further out, past it.
			for try := 0; try < 8; try++ {
				hit := false
				for _, q := range placed {
					if px < q.right && px+w > q.left && y < q.bottom && y+ht > q.top {
						hit = true
						y = q.top // and stands level with it: one line of windows, not stairs
						if side > 0 {
							px = q.right + gap
						} else {
							px = q.left - gap - w
						}
					}
				}
				if !hit {
					break
				}
			}
			if px+w > work.right-dp(4) {
				px = work.right - dp(4) - w
			}
			if px < work.left+dp(4) {
				px = work.left + dp(4)
			}
			placed = append(placed, rect{px, y, px + w, y + ht})
			py, ph := y, ht
			if p.min {
				ph = paneHeaderH()
				py = y + ht - ph
			}
			p.side, p.last = side, i == len(row)-1
			pSetWindowPos.Call(p.hwnd, 0, uintptr(px), uintptr(py), uintptr(w), uintptr(ph), swpNoZOrder|swpNoActivate)
		}
	}
}

// paneSend sends what the field of a pane holds: a message, a reply, an edit or a file,
// whatever the field is for. It runs with the bank of the pane entered.
func paneSend(p *pane) {
	if be.current() == nil { // said in the pane itself, not in the panel of another conversation
		viewToast("Brak połączenia. Wiadomość nie została wysłana.")
		return
	}
	doSend()
}

// paneCovered gives the question or the large picture that covers the pane, 0 for none.
func paneCovered(p *pane) uintptr {
	switch {
	case dlg.hwnd != 0 && dlg.host == p.hwnd:
		return dlg.hwnd
	case vw.hwnd != 0 && vw.host == p.hwnd:
		return viewerInner()
	}
	return 0
}

func paneLayout(p *pane) {
	if curBank != p.bank {
		defer enterBank(p.bank)()
	}
	hh := paneHeaderH()
	p.closeR = rect{p.w - dp(10) - dp(36), (hh - dp(36)) / 2, p.w - dp(10), (hh + dp(36)) / 2}
	p.minR = rect{}
	if cfg.HeadMode == headModeDock {
		p.minR = rect{p.closeR.left - dp(4) - dp(36), p.closeR.top, p.closeR.left - dp(4), p.closeR.bottom}
	}
	next := p.closeR
	if p.minR.right > p.minR.left {
		next = p.minR
	}
	p.appR = rect{next.left - dp(4) - dp(36), next.top, next.left - dp(4), next.bottom}
	if cover := paneCovered(p); cover != 0 && !p.min {
		show(p.view, false)
		show(p.input, false)
		moveTo(cover, rect{0, 0, p.w, p.h})
		return
	}
	inH := int32(p.lines) * lay.editH
	fieldH := inH + dp(16)
	if fieldH < dp(36) {
		fieldH = dp(36)
	}
	p.compH = fieldH + dp(20)
	p.barR = rect{}
	if compose.mode != cmNone { // what the field is for, above it
		barH := sp(36)
		if composeThumb() != 0 {
			barH = composeThumbSide() + dp(8)
		}
		p.compH += barH + dp(6)
		p.barR = rect{dp(12), p.h - p.compH + dp(8), p.w - dp(12), p.h - p.compH + dp(8) + barH}
		compose.closeHit = rect{p.barR.right - dp(32), p.barR.top, p.barR.right, p.barR.bottom}
	}
	if !paneOpenBody(p) {
		show(p.view, false)
		show(p.input, false)
		return
	}
	show(p.view, true)
	show(p.input, true)
	moveTo(p.view, rect{0, hh + 1, p.w, p.h - p.compH - 1})
	p.attachR = rect{dp(10), p.h - dp(10) - dp(36), dp(46), p.h - dp(10)}
	p.sendR = rect{p.w - dp(12) - dp(36), p.h - dp(10) - dp(36), p.w - dp(12), p.h - dp(10)}
	p.fieldR = rect{dp(12) + dp(40), p.h - dp(10) - fieldH, p.w - dp(12) - dp(36) - dp(8), p.h - dp(10)}
	it := p.fieldR.top + (fieldH-inH)/2
	moveTo(p.input, rect{p.fieldR.left + dp(10), it, p.fieldR.right - dp(8), it + inH})
}

// paneOpenBody reports whether the pane shows its conversation, not only its header.
func paneOpenBody(p *pane) bool {
	return !p.min && p.h > paneHeaderH()+p.compH+dp(40) && paneCovered(p) == 0
}

func paneCanSend(p *pane) bool {
	return !p.empty || p.bank.compose.mode == cmFile
}

func panePaint(p *pane, target uintptr) {
	if curBank != p.bank {
		defer enterBank(p.bank)()
	}
	buf := newBuffer(target, p.w, p.h)
	c := canvas{buf.hdc}
	hh := paneHeaderH()
	body := paneOpenBody(p)
	c.fill(rect{0, 0, p.w, p.h}, pal.layer)
	c.fill(rect{0, 0, p.w, hh}, pal.card)
	c.fill(rect{0, hh, p.w, hh + 1}, pal.line)
	t := p.thread
	g := newGfx(buf.hdc)
	face := threadAvatar(g, t, dp(14), (hh-dp(36))/2, dp(36), pal.card)
	hotBack := func(r rect) {
		g.round(r.left, r.top, r.right-r.left, r.bottom-r.top, dp(4), dp(4), dp(4), dp(4), mix(pal.fg, pal.card, 9))
	}
	switch p.hot {
	case pbtClose:
		hotBack(p.closeR)
	case pbtMin:
		hotBack(p.minR)
	case pbtApp:
		hotBack(p.appR)
	}
	if body {
		c.fill(rect{0, p.h - p.compH, p.w, p.h}, pal.card)
		c.fill(rect{0, p.h - p.compH - 1, p.w, p.h - p.compH}, pal.line)
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
		switch {
		case !paneCanSend(p):
			fill = mix(pal.fg, pal.card, 8)
		case p.hot == pbtSend:
			fill = pal.accentHover
		}
		g.round(s.left, s.top, s.right-s.left, s.bottom-s.top, rr, rr, rr, rr, fill)
		if p.hot == pbtAttach {
			hotBack(p.attachR)
		}
		if compose.mode != cmNone {
			b := p.barR
			g.pill(b.left, b.top+dp(2), dp(3), b.bottom-b.top-dp(4), pal.accent)
			if img := composeThumb(); img != 0 {
				side := composeThumbSide()
				g.image(img, b.left+dp(12), (b.top+b.bottom-side)/2, side, side)
			}
		}
	}
	g.free()
	if !face {
		c.font(fnt.avSm)
		c.text(initials(threadTitle(t)), rect{dp(14), (hh - dp(36)) / 2, dp(50), (hh + dp(36)) / 2}, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	}
	right := p.closeR.left - dp(8)
	if p.minR.right > p.minR.left {
		right = p.minR.left - dp(8)
		c.font(fnt.head)
		c.text("–", p.minR, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	}
	c.font(fnt.icon)
	c.text(gClose, p.closeR, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	c.text(gOpenApp, p.appR, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	right = p.appR.left - dp(8)
	sub := ""
	switch {
	case isGroup(t):
		sub = "Rozmowa grupowa"
	case isE2EE(t):
		sub = "Rozmowa szyfrowana"
	}
	x := dp(60)
	c.font(fnt.head)
	if sub == "" || p.min {
		c.text(threadTitle(t), rect{x, 0, right, hh}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
	} else {
		top := (hh - sp(38)) / 2
		c.text(threadTitle(t), rect{x, top, right, top + sp(20)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
		c.font(fnt.small)
		c.text(sub, rect{x, top + sp(20), right, top + sp(38)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
	}
	if body {
		ink := pal.onAccent
		if !paneCanSend(p) {
			ink = pal.fg2
		}
		c.font(fnt.icon)
		c.text(gSend, p.sendR, ink, dtCenter|dtVCenter|dtSingleLine)
		ink = pal.fg2
		if p.hot == pbtAttach {
			ink = pal.fg
		}
		c.text(gAttach, p.attachR, ink, dtCenter|dtVCenter|dtSingleLine)
		if compose.mode != cmNone {
			b := p.barR
			title, excerpt := composeLabels()
			mid := (b.top + b.bottom) / 2
			tx := b.left + dp(12)
			if composeThumb() != 0 {
				tx += composeThumbSide() + dp(12)
			}
			c.font(fnt.smallSemi)
			c.text(title, rect{tx, mid - sp(17), b.right - dp(40), mid}, pal.accentText, dtSingleLine|dtVCenter|dtEndEllipsis)
			c.font(fnt.small)
			c.text(excerpt, rect{tx, mid, b.right - dp(40), mid + sp(17)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
			c.font(fnt.iconSm)
			ink = pal.fg2
			if p.hot == pbtBarClose {
				ink = pal.fg
			}
			c.text(gClose, compose.closeHit, ink, dtCenter|dtVCenter|dtSingleLine)
		}
	}
	buf.flush(0, 0)
}

// paneHit names the button of a pane under a point.
func paneHit(p *pane, x, y int32) int {
	in := func(r rect) bool { return r.right > r.left && x >= r.left && x < r.right && y >= r.top && y < r.bottom }
	switch {
	case in(p.closeR):
		return pbtClose
	case in(p.minR):
		return pbtMin
	case in(p.appR):
		return pbtApp
	case !paneOpenBody(p):
		return pbtNone
	case in(p.attachR):
		return pbtAttach
	case in(p.sendR):
		return pbtSend
	case p.bank.compose.mode != cmNone && in(p.bank.compose.closeHit):
		return pbtBarClose
	}
	return pbtNone
}

func paneProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	p := paneOf(hwnd)
	if p == nil || p.hwnd != hwnd {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
		return r
	}
	if curBank != p.bank {
		defer enterBank(p.bank)()
	}
	switch msg {
	case wmNcCalcSize:
		if wp != 0 {
			return 0 // the whole pane is its content; the frame stays only for its edge
		}
	case wmNcHitTest:
		// A pane belongs to its head and is moved by the head alone. Its size is another
		// matter: the edges away from the heads can be pulled.
		if p.min || paneCovered(p) != 0 {
			return htClient
		}
		return edgeHit(hwnd, lp, p.side, p.last, cfg.HeadMode == headModeDock)
	case wmGetMinMaxInfo:
		if !p.min {
			mm := (*minMaxInfo)(unsafe.Pointer(lp))
			mm.minTrack = point{dp(300), dp(360)}
		}
		return 0
	case wmExitSizeMove: // the size it was dragged to is the size of them all from now on
		panelSized(hwnd)
		panesPlace()
		return 0
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
			if cover := paneCovered(p); cover != 0 {
				pSetFocus.Call(cover)
			} else if !p.min {
				pSetFocus.Call(p.input)
				if t, ok := threadByID(p.id); ok && t.Unread() {
					be.openThread(t, true)
				}
			}
			headsSync(false)
		}
		return 0
	case wmMouseMove:
		x, y := lpPoint(lp)
		if hot := paneHit(p, x, y); hot != p.hot {
			p.hot = hot
			invalidate(hwnd)
			trackLeave(hwnd) // told when the pointer leaves, so the mark does not stay
		}
		return 0
	case wmMouseLeave:
		if p.hot != pbtNone {
			p.hot = pbtNone
			invalidate(hwnd)
		}
		return 0
	case wmLButtonUp:
		x, y := lpPoint(lp)
		switch hit := paneHit(p, x, y); {
		case hit == pbtClose:
			paneClose(p)
		case hit == pbtMin:
			p.min = !p.min
			panesPlace()
		case p.min && y < paneHeaderH(): // the folded pane opens again from its header
			paneFront(p)
		case hit == pbtApp:
			openInApp(p.id, getText(p.input))
		case hit == pbtNone && y < paneHeaderH(): // twice on the header: as tall as the screen, and back
			if headerClick(hwnd) {
				panesPlace()
			}
		case hit == pbtAttach:
			pickFile()
		case hit == pbtSend:
			paneSend(p)
		case hit == pbtBarClose:
			cancelCompose()
		case x >= p.fieldR.left && x < p.fieldR.right && y >= p.fieldR.top && y < p.fieldR.bottom:
			pSetFocus.Call(p.input)
		}
		return 0
	case wmCtlColorEdit:
		pSetTextColor.Call(wp, cref(pal.fg))
		pSetBkColor.Call(wp, cref(pal.card))
		return brCard
	case wmCommand:
		switch code := (wp >> 16) & 0xFFFF; code {
		case enChange:
			paneInputChanged(p)
		case enSetFocus, enKillFocus:
			invalidate(hwnd)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
