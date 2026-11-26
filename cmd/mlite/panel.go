//go:build windows

package main

// The panel on the right of a conversation: who it is with, the switches that belong to
// this one conversation (its head on the desktop, quiet), a search through its messages,
// and its pictures, files and links.

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"lazyfmess/internal/store"
)

const (
	gInfo    = ""
	gChevron = ""

	phClose = iota
	phBack
	phSearch
	phPin
	phQuiet
	phPhoto
	phEntry
	phResult
	phPhotoNewer // the row under the pictures that turns their pages
	phPhotoOlder
	phEntryNewer // and the one under the files and links
	phEntryOlder
	phPull       // fetch older history of the conversation from Messenger
	phPhotoFirst // to the newest page of pictures, and to the oldest
	phPhotoLast
	phEntryFirst
	phEntryLast
	phPhotoGoto // the words "page 4 of 12": a click lets the number be typed
	phEntryGoto

	idPanelPage = 361
	esNumber    = 0x2000
	esCenter    = 0x0001

	panelPhotoPage = 18 // pictures on a page
	panelEntryPage = 10 // files and links on a page
	panelPullPages = 25 // pages of older history fetched for one press

	idFind      = 333
	idInfo      = 334
	idClose     = 335
	timerFlash  = 23
	timerToast  = 24
	idPanelEdit = 360
)

type panelHit struct {
	r    rect // in window coordinates of the panel
	kind int
	arg  int
}

type panelEntry struct {
	icon, label, when string
	m                 store.Message
	link              string
}

var pn struct {
	hwnd, edit  uintptr
	open        bool
	search      bool // the panel shows the search rather than the sections
	side, cover bool // beside the conversation / over it
	w, h        int32
	scroll      int32
	total       int32
	sc          scroller
	hits        []panelHit
	hot         int
	focus       int  // row the keyboard is on, an index into hits
	kbd         bool // the keyboard was used last: the focus mark is shown
	thumbs      map[string]uintptr
	thumbSide   int32
	photos      []store.Message
	entries     []panelEntry
	photoPage   int // the page of pictures shown; 0 is the newest
	entryPage   int
	photoAll    int // how many there are in all, on this computer
	entryAll    int
	pageEdit    uintptr // the small field a page number is typed into
	pageOld     uintptr
	pageFor     int      // the section whose page is being typed: 0 none, 1 pictures, 2 files and links
	pull        struct { // older history being fetched from Messenger for the panel
		thread int64
		left   int
		note   string
	}
	query     string
	all       []store.Message // every message of the conversation kept on this computer
	results   []int           // indexes into all, newest first
	forThread int64
	host      uintptr   // the window the panel is in: the program's, or a pane beside a head
	bank      *viewBank // the view of the conversation it tells about
	forCount  int
}

var hFind, hInfo, hClose uintptr // the buttons at the right end of the conversation header

// drawHeaderBtn draws one of the two buttons at the right end of the conversation header.
func drawHeaderBtn(d *drawItem) {
	w, h := d.rcItem.right, d.rcItem.bottom
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.card)
	search := d.ctlID == idFind
	fill := pal.card
	if d.ctlID == idClose { // the panel of the overlay: only this one is there
		if d.itemState&odsSelected != 0 {
			fill = mix(pal.fg, pal.card, 14)
		} else if hotBtn == d.hwndItem {
			fill = mix(pal.signal, pal.card, 70) // closing is the red button of a title bar
		}
	}
	switch {
	case pn.open && pn.search == search && d.ctlID != idApp && d.ctlID != idClose: // the panel it opened is showing
		fill = mix(pal.accent, pal.card, 22)
	case d.itemState&odsSelected != 0:
		fill = mix(pal.fg, pal.card, 14)
	case hotBtn == d.hwndItem:
		fill = mix(pal.fg, pal.card, 8)
	}
	if fill != pal.card {
		g := newGfx(buf.hdc)
		g.round(0, 0, w, h, dp(8), dp(8), dp(8), dp(8), fill)
		g.free()
	}
	glyph := gInfo
	switch {
	case d.ctlID == idClose:
		glyph = gClose
	case d.ctlID == idApp:
		glyph = gOpenApp
	case search:
		glyph = gSearch
	}
	ink := pal.fg
	if d.ctlID == idClose && hotBtn == d.hwndItem {
		ink = 0xFFFFFF
	}
	c.font(fnt.icon)
	c.text(glyph, rect{0, 0, w, h}, ink, dtCenter|dtVCenter|dtSingleLine)
	if focusCue(d) {
		fr := rect{1, 1, w - 1, h - 1}
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(0, 0)
}

func panelWidth() int32 { return dp(304) }
func panelBar() int32   { return dp(52) }

func initPanel() {
	pn.hwnd = createWindow(0, "MlitePanel", "Informacje o rozmowie", wsChild|wsTabStop|wsClipChildren, hMain, 0)
	pn.edit = createWindow(0, "EDIT", "", wsChild|wsTabStop|esAutoHScroll, pn.hwnd, idPanelEdit)
	pn.pageEdit = createWindow(0, "EDIT", "", wsChild|esAutoHScroll|esNumber|esCenter, pn.hwnd, idPanelPage)
	pn.pageOld = subclass(pn.pageEdit, panelPageProc)
	pn.sc = scroller{hwnd: pn.hwnd, pos: &pn.scroll, total: func() int32 { return pn.total }, page: func() int32 { return pn.h }}
	pn.hot = -1
	pn.thumbs = map[string]uintptr{}
}

