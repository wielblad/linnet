//go:build windows

package main

// Chat heads: a pinned conversation shown as a small circle that stays on top of other
// windows, with the unread count and, when allowed, a short look at the newest message.

import (
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"time"
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

// headBadge is the red of the unread count on a head; white text on it reads at 5:1.
const headBadge = 0xD92D20

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
	Group int   `json:"group,omitempty"` // the group the head is tied into, 0 for none
	ID    int64 `json:"id"`
	X     int32 `json:"x"`
	Y     int32 `json:"y"`
}

type head struct {
	target, under, hidden, shift bool // would be tied to; behind the program's window; inside a folded group; Shift was down
	id                           int64
	hwnd, peek                   uintptr
	key                          uint64
	unread                       int
	down, dragging               bool
	startX, startY               int32 // pointer when the button went down
	originX, originY             int32 // window position at that moment
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

func headSize() int32 { return dp(headPx()) }

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
	groupsLayout()
	saveConfig()
	headsSync(false)
	headsApply()
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

// glow lays a soft round light on the ground: rings of barely visible colour, so that the
// edge melts away instead of cutting a disc out of the background.
func (g gfx) glow(cx, cy, r int32, argb uint32) {
	const steps = 16
	a, rgb := (argb>>24)&0xFF, argb&0xFFFFFF
	step := a / steps
	if step == 0 {
		step = 1
	}
	for i := int32(steps); i >= 1; i-- {
		rr := r * i / steps
		g.ellipseA(cx-rr, cy-rr, 2*rr, 2*rr, step<<24|rgb)
	}
}

func (g gfx) ellipseA(x, y, w, h int32, argb uint32) {
	var br uintptr
	pGdipCreateSolidFill.Call(uintptr(argb), uintptr(unsafe.Pointer(&br)))
	pGdipFillEllipseA.Call(uintptr(g), br, iarg(x), iarg(y), iarg(w), iarg(h))
	pGdipDeleteBrush.Call(br)
}

// paintHead draws one head: soft shadow, picture or initials, unread badge, and in the
// lower corner what the other person is doing: writing, or having read what was sent.
func paintHead(g gfx, id int64, title string, unread, mark, phase int, stack, target bool) {
	s := headSize()
	d := hs(52)
	ax, ay := (s-d)/2, s-d-hs(4)
	if stack { // a folded group: the heads behind this one show at its edge
		g.ellipseA(ax+hs(5), ay-hs(3), d, d, 0xFF303E58)
		g.ellipseA(ax+hs(3), ay-hs(1), d, d, 0xFF46587A)
	}
	if target { // the dragged head would be tied to this one
		g.ellipseA(ax-hs(3), ay-hs(3), d+2*hs(3), d+2*hs(3), 0xFF76ADFF)
	}
	g.ellipseA(ax-1, ay+dp(1), d+2, d+dp(3), 0x30000000)
	g.ellipseA(ax, ay+dp(1), d, d+dp(1), 0x40000000)
	if !g.avatar(id, ax, ay, d) {
		g.label(initials(title), ax, ay, d, d, hs(19), true, pal.fg)
	}
	if unread > 0 {
		text := countLabel(unread)
		bh, px := hs(22), hs(13)
		if bh < dp(18) { // the count stays readable on a small head
			bh, px = dp(18), dp(11)
		}
		bw := bh + int32(len(text)-1)*bh*7/22
		bx := s - bw - dp(1)
		g.pill(bx, dp(2), bw, bh, headBadge)
		g.label(text, bx, dp(1), bw, bh, px, true, 0xFFFFFF)
	}
	paintHeadMark(g, mark, phase)
}

// What a head says about the other person, in its lower right corner.
const (
	markNone = iota
	markTyping
	markSeen

	headTyping = 0x0866E8 // the accent: something is happening now
	headSeen   = 0x5F6A79 // a quiet grey: a state, not a call for attention
)

// paintHeadMark draws that mark: three dots that take turns while the other person is
// writing, or a tick once they have read the last message. The two differ in shape as well
// as in colour, so that nothing hangs on telling blue from grey.
func paintHeadMark(g gfx, mark, phase int) {
	if mark == markNone {
		return
	}
	s, d := headSize(), hs(22)
	if d < dp(18) {
		d = dp(18)
	}
	x, y := s-d-dp(1), s-d-dp(1)
	cx, cy := x+d/2, y+d/2
	if mark == markTyping {
		g.ellipse(x, y, d, d, headTyping)
		r := d * 2 / 22
		if r < 1 {
			r = 1
		}
		for k := int32(0); k < 3; k++ {
			alpha := uint32(0x66)
			if int32(phase%3) == k {
				alpha = 0xFF
			}
			g.ellipseA(cx+(k-1)*d*5/22-r, cy-r, 2*r, 2*r, alpha<<24|0xFFFFFF)
		}
		return
	}
	g.ellipse(x, y, d, d, headSeen)
	// the tick: two strokes with round ends
	u := func(v int32) int32 { return v * d / 22 }
	a, b, c := point{cx - u(5), cy}, point{cx - u(2), cy + u(3)}, point{cx + u(5), cy - u(4)}
	w := u(1)
	if w < 1 {
		w = 1
	}
	stroke := func(p, q point) {
		dx, dy := float64(q.x-p.x), float64(q.y-p.y)
		n := math.Hypot(dx, dy)
		if n == 0 {
			return
		}
		nx, ny := int32(math.Round(-dy/n*float64(w))), int32(math.Round(dx/n*float64(w)))
		g.poly([]point{{p.x + nx, p.y + ny}, {q.x + nx, q.y + ny}, {q.x - nx, q.y - ny}, {p.x - nx, p.y - ny}}, 0xFFFFFF)
	}
	stroke(a, b)
	stroke(b, c)
	for _, p := range []point{a, b, c} {
		g.ellipse(p.x-w, p.y-w, 2*w, 2*w, 0xFFFFFF)
	}
}

// ---- what the other person is doing

const (
	wmAppTyping   = wmApp + 14 // someone started or stopped writing: wp 1 or 0, lp the conversation
	timerHeadDots = 7
)

var (
	typingUntil = map[int64]time.Time{} // conversations where the other person is writing now
	headPhase   int                     // which of the three dots is lit
)

// typingArrived notes that someone is writing, or has stopped. Messenger repeats the news
// every few seconds while it lasts, so it is believed only for a little while.
func typingArrived(id int64, active bool) {
	if active {
		typingUntil[id] = time.Now().Add(8 * time.Second)
	} else {
		delete(typingUntil, id)
	}
	headsSync(false)
	headDotsTimer()
}

// headDotsTimer keeps the dots turning only while a head really shows them.
func headDotsTimer() {
	for id := range heads {
		if headMark(id) == markTyping {
			pSetTimer.Call(hMain, timerHeadDots, 350, 0)
			return
		}
	}
	pKillTimer.Call(hMain, timerHeadDots)
}

// headMark is what the head of a conversation shows in its corner. A head whose panel is
// open shows nothing: the conversation itself says it all there.
func headMark(id int64) int {
	switch os.Getenv("MLITE_HEADMARK") { // for looking at the marks only
	case "typing":
		return markTyping
	case "seen":
		return markSeen
	}
	if vis, _, _ := pIsWindowVisible.Call(hMain); compact.on && vis != 0 && (compact.forID == id || (tabsMode() && tabIndex(id) >= 0)) {
		return markNone
	}
	if p := paneFor(id); p != nil && !p.min { // open in a pane of its own
		return markNone
	}
	if until, ok := typingUntil[id]; ok {
		if time.Now().Before(until) {
			return markTyping
		}
		delete(typingUntil, id)
	}
	if headSeenBy(id) {
		return markSeen
	}
	return markNone
}

// headSeenBy reports whether the last message of the conversation is one's own and the
// other person has read it.
func headSeenBy(id int64) bool {
	self := be.accountID()
	if self == 0 {
		return false
	}
	msgs, _, err := be.db.RecentMessages(be.ctx, id, 1)
	if err != nil || len(msgs) == 0 {
		return false
	}
	last := msgs[len(msgs)-1]
	if last.SenderID != self || last.Pending || last.Error != "" {
		return false
	}
	marks, err := be.db.ReadMarks(be.ctx, id, self)
	if err != nil {
		return false
	}
	for _, at := range marks {
		if !at.Before(last.Timestamp) {
			return true
		}
	}
	return false
}

// headsSync creates, removes and redraws the heads so they match the pinned list and the unread counts.
// announce allows the short look at a new message.
func headsSync(announce bool) {
	groupsLayout() // the heads of a group follow its first one, whatever was read from the settings
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
		hidden, stack := headHidden(hc.ID), false
		if g := groupOf(hc.ID); g != nil && g.Fold && !hidden { // the one head a folded group shows counts for all
			stack, unread = true, 0
			for _, m := range groupMembers(g.ID) {
				unread += unreadBy[m]
			}
		}
		if hidden != h.hidden {
			h.hidden, h.key = hidden, 0
			if hidden {
				pShowWindow.Call(h.hwnd, swHide)
			}
		}
		mark, phase := headMark(hc.ID), 0
		if mark == markTyping {
			phase = headPhase
		}
		sum := fnv.New64a()
		fmt.Fprintf(sum, "%s|%d|%v|%06x|%d|%v|%d|%d|%v|%v|%d", title, unread, avatarImage(hc.ID, hs(52)) != 0, pal.accent, dpi, pal.dark, mark, phase, stack, h.target, headPx())
		if key := sum.Sum64(); key != h.key && !hidden {
			h.key = key
			target := h.target
			layer(h.hwnd, headSize(), headSize(), func(g gfx) { paintHead(g, hc.ID, title, unread, mark, phase, stack, target) })
			pShowWindow.Call(h.hwnd, swShowNA)
		}
		if hidden { // its own count, so that a new message of its own is noticed
			unread = unreadBy[hc.ID]
		}
		if announce && known && unread > h.unread && !h.under && cfg.HeadPreview && cfg.Body && !cfg.DND && !quietThread(h.id) && !conversationShown(h.id) {
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
	// The bubble takes clicks: one on it opens the conversation beside its head.
	ex := uint32(wsExLayered | wsExTopmost | wsExToolWindow | wsExNoActivate)
	h.peek = createWindow(ex, "MlitePeek", "Nowa wiadomość", wsPopup, 0, 0)
	if h.peek == 0 {
		return
	}
	pSetLayeredAttrs.Call(h.peek, 0, 255, 2) // opaque; layered only so that clicks can pass through
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(ht+1), uintptr(dp(24)), uintptr(dp(24)))
	pSetWindowRgn.Call(h.peek, rgn, 0)
	peeks[h.peek] = p
	var r, work rect
	anchor := h.hwnd
	if g := groupOf(h.id); g != nil && headHidden(h.id) { // inside a folded group: beside the head that shows
		if first := heads[groupMembers(g.ID)[0]]; first != nil {
			anchor = first.hwnd
		}
	}
	pGetWindowRect.Call(anchor, uintptr(unsafe.Pointer(&r)))
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
	side    int32 // which side of the head the panel took when it opened: -1 left, 1 right
}

// panelPlace is where the panel of the overlay belongs beside a head: beside the whole
// group the head is in, on the side that has room, and level with the head itself.
func panelPlace(h *head) (x, y, w, ht int32) {
	var hr rect
	if hc := headCfgOf(h.id); hc != nil {
		hr = rect{hc.X, hc.Y, hc.X + headSize(), hc.Y + headSize()}
	} else {
		pGetWindowRect.Call(h.hwnd, uintptr(unsafe.Pointer(&hr)))
	}
	er, ok := entityRect(h.id)
	if !ok {
		er = hr
	}
	if headHidden(h.id) {
		hr = er
	}
	w, ht = panelSize()
	work := workArea()
	// The side is settled when the panel opens and kept while it is open: a panel that
	// jumped from one side of the head to the other mid-drag looked like a fault.
	if compact.side == 0 {
		compact.side = -1
		if (er.left+er.right)/2 < (work.left+work.right)/2 {
			compact.side = 1
		}
	}
	place := func(side int32) int32 {
		if side > 0 {
			return er.right + dp(10)
		}
		return er.left - w - dp(10)
	}
	x = place(compact.side)
	if x+w > work.right-dp(4) || x < work.left+dp(4) { // no room on that side any more
		if other := place(-compact.side); other+w <= work.right-dp(4) && other >= work.left+dp(4) {
			x = other
		}
	}
	if x+w > work.right-dp(4) {
		x = work.right - dp(4) - w
	}
	if x < work.left+dp(4) {
		x = work.left + dp(4)
	}
	y = hr.top - dp(120)
	if y+ht > work.bottom-dp(8) {
		y = work.bottom - dp(8) - ht
	}
	if y < work.top+dp(8) {
		y = work.top + dp(8)
	}
	return x, y, w, ht
}

// windowToRestore is the place the ordinary window comes back to when the panel is left.
// A window that has never been seen at its own size — or one that is somehow already the
// size of a panel — would come back unusably small, so it is given a place of its own.
func windowToRestore() rect {
	var r, work rect
	pGetWindowRect.Call(hMain, uintptr(unsafe.Pointer(&r)))
	if r.right-r.left >= dp(640) && r.bottom-r.top >= dp(420) {
		return r
	}
	pSystemParametersInfo.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0)
	w, h := dp(1020), dp(720)
	if w > work.right-work.left-dp(80) {
		w = work.right - work.left - dp(80)
	}
	if h > work.bottom-work.top-dp(80) {
		h = work.bottom - work.top - dp(80)
	}
	x, y := work.left+(work.right-work.left-w)/2, work.top+(work.bottom-work.top-h)/2
	return rect{x, y, x + w, y + h}
}