// quietThread reports whether the user asked this program not to notify about a conversation.
func quietThread(id int64) bool {
	for _, q := range cfg.Quiet {
		if q == id {
			return true
		}
	}
	return false
}

func setQuiet(id int64, on bool) {
	out := cfg.Quiet[:0]
	for _, q := range cfg.Quiet {
		if q != id {
			out = append(out, q)
		}
	}
	if on {
		out = append(out, id)
	}
	cfg.Quiet = out
	saveConfig()
}

// showPanel opens the panel (in search mode when asked), or closes it when it is already
// open in that mode.
func showPanel(search bool) {
	if _, ok := threadByID(hostThreadID()); !ok {
		return
	}
	here := pn.host == hostWnd() || (pn.host == 0 && hostWnd() == hMain)
	if pn.open && pn.search == search && here {
		closePanel()
		return
	}
	if pn.open && !here { // open in another window: there is one panel, and it comes here
		closePanel()
	}
	pn.host, pn.bank = hostWnd(), curBank
	if pn.host != hMain { // into the pane it was asked from, over its conversation
		pSetParent.Call(pn.hwnd, pn.host)
	}
	pn.open, pn.search, pn.scroll = true, search, 0
	pn.focus, pn.kbd = 0, false
	panelCollect()
	hostLayout()
	if search {
		pSetFocus.Call(pn.edit)
	} else {
		pSetFocus.Call(pn.hwnd)
	}
}

func closePanel() {
	if !pn.open {
		return
	}
	pn.open, pn.search = false, false
	pn.all, pn.results = nil, nil
	panelDropThumbs()
	b := pn.bank
	if b == nil {
		b = &mainBank
	}
	if b != curBank { // back in the conversation it was opened over
		defer enterBank(b)()
	}
	if pn.host != 0 && pn.host != hMain { // home again, before the pane it was in can take it along
		show(pn.hwnd, false)
		pSetParent.Call(pn.hwnd, hMain)
	}
	pn.host, pn.bank = hMain, nil
	hostLayout()
	pSetFocus.Call(hostInput())
}

var pSetParent = user32.NewProc("SetParent")

// panelThreadID is the conversation the panel tells about.
func panelThreadID() int64 {
	if pn.bank != nil && pn.bank.pane != nil {
		return pn.bank.pane.id
	}
	return selectedID
}

func panelDropThumbs() {
	for id, t := range pn.thumbs {
		if t != 0 {
			pGdipDisposeImage.Call(t)
		}
		delete(pn.thumbs, id)
	}
}

// panelCollect gathers what the sections list from the messages the view holds.
func panelCollect() {
	pn.photos, pn.entries = pn.photos[:0], pn.entries[:0]
	pn.all, _ = be.db.ListMessages(be.ctx, mv.thread.ID)
	if pn.forThread != mv.thread.ID { // another conversation: from the first page again
		pn.photoPage, pn.entryPage = 0, 0
		pn.pull.note = ""
	}
	pn.photoAll, pn.entryAll = 0, 0
	onPhotoPage := func() bool {
		return pn.photoAll > pn.photoPage*panelPhotoPage && pn.photoAll <= (pn.photoPage+1)*panelPhotoPage
	}
	onEntryPage := func() bool {
		return pn.entryAll > pn.entryPage*panelEntryPage && pn.entryAll <= (pn.entryPage+1)*panelEntryPage
	}
	pn.forThread, pn.forCount = mv.thread.ID, len(mv.msgs)
	for i := len(pn.all) - 1; i >= 0; i-- {
		m := pn.all[i]
		if m.Unsent {
			continue
		}
		switch {
		case showsInline(m):
			pn.photoAll++
			if onPhotoPage() {
				pn.photos = append(pn.photos, m)
			}
		case hasAttachment(m):
			pn.entryAll++
			if onEntryPage() {
				pn.entries = append(pn.entries, panelEntry{icon: attachmentIcon(m), label: mediaLabel(m), when: shortDate(m), m: m})
			}
		default:
			u := linkOf(m)
			if u != "" {
				pn.entryAll++
			}
			if u != "" && onEntryPage() {
				label := oneLine(m.LinkTitle, 80)
				if label == "" {
					label = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
				}
				pn.entries = append(pn.entries, panelEntry{icon: gLink, label: label, when: linkHost(u) + " · " + shortDate(m), m: m, link: u})
			}
		}
	}
	// a page past the end, after messages went away: back to the last one there is
	if last := panelPages(pn.photoAll, panelPhotoPage) - 1; pn.photoPage > last {
		pn.photoPage = last
		panelCollect()
		return
	}
	if last := panelPages(pn.entryAll, panelEntryPage) - 1; pn.entryPage > last {
		pn.entryPage = last
		panelCollect()
		return
	}
	panelSearch()
}