// panelFollow keeps the open panel beside its head while the head is dragged.
func panelFollow(h *head) {
	if !compact.on || compact.forID != h.id {
		return
	}
	if vis, _, _ := pIsWindowVisible.Call(hMain); vis == 0 {
		return
	}
	x, y, w, ht := panelPlace(h)
	pSetWindowPos.Call(hMain, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), swpNoZOrder|swpNoActivate)
}

// openHead shows the conversation in a small window beside the head; a second click puts it away.
func openHead(h *head) {
	t, ok := threadByID(h.id)
	if !ok {
		showMain()
		return
	}
	hidePeek(h)
	vis, _, _ := pIsWindowVisible.Call(hMain)
	if multiMode() {
		// Kolumny and Dok: every conversation is a window of its own, in the row of the
		// group its head is in. The head of an open one puts it away, a folded one opens again.
		if p := paneFor(h.id); p != nil {
			if p.min {
				paneFront(p)
				headsSync(false)
			} else {
				paneClose(p)
			}
			return
		}
		paneOpen(h.id)
		return
	}
	if tabsMode() && compact.on && vis != 0 {
		// The panel is up: the head adds its conversation as a tab, or goes to it. The panel
		// stays where it is; a click on the head of the conversation being read puts it away.
		if selectedID == h.id {
			hideOverlay()
			return
		}
		tabAdd(h.id)
		tabSwitch(h.id)
		headsSync(false)
		return
	}
	if compact.on && compact.forID == h.id && vis != 0 {
		hideOverlay()
		return
	}
	if tabsMode() {
		tabAdd(h.id)
	}
	if !compact.on {
		compact.restore = windowToRestore()
		compact.on = true
		setOverlay(true)
	}
	compact.forID, compact.side = h.id, 0 // the side is settled afresh each time it opens
	x, y, w, ht := panelPlace(h)
	if r, _, _ := pIsIconic.Call(hMain); r != 0 {
		pShowWindow.Call(hMain, swRestore)
	}
	pSetWindowPos.Call(hMain, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), 0x0004|0x0040)
	pSetForegroundWindow.Call(hMain)
	refresh()
	if tabsMode() {
		tabSwitch(h.id)
		return
	}
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
		h.down, h.dragging, h.shift = true, false, keyDown(vkShift)
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
			if h.shift && groupOf(h.id) != nil { // Shift takes this one head out of its group
				groupLeave(h.id)
				headsRedraw()
			}
			if hc := headCfgOf(h.id); hc != nil {
				h.originX, h.originY = hc.X, hc.Y
			}
		}
		if h.dragging {
			// The head moves with its group, and what stands beside them moves along.
			entityMove(h.id, h.originX+dx, h.originY+dy)
			to := int64(0)
			if groupOf(h.id) == nil {
				to = nearestHead(h.id)
			}
			changed := false
			for id, o := range heads {
				if t := id == to; o.target != t {
					o.target, changed = t, true
				}
			}
			headsApply()
			if changed {
				headsSync(false)
			}
		}
		return 0
	case wmLButtonUp:
		if !h.down {
			return 0
		}
		h.down = false
		pReleaseCapture.Call()
		switch g := groupOf(h.id); {
		case h.dragging:
			headDragged(h)
		case g != nil && g.Fold: // a folded group opens out
			g.Fold = false
			groupsLayout()
			entitySettle(h.id)
			saveConfig()
			headsRedraw()
			headsApply()
			headsZ()
		default:
			openHead(h)
		}
		return 0
	case wmRButtonUp:
		headMenu(h)
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
		if p := peeks[hwnd]; p != nil && lp&0xFFFF == 1 { // all of it can be pressed
			cur, _, _ := pLoadCursorW.Call(0, idcHand)
			pSetCursor.Call(cur)
			return 1
		}
	case wmLButtonUp:
		if p := peeks[hwnd]; p != nil {
			x, y := lpPoint(lp)
			if r := p.linkRect(); p.link != "" && x >= r.left && x < r.right && y >= r.top && y < r.bottom {
				peekOpen(hwnd)
				return 0
			}
			for _, h := range heads { // anywhere else: the conversation opens beside its head
				if h.peek != hwnd {
					continue
				}
				hidePeek(h)
				if g := groupOf(h.id); g != nil && g.Fold {
					g.Fold = false
					groupsLayout()
					saveConfig()
					headsRedraw()
					headsApply()
				}
				openHeadFor(h)
				break
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
		paintHead(g, hc.ID, threadTitle(t), unreadBy[hc.ID], headMark(hc.ID), 0, false, false)
		g.free()
		name := filepath.Join(dir, fmt.Sprintf("head-%d.png", hc.ID))
		pGdipSaveImageToFile.Call(bmp, uintptr(unsafe.Pointer(u16(name))), uintptr(unsafe.Pointer(&png)), 0)
		pGdipDisposeImage.Call(bmp)
	}
}