// panelGoto shows a page of a section (1 the pictures, 2 the files and links); a number
// past either end is brought to that end.
func panelGoto(section, page int) {
	if section == 1 {
		page = min(max(page, 0), panelPages(pn.photoAll, panelPhotoPage)-1)
		if page != pn.photoPage {
			pn.photoPage = page
			panelDropThumbs()
		}
	} else {
		pn.entryPage = min(max(page, 0), panelPages(pn.entryAll, panelEntryPage)-1)
	}
	panelCollect()
	invalidate(pn.hwnd)
}

// panelPageDone ends the typing of a page number: with go the page typed is shown.
func panelPageDone(goTo bool) {
	section := pn.pageFor
	if section == 0 {
		return
	}
	pn.pageFor = 0
	show(pn.pageEdit, false)
	if n, err := strconv.Atoi(strings.TrimSpace(getText(pn.pageEdit))); goTo && err == nil {
		panelGoto(section, n-1)
	}
	invalidate(pn.hwnd)
	pSetFocus.Call(pn.hwnd)
}

// panelPageProc lets Enter go to the page typed and Escape, or leaving the field, drop it.
func panelPageProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmKeyDown:
		switch wp {
		case 0x0D:
			panelPageDone(true)
			return 0
		case vkEscape:
			panelPageDone(false)
			return 0
		}
	case 0x0102: // WM_CHAR: Enter and Escape have been dealt with; no bell for them
		if wp == 0x0D || wp == 0x1B {
			return 0
		}
	case wmKillFocus:
		defer panelPageDone(false)
	}
	r, _, _ := pCallWindowProcW.Call(pn.pageOld, hwnd, uintptr(msg), wp, lp)
	return r
}

// panelPages is how many pages a section of n things has; one at least.
func panelPages(n, per int) int {
	if n <= per {
		return 1
	}
	return (n + per - 1) / per
}

// panelPull fetches older history of the panel's conversation from Messenger, a number
// of pages for one press, so that its older pictures, files and links can be listed.
func panelPull() {
	if pn.pull.left > 0 || older.busy != 0 {
		return
	}
	if be.current() == nil || av.offline {
		pn.pull.note = "Brak połączenia z Messengerem."
		return
	}
	pn.pull.thread, pn.pull.left, pn.pull.note = mv.thread.ID, panelPullPages, ""
	if be.fetchOlder(mv.thread) {
		older.busy = mv.thread.ID
	} else {
		pn.pull.left = 0
	}
}

// panelOlderArrived is a page of older history stored, or the end of it; it runs with the
// bank of the panel's conversation entered.
func panelOlderArrived(thread int64, res uintptr) {
	if pn.pull.left == 0 || pn.pull.thread != thread {
		return
	}
	switch res {
	case 1:
		pn.pull.left--
		if pn.pull.left > 0 && pn.open && mv.thread.ID == thread && be.fetchOlder(mv.thread) {
			older.busy = thread
		} else {
			pn.pull.left = 0
		}
	case 0:
		pn.pull.left, pn.pull.note = 0, "To już początek rozmowy."
	case 3:
		pn.pull.left, pn.pull.note = 0, "Starsze wiadomości są w kopii zapasowej, która czeka na PIN."
	default:
		pn.pull.left, pn.pull.note = 0, "Nie udało się pobrać starszych wiadomości. Spróbuj ponownie."
	}
	if pn.open && pn.forThread == thread {
		panelCollect()
		invalidate(pn.hwnd)
	}
}

func shortDate(m store.Message) string { return m.Timestamp.Format("2.01.2006") }

// panelSearch looks the query up in the messages of the conversation kept on this computer.
func panelSearch() {
	pn.results = pn.results[:0]
	q := strings.ToLower(strings.TrimSpace(pn.query))
	if len([]rune(q)) < 2 {
		return
	}
	for i := len(pn.all) - 1; i >= 0 && len(pn.results) < 200; i-- {
		if m := pn.all[i]; !m.Unsent && strings.Contains(strings.ToLower(m.Text), q) {
			pn.results = append(pn.results, i)
		}
	}
}

// panelSync keeps the panel in step with the conversation shown.
func panelSync() {
	if !pn.open {
		return
	}
	if mv.thread.ID != pn.forThread {
		panelDropThumbs()
		pn.scroll = 0
	}
	if mv.thread.ID != pn.forThread || len(mv.msgs) != pn.forCount {
		panelCollect()
	}
	invalidate(pn.hwnd)
}

// jumpToMessage scrolls the conversation to a message and marks it for a moment.
func jumpToMessage(id string) {
	for i := range mv.items {
		it := &mv.items[i]
		if it.kind == itMsg && it.m.ID == id {
			mv.sc.stop()
			mv.sc.set(it.y - mv.h/3)
			mv.flash = id
			pSetTimer.Call(mv.hwnd, timerFlash, 1600, 0)
			invalidate(mv.hwnd)
			return
		}
	}
}

func paintPanel(target uintptr) {
	buf := newBuffer(target, pn.w, pn.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, pn.w, pn.h}, pal.mica)
	t, ok := threadByID(panelThreadID())
	if !ok {
		buf.flush(0, 0)
		return
	}
	bar, pad := panelBar(), dp(12)
	cw := pn.w - 2*pad
	pn.hits = pn.hits[:0]
	hit := func(r rect, kind, arg int) int {
		pn.hits = append(pn.hits, panelHit{r, kind, arg})
		return len(pn.hits) - 1
	}
	type textOp struct {
		s     string
		r     rect
		col   uint32
		font  uintptr
		flags uint32
	}
	var texts []textOp
	text := func(font uintptr, s string, r rect, col uint32, flags uint32) {
		texts = append(texts, textOp{s, r, col, font, flags})
	}
	g := newGfx(buf.hdc)
	card := func(y, h int32, hot bool) {
		edge, fill := pal.line, pal.card
		if hot {
			fill = mix(pal.fg, pal.card, 6)
		}
		g.round(pad, y, cw, h, dp(8), dp(8), dp(8), dp(8), edge)
		g.round(pad+1, y+1, cw-2, h-2, dp(7), dp(7), dp(7), dp(7), fill)
	}
	// pager is the row that turns the pages of a section: to the newest and the newer on
	// its left, to the older and the oldest on its right, and between them which page of
	// how many, where a click lets the number be typed. section is 1 for the pictures and
	// 2 for the files and links.
	pagerH := dp(40)
	pager := func(y int32, section, page, pages int) {
		if y+pagerH < 0 || y > pn.h {
			if pn.pageFor == section { // the row has scrolled away with its field
				show(pn.pageEdit, false)
			}
			return
		}
		kinds := [][5]int{{}, {phPhotoFirst, phPhotoNewer, phPhotoOlder, phPhotoLast, phPhotoGoto}, {phEntryFirst, phEntryNewer, phEntryOlder, phEntryLast, phEntryGoto}}[section]
		card(y, pagerH, false)
		bw := dp(38)
		btn := func(x int32, kind int, sign string, on bool) {
			r := rect{x, y + dp(2), x + bw, y + pagerH - dp(2)}
			ink := mix(pal.fg2, pal.card, 40)
			if on {
				hi := hit(r, kind, 0)
				ink = pal.accentText
				if pn.hot == hi {
					g.round(r.left, r.top, bw, r.bottom-r.top, dp(6), dp(6), dp(6), dp(6), mix(pal.fg, pal.card, 9))
				}
			}
			text(fnt.head, sign, r, ink, dtCenter|dtSingleLine|dtVCenter)
		}
		btn(pad+dp(2), kinds[0], "«", page > 0)
		btn(pad+dp(2)+bw, kinds[1], "‹", page > 0)
		btn(pad+cw-dp(2)-2*bw, kinds[2], "›", page < pages-1)
		btn(pad+cw-dp(2)-bw, kinds[3], "»", page < pages-1)
		mid := rect{pad + dp(2) + 2*bw, y + dp(2), pad + cw - dp(2) - 2*bw, y + pagerH - dp(2)}
		if pn.pageFor == section { // the number is being typed: "Strona [ 4 ] z 12"
			ew, eh := dp(46), lay.editH
			ex := (mid.left+mid.right)/2 - ew/2
			g.round(ex-dp(4), y+dp(6), ew+dp(8), pagerH-dp(12), dp(6), dp(6), dp(6), dp(6), pal.accent)
			g.round(ex-dp(4)+2, y+dp(6)+2, ew+dp(8)-4, pagerH-dp(12)-4, dp(4), dp(4), dp(4), dp(4), pal.card)
			moveTo(pn.pageEdit, rect{ex, y + (pagerH-eh)/2, ex + ew, y + (pagerH-eh)/2 + eh})
			show(pn.pageEdit, true)
			text(fnt.small, "Strona", rect{mid.left, y, ex - dp(10), y + pagerH}, pal.fg2, dtRight|dtSingleLine|dtVCenter)
			text(fnt.small, fmt.Sprintf("z %d", pages), rect{ex + ew + dp(10), y, mid.right, y + pagerH}, pal.fg2, dtSingleLine|dtVCenter)
			return
		}
		hi := hit(mid, kinds[4], 0)
		if pn.hot == hi {
			g.round(mid.left, mid.top, mid.right-mid.left, mid.bottom-mid.top, dp(6), dp(6), dp(6), dp(6), mix(pal.fg, pal.card, 6))
		}
		text(fnt.small, fmt.Sprintf("Strona %d z %d", page+1, pages), mid, pal.fg2, dtCenter|dtSingleLine|dtVCenter)
	}
	toggle := func(y, h int32, on bool) {
		tx, ty := pad+cw-dp(14)-dp(40), y+(h-dp(20))/2
		if on {
			g.pill(tx, ty, dp(40), dp(20), pal.accent)
			g.ellipse(tx+dp(24), ty+dp(4), dp(12), dp(12), pal.onAccent)
		} else {
			g.pill(tx, ty, dp(40), dp(20), pal.fg2)
			g.pill(tx+1, ty+1, dp(40)-2, dp(20)-2, pal.card)
			g.ellipse(tx+dp(4), ty+dp(4), dp(12), dp(12), pal.fg2)
		}
	}
	y := bar - pn.scroll

	if pn.search {
		// the field
		fh := lay.editH + dp(12)
		fy := y + dp(4)
		g.round(pad, fy, cw, fh, dp(8), dp(8), dp(8), dp(8), pal.accent)
		g.round(pad+2, fy+2, cw-4, fh-4, dp(6), dp(6), dp(6), dp(6), pal.card)
		text(fnt.icon, gSearch, rect{pad + dp(8), fy, pad + dp(30), fy + fh}, pal.fg2, dtSingleLine|dtVCenter)
		moveTo(pn.edit, rect{pad + dp(32), fy + (fh-lay.editH)/2, pad + cw - dp(8), fy + (fh-lay.editH)/2 + lay.editH})
		y = fy + fh + dp(10)
		note := ""
		switch {
		case len([]rune(strings.TrimSpace(pn.query))) < 2:
			note = "Wpisz co najmniej dwa znaki. Szukanie obejmuje wiadomości tej rozmowy pobrane na ten komputer."
		case len(pn.results) == 0:
			note = "Nic nie znaleziono w wiadomościach pobranych na ten komputer."
		default:
			note = fmt.Sprintf("Znalezione wiadomości: %d", len(pn.results))
		}
		nh := sp(18)
		if len(pn.results) == 0 {
			nh = sp(56)
		}
		text(fnt.small, note, rect{pad + dp(4), y, pad + cw - dp(4), y + nh}, pal.fg2, dtWordBreak)
		y += nh + dp(6)
		rh := dp(16) + sp(18) + 2*fnt.lineH
		for k, idx := range pn.results {
			if y+rh >= 0 && y < pn.h {
				m := pn.all[idx]
				hi := hit(rect{pad, y, pad + cw, y + rh}, phResult, k)
				card(y, rh, pn.hot == hi)
				who := senderName(m)
				if m.SenderID == mv.self && mv.self != 0 {
					who = "Ty"
				}
				text(fnt.smallSemi, who, rect{pad + dp(12), y + dp(8), pad + cw - dp(96), y + dp(8) + sp(18)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
				text(fnt.small, m.Timestamp.Format("2.01.2006 15:04"), rect{pad + cw - dp(100), y + dp(8), pad + cw - dp(12), y + dp(8) + sp(18)}, pal.fg2, dtSingleLine|dtVCenter|dtRight)
				text(fnt.prev, snippet(emojify(m.Text), pn.query), rect{pad + dp(12), y + dp(8) + sp(18), pad + cw - dp(12), y + rh - dp(8)}, pal.fg2, dtWordBreak|dtEndEllipsis)
			}
			y += rh + dp(4)
		}
	} else {
		show(pn.edit, false)
		// who
		d := dp(72)
		ax := (pn.w - d) / 2
		y += dp(16)
		photo := threadAvatar(g, t, ax, y, d, pal.mica)
		if !photo {
			text(fnt.title, initials(threadTitle(t)), rect{ax, y, ax + d, y + d}, pal.fg, dtCenter|dtVCenter|dtSingleLine)
		}
		y += d + dp(10)
		text(fnt.head, threadTitle(t), rect{pad, y, pad + cw, y + sp(24)}, pal.fg, dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
		y += sp(24)
		sub := ""
		switch {
		case isGroup(t):
			sub = "Rozmowa grupowa"
		case isE2EE(t):
			sub = "Rozmowa szyfrowana"
		}
		if real := realTitle(t); real != threadTitle(t) {
			if sub == "" {
				sub = real
			} else {
				sub = real + " · " + sub
			}
		}
		if sub != "" {
			text(fnt.small, sub, rect{pad, y, pad + cw, y + sp(18)}, pal.fg2, dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
			y += sp(18)
		}
		y += dp(16)
		// search
		rh := dp(44)
		hi := hit(rect{pad, y, pad + cw, y + rh}, phSearch, 0)
		card(y, rh, pn.hot == hi)
		text(fnt.icon, gSearch, rect{pad + dp(12), y, pad + dp(34), y + rh}, pal.fg2, dtSingleLine|dtVCenter)
		text(fnt.body, "Szukaj w rozmowie", rect{pad + dp(40), y, pad + cw - dp(36), y + rh}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
		text(fnt.iconSm, gChevron, rect{pad + cw - dp(32), y, pad + cw - dp(10), y + rh}, pal.fg2, dtSingleLine|dtVCenter|dtCenter)
		y += rh + dp(4)
		// switches
		for _, sw := range []struct {
			kind        int
			title, desc string
			on          bool
		}{
			{phPin, "Główka na pulpicie", "Kółko z tą rozmową zawsze pod ręką", isPinned(t.ID)},
			{phQuiet, "Wycisz w Linnet", "Bez powiadomień o tej rozmowie na tym komputerze", quietThread(t.ID)},
		} {
			c.font(fnt.small)
			_, dh := c.measure(sw.desc, cw-dp(76))
			rh := dp(20) + sp(20) + dh
			hi := hit(rect{pad, y, pad + cw, y + rh}, sw.kind, 0)
			card(y, rh, pn.hot == hi)
			toggle(y, rh, sw.on)
			text(fnt.body, sw.title, rect{pad + dp(12), y + dp(10), pad + cw - dp(64), y + dp(10) + sp(20)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
			text(fnt.small, sw.desc, rect{pad + dp(12), y + dp(10) + sp(20), pad + cw - dp(64), y + rh - dp(8)}, pal.fg2, dtWordBreak)
			y += rh + dp(4)
		}
		// pictures
		if len(pn.photos) > 0 {
			y += dp(14)
			text(fnt.name, "Zdjęcia", rect{pad + dp(2), y, pad + cw, y + sp(22)}, pal.fg, dtSingleLine|dtVCenter)
			y += sp(22) + dp(6)
			gap := dp(4)
			side := (cw - 2*gap) / 3
			if side != pn.thumbSide {
				panelDropThumbs()
				pn.thumbSide = side
			}
			for k, m := range pn.photos {
				px, py := pad+int32(k%3)*(side+gap), y+int32(k/3)*(side+gap)
				if py+side < 0 || py > pn.h {
					continue
				}
				hi := hit(rect{px, py, px + side, py + side}, phPhoto, k)
				th, made := pn.thumbs[m.ID]
				if !made {
					if p, ok := mediaFile(m); ok {
						shown, _, _ := drawable(p)
						th = thumbOf(shown, side, side, dp(6), true)
						pn.thumbs[m.ID] = th
					}
				}
				if pn.hot == hi { // a frame in the accent colour around the picture under the pointer
					g.round(px-dp(2), py-dp(2), side+dp(4), side+dp(4), dp(7), dp(7), dp(7), dp(7), pal.accent)
				}
				if th != 0 {
					g.image(th, px, py, side, side)
				} else {
					g.round(px, py, side, side, dp(6), dp(6), dp(6), dp(6), mix(pal.fg, pal.mica, 9))
				}
			}
			rows := int32((len(pn.photos) + 2) / 3)
			y += rows*(side+gap) - gap
			if pages := panelPages(pn.photoAll, panelPhotoPage); pages > 1 {
				y += dp(8)
				pager(y, 1, pn.photoPage, pages)
				y += pagerH
			}
		}
		// files and links
		if len(pn.entries) > 0 {
			y += dp(18)
			text(fnt.name, "Pliki i linki", rect{pad + dp(2), y, pad + cw, y + sp(22)}, pal.fg, dtSingleLine|dtVCenter)
			y += sp(22) + dp(6)
			rh := dp(16) + sp(20) + sp(17)
			for k, e := range pn.entries {
				if y+rh >= 0 && y < pn.h {
					hi := hit(rect{pad, y, pad + cw, y + rh}, phEntry, k)
					card(y, rh, pn.hot == hi)
					text(fnt.icon, e.icon, rect{pad + dp(12), y, pad + dp(34), y + rh}, pal.accentText, dtSingleLine|dtVCenter)
					text(fnt.body, e.label, rect{pad + dp(42), y + dp(8), pad + cw - dp(12), y + dp(8) + sp(20)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
					text(fnt.small, e.when, rect{pad + dp(42), y + dp(8) + sp(20), pad + cw - dp(12), y + rh - dp(8)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
				}
				y += rh + dp(4)
			}
			if pages := panelPages(pn.entryAll, panelEntryPage); pages > 1 {
				y += dp(4)
				pager(y, 2, pn.entryPage, pages)
				y += pagerH
			}
		}
		// how far back what is listed goes, and the way to reach further
		if len(pn.all) > 0 {
			y += dp(18)
			rh := dp(16) + sp(20) + sp(34)
			title, desc := "Doczytaj starszą historię", "Na tym komputerze rozmowa jest od "+shortDate(pn.all[0])+". Starsze zdjęcia, pliki i linki pojawią się po pobraniu wiadomości z Messengera."
			switch {
			case pn.pull.left > 0 && pn.pull.thread == mv.thread.ID:
				title = fmt.Sprintf("Doczytuję starszą historię… (%d z %d)", panelPullPages-pn.pull.left+1, panelPullPages)
			case older.done[mv.thread.ID]:
				title, desc = "To już cała rozmowa", "Na tym komputerze jest wszystko od "+shortDate(pn.all[0])+"."
			case pn.pull.note != "":
				desc = pn.pull.note
			}
			if y+rh >= 0 && y < pn.h {
				hi := hit(rect{pad, y, pad + cw, y + rh}, phPull, 0)
				card(y, rh, pn.hot == hi && !older.done[mv.thread.ID])
				text(fnt.bodySemi, title, rect{pad + dp(12), y + dp(8), pad + cw - dp(12), y + dp(8) + sp(20)}, pal.accentText, dtSingleLine|dtVCenter|dtEndEllipsis)
				text(fnt.small, desc, rect{pad + dp(12), y + dp(8) + sp(20), pad + cw - dp(12), y + rh - dp(6)}, pal.fg2, dtWordBreak)
			}
			y += rh
		}
		if len(pn.photos) == 0 && len(pn.entries) == 0 {
			y += dp(18)
			text(fnt.small, "W wiadomościach pobranych na ten komputer nie ma zdjęć, plików ani linków.", rect{pad + dp(4), y, pad + cw - dp(4), y + sp(40)}, pal.fg2, dtWordBreak)
			y += sp(40)
		}
	}
	pn.total = y + pn.scroll + dp(20)
	if most := max(pn.total-pn.h, 0); pn.scroll > most {
		// a shorter page, or a section gone: what is left is brought back into view
		pn.scroll = most
		invalidate(pn.hwnd)
	}

	// the bar on top stays in place
	g.round(0, 0, pn.w, bar, 0, 0, 0, 0, pal.mica)
	closeR := rect{pn.w - dp(8) - dp(36), (bar - dp(36)) / 2, pn.w - dp(8), (bar + dp(36)) / 2}
	backR := rect{dp(6), (bar - dp(36)) / 2, dp(6) + dp(36), (bar + dp(36)) / 2}
	// hits of the bar go last and win over what scrolled under it
	ci := hit(closeR, phClose, 0)
	if pn.hot == ci {
		g.round(closeR.left, closeR.top, dp(36), dp(36), dp(4), dp(4), dp(4), dp(4), mix(pal.fg, pal.mica, 10))
	}
	titleX := dp(16)
	bi := -1
	if pn.search {
		bi = hit(backR, phBack, 0)
		if pn.hot == bi {
			g.round(backR.left, backR.top, dp(36), dp(36), dp(4), dp(4), dp(4), dp(4), mix(pal.fg, pal.mica, 10))
		}
		titleX = backR.right + dp(6)
	}
	g.round(0, 0, 1, pn.h, 0, 0, 0, 0, pal.line)
	pn.sc.paint(g, pn.w, pn.h, pal.mica)
	g.free()

	for _, op := range texts {
		if op.r.bottom <= bar || op.r.top >= pn.h {
			continue // under the bar or below the window
		}
		c.font(op.font)
		c.text(op.s, op.r, op.col, op.flags)
	}
	// the bar's own ground again over text that scrolled into it, then its labels
	c.fill(rect{1, 0, pn.w, bar}, pal.mica)
	title := "Informacje"
	if pn.search {
		title = "Szukaj w rozmowie"
		c.font(fnt.icon)
		c.text(gBack, backR, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	}
	if pn.hot == ci || (bi >= 0 && pn.hot == bi) {
		hg := newGfx(buf.hdc)
		r := closeR
		if pn.hot == bi {
			r = backR
		}
		hg.round(r.left, r.top, dp(36), dp(36), dp(4), dp(4), dp(4), dp(4), mix(pal.fg, pal.mica, 10))
		hg.free()
		if pn.hot == bi {
			c.font(fnt.icon)
			c.text(gBack, backR, pal.fg, dtCenter|dtVCenter|dtSingleLine)
		}
	}
	c.font(fnt.name)
	c.text(title, rect{titleX, 0, closeR.left - dp(8), bar}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
	c.font(fnt.iconSm)
	c.text(gClose, closeR, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	if pn.kbd && pn.focus >= 0 && pn.focus < len(pn.hits) {
		fr := pn.hits[pn.focus].r
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(0, 0)
}

// snippet returns the part of a text around the first match, on one line.
func snippet(text, query string) string {
	flat := oneLine(text, 4000)
	i := strings.Index(strings.ToLower(flat), strings.ToLower(strings.TrimSpace(query)))
	if i > 60 {
		start := i - 40
		for start > 0 && flat[start]&0xC0 == 0x80 { // not inside a letter
			start--
		}
		flat = "…" + flat[start:]
	}
	return oneLine(flat, 220)
}

func panelHitAt(x, y int32) int {
	for i := len(pn.hits) - 1; i >= 0; i-- {
		h := pn.hits[i]
		if x >= h.r.left && x < h.r.right && y >= h.r.top && y < h.r.bottom {
			if h.kind != phClose && h.kind != phBack && y < panelBar() {
				continue // scrolled under the bar
			}
			return i
		}
	}
	return -1
}

func panelClick(i int) {
	if i < 0 || i >= len(pn.hits) {
		return
	}
	h := pn.hits[i]
	t, ok := threadByID(panelThreadID())
	if !ok {
		return
	}
	switch h.kind {
	case phClose:
		closePanel()
	case phBack:
		pn.search, pn.scroll = false, 0
		hostLayout()
		pSetFocus.Call(pn.hwnd)
	case phSearch:
		pn.search, pn.scroll = true, 0
		hostLayout()
		pSetFocus.Call(pn.edit)
	case phPin:
		if isPinned(t.ID) {
			unpinHead(t.ID)
		} else {
			pinHead(t.ID)
		}
		headsSync(false)
		invalidate(pn.hwnd)
	case phQuiet:
		setQuiet(t.ID, !quietThread(t.ID))
		invalidate(pn.hwnd)
		invalidate(hList)
	case phPhoto:
		if h.arg < len(pn.photos) {
			openViewer(pn.photos[h.arg])
		}
	case phPhotoNewer, phPhotoOlder, phPhotoFirst, phPhotoLast: // another page takes the place of the one shown
		last := panelPages(pn.photoAll, panelPhotoPage) - 1
		panelGoto(1, map[int]int{phPhotoNewer: pn.photoPage - 1, phPhotoOlder: pn.photoPage + 1, phPhotoFirst: 0, phPhotoLast: last}[h.kind])
	case phEntryNewer, phEntryOlder, phEntryFirst, phEntryLast:
		last := panelPages(pn.entryAll, panelEntryPage) - 1
		panelGoto(2, map[int]int{phEntryNewer: pn.entryPage - 1, phEntryOlder: pn.entryPage + 1, phEntryFirst: 0, phEntryLast: last}[h.kind])
	case phPhotoGoto, phEntryGoto: // the number of the page is typed into a small field
		page := pn.photoPage
		pn.pageFor = 1
		if h.kind == phEntryGoto {
			pn.pageFor, page = 2, pn.entryPage
		}
		setText(pn.pageEdit, strconv.Itoa(page+1))
		invalidate(pn.hwnd)
		pUpdateWindow.Call(pn.hwnd) // drawn now, so that the field is in its place before it is used
		pSetFocus.Call(pn.pageEdit)
		sendMsg(pn.pageEdit, emSetSel, 0, ^uintptr(0))
	case phPull:
		if !older.done[t.ID] {
			panelPull()
			invalidate(pn.hwnd)
		}
	case phEntry:
		if h.arg < len(pn.entries) {
			if e := pn.entries[h.arg]; e.link != "" {
				openURL(e.link)
			} else {
				openAttachment(e.m)
			}
		}
	case phResult:
		if h.arg < len(pn.results) {
			idx := pn.results[h.arg]
			id := pn.all[idx].ID
			if need := len(pn.all) - idx + 20; need > mv.limit { // older than what is loaded: load back to it
				mv.limit = need
				loadView(true)
			}
			if pn.cover || pn.host != hMain {
				closePanel()
			}
			jumpToMessage(id)
		}
	}
}

func panelProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if pn.bank != nil && pn.bank != curBank {
		defer enterBank(pn.bank)()
	}
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if pn.open {
			paintPanel(hdc)
		}
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSize:
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		pn.w, pn.h = rc.right, rc.bottom
		pn.sc.set(pn.scroll)
		invalidate(hwnd)
		return 0
	case wmCtlColorEdit:
		pSetTextColor.Call(wp, cref(pal.fg))
		pSetBkColor.Call(wp, cref(pal.card))
		return brCard
	case wmCommand:
		if lp == pn.edit && wp>>16 == 0x0300 { // the text of the field changed
			pn.query = getText(pn.edit)
			pn.scroll = 0
			panelSearch()
			invalidate(hwnd)
		}
		return 0
	case wmMouseWheel:
		pn.sc.wheel(int32(int16(wp >> 16)))
		return 0
	case wmMouseMove:
		x, y := lpPoint(lp)
		if pn.sc.mouse(msg, x, y, pn.w, pn.h) {
			return 0
		}
		if hot := panelHitAt(x, y); hot != pn.hot {
			pn.hot = hot
			invalidate(hwnd)
		}
		return 0
	case wmMouseLeave:
		pn.sc.leave()
		if pn.hot != -1 {
			pn.hot = -1
			invalidate(hwnd)
		}
		return 0
	case wmSetCursor:
		if lp&0xFFFF == 1 && wp == hwnd && pn.hot >= 0 {
			cur, _, _ := pLoadCursorW.Call(0, idcHand)
			pSetCursor.Call(cur)
			return 1
		}
	case wmLButtonDown:
		x, y := lpPoint(lp)
		pn.kbd = false
		if pn.sc.mouse(msg, x, y, pn.w, pn.h) {
			return 0
		}
		if !pn.search {
			pSetFocus.Call(hwnd)
		}
		return 0
	case wmLButtonUp:
		x, y := lpPoint(lp)
		if pn.sc.mouse(msg, x, y, pn.w, pn.h) {
			return 0
		}
		panelClick(panelHitAt(x, y))
		return 0
	case wmGetDlgCode:
		return 0x0001
	case wmKeyDown:
		switch wp {
		case vkEscape:
			closePanel()
		case vkUp, vkDown:
			// the arrows walk the rows that are on screen; past the last one they scroll
			step := 1
			if wp == vkUp {
				step = -1
			}
			next := pn.focus + step
			if !pn.kbd {
				next = 0
			}
			pn.kbd = true
			if next >= 0 && next < len(pn.hits) {
				pn.focus = next
				invalidate(hwnd)
			} else {
				pn.sc.by(int32(step) * dp(80))
			}
		case vkSpace, vkReturn:
			if pn.kbd {
				panelClick(pn.focus)
				pn.kbd = pn.open
			}
		case vkPrior:
			pn.sc.by(-pn.h * 9 / 10)
		case vkNext:
			pn.sc.by(pn.h * 9 / 10)
		}
		return 0
	case wmTimer:
		if pn.sc.timer(wp) {
			return 0
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
