//go:build windows

package main

// Heads tied into groups, and what follows from heads being more than single dots: their
// size, the place the windows of conversations take beside them, the size of those windows,
// the way from such a window to the ordinary window of the program, and the order of heads
// and that window on the screen.
//
// A group is a stack or a strip of heads that moves as one. A head dropped beside another is
// tied to it; Shift while dragging, or the menu, takes one out. The first head of a group
// says where the group is, and the others are laid out after it. A group, like a head on its
// own, has its own row of conversations beside it.

import (
	"time"
	"unsafe"
)

type groupCfg struct {
	ID   int    `json:"id"`
	Dir  string `json:"dir"` // "v" a stack, "h" a strip
	Fold bool   `json:"fold,omitempty"`
}

const (
	idHeadLeave = 434
	idHeadFold  = 435
	idHeadDir   = 436
	idApp       = 336

	gOpenApp = "" // a window with an arrow: to the ordinary window of the program

	htLeft, htRight, htTop, htTopLeft, htTopRight, htBottom, htBottomLeft, htBottomRight = 10, 11, 12, 13, 14, 15, 16, 17
	wmExitSizeMove                                                                       = 0x0232
	wmWindowPosChanged                                                                   = 0x0047
)

var (
	hApp     uintptr // the button of the panel that leads to the ordinary window
	paneBig  bool    // the windows of conversations are as tall as the screen for now
	rowSide  = map[int64]int32{}
	lastHdr  time.Time // the last click on the header of a window of a conversation
	lastHdrW uintptr
)

// ---- sizes

func headPx() int {
	if cfg.HeadSize == 48 || cfg.HeadSize == 80 {
		return cfg.HeadSize
	}
	return 64
}

// hs scales a measure drawn for a head of 64 to the size the heads have.
func hs(v int) int32 { return dp(v) * int32(headPx()) / 64 }

func headStep() int32 { return headSize() + dp(6) }

var headSizes = []struct {
	px    int
	label string
}{{48, "Mały"}, {64, "Średni"}, {80, "Duży"}}

func headSizeLabel() string {
	for _, s := range headSizes {
		if s.px == headPx() {
			return s.label + " ▾"
		}
	}
	return "Średni ▾"
}

func pickHeadSize() {
	labels, current := []string{}, 1
	for i, s := range headSizes {
		labels = append(labels, s.label)
		if s.px == headPx() {
			current = i
		}
	}
	i := settingsPick(labels, current)
	if i < 0 || headSizes[i].px == headPx() {
		return
	}
	cfg.HeadSize = headSizes[i].px
	groupsLayout()
	for _, hc := range cfg.Heads {
		entitySettle(hc.ID)
	}
	saveConfig()
	headsRedraw()
	headsApply()
	invalidate(sv.hwnd)
}

// paneLimit is how many conversations Kolumny and Dok keep open at once.
func paneLimit() int {
	if cfg.HeadPanes == 4 {
		return 4
	}
	return 3
}

func headPanesLabel() string {
	if paneLimit() == 4 {
		return "4 ▾"
	}
	return "3 ▾"
}

func pickHeadPanes() {
	i := settingsPick([]string{"3", "4"}, paneLimit()-3)
	if i < 0 || i+3 == paneLimit() {
		return
	}
	cfg.HeadPanes = i + 3
	saveConfig()
	for len(panes) > paneLimit() {
		paneClose(paneOldest())
	}
	invalidate(sv.hwnd)
}

var pMonitorFromRect = user32.NewProc("MonitorFromRect")

// workAt is the room for windows on the screen a place is on: the monitor that holds most
// of it, or the nearest one, without its taskbar.
func workAt(r rect) rect {
	mi := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	mon, _, _ := pMonitorFromRect.Call(uintptr(unsafe.Pointer(&r)), 2) // the nearest one
	if ok, _, _ := pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); mon == 0 || ok == 0 || mi.work.right <= mi.work.left {
		return workArea()
	}
	return mi.work
}

// workArea is the room for windows on the main screen.
func workArea() rect {
	var work rect
	pSystemParametersInfo.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0)
	return work
}

// undp turns pixels back into the measure dp was given, so a size survives a change of scale.
func undp(v int32) int32 { return v * 96 / dpi }

// panelSize is the size of a window of a conversation beside a head: the one it was dragged
// to, or the size it has always had. The dock keeps a size of its own.
func panelSize(work rect) (w, ht int32) {
	w, ht = dp(380), dp(580)
	sw, sh := cfg.PanelW, cfg.PanelH
	if cfg.HeadMode == headModeDock {
		w, ht = dp(340), dp(470)
		sw, sh = cfg.DockW, cfg.DockH
	}
	if sw > 0 {
		w = sw * dpi / 96
	}
	if sh > 0 {
		ht = sh * dpi / 96
	}
	if paneBig {
		ht = work.bottom - work.top
	}
	if w < dp(300) {
		w = dp(300)
	}
	if ht < dp(360) {
		ht = dp(360)
	}
	if max := work.right - work.left - dp(8); w > max {
		w = max
	}
	if max := work.bottom - work.top - dp(16); ht > max {
		ht = max
	}
	return w, ht
}

// panelSized keeps the size a window of a conversation was dragged to.
func panelSized(hwnd uintptr) {
	var r rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	w, ht := undp(r.right-r.left), undp(r.bottom-r.top)
	if cfg.HeadMode == headModeDock {
		cfg.DockW, cfg.DockH = w, ht
	} else {
		cfg.PanelW, cfg.PanelH = w, ht
	}
	paneBig = false
	saveConfig()
}

// headerClick notes a click on the header of a window of a conversation; the second one
// soon after switches between the size that is kept and the height of the screen.
func headerClick(hwnd uintptr) bool {
	twice := lastHdrW == hwnd && time.Since(lastHdr) < 400*time.Millisecond
	lastHdr, lastHdrW = time.Now(), hwnd
	if !twice {
		return false
	}
	lastHdrW = 0
	paneBig = !paneBig
	return true
}

// edgeHit says which edge of a window of a conversation a point of the screen is on. Only
// the edges away from the head take part: the one beside the head stays where it is. side is
// where the window stands, 1 to the right of its head; top says the window grows upwards.
func edgeHit(hwnd uintptr, lp uintptr, side int32, outer, top bool) uintptr {
	var r rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	x, y := int32(int16(lp)), int32(int16(lp>>16))
	b := dp(6)
	onSide := outer && ((side > 0 && x >= r.right-b) || (side < 0 && x < r.left+b))
	onEnd := (top && y < r.top+b) || (!top && y >= r.bottom-b)
	switch {
	case onSide && onEnd && top && side > 0:
		return htTopRight
	case onSide && onEnd && top:
		return htTopLeft
	case onSide && onEnd && side > 0:
		return htBottomRight
	case onSide && onEnd:
		return htBottomLeft
	case onSide && side > 0:
		return htRight
	case onSide:
		return htLeft
	case onEnd && top:
		return htTop
	case onEnd:
		return htBottom
	}
	return htClient
}

// mainSide says on which side of its heads the panel of the overlay stands.
func mainSide() int32 {
	var r rect
	pGetWindowRect.Call(hMain, uintptr(unsafe.Pointer(&r)))
	if er, ok := entityRect(compact.forID); ok && (r.left+r.right)/2 < (er.left+er.right)/2 {
		return -1
	}
	return 1
}

// ---- groups

func headCfgOf(id int64) *headCfg {
	for i := range cfg.Heads {
		if cfg.Heads[i].ID == id {
			return &cfg.Heads[i]
		}
	}
	return nil
}

func groupByID(gid int) *groupCfg {
	for i := range cfg.Groups {
		if cfg.Groups[i].ID == gid && gid != 0 {
			return &cfg.Groups[i]
		}
	}
	return nil
}

func groupOf(id int64) *groupCfg {
	if hc := headCfgOf(id); hc != nil {
		return groupByID(hc.Group)
	}
	return nil
}

// groupMembers lists the heads of a group in their order, which is their order in the settings.
func groupMembers(gid int) []int64 {
	var out []int64
	for _, hc := range cfg.Heads {
		if hc.Group == gid && gid != 0 {
			out = append(out, hc.ID)
		}
	}
	return out
}

// groupsTidy lets go of groups that are no longer groups: one head, or none.
func groupsTidy() {
	kept := cfg.Groups[:0]
	for _, g := range cfg.Groups {
		m := groupMembers(g.ID)
		if len(m) >= 2 {
			kept = append(kept, g)
			continue
		}
		for _, id := range m {
			headCfgOf(id).Group = 0
		}
	}
	cfg.Groups = kept
	for i := range cfg.Heads { // a head that names a group that is not there
		if cfg.Heads[i].Group != 0 && groupByID(cfg.Heads[i].Group) == nil {
			cfg.Heads[i].Group = 0
		}
	}
}

// groupsLayout puts the heads of every group in their places after its first head.
func groupsLayout() {
	groupsTidy()
	for _, g := range cfg.Groups {
		m := groupMembers(g.ID)
		first := *headCfgOf(m[0])
		for i, id := range m {
			hc := headCfgOf(id)
			hc.X, hc.Y = first.X, first.Y
			if g.Dir == "h" {
				hc.X += int32(i) * headStep()
			} else {
				hc.Y += int32(i) * headStep()
			}
		}
	}
}

// headHidden reports whether a head is put away inside a folded group.
func headHidden(id int64) bool {
	g := groupOf(id)
	return g != nil && g.Fold && groupMembers(g.ID)[0] != id
}

// entityKey names what a head moves with: its group, or itself.
func entityKey(id int64) int64 {
	if g := groupOf(id); g != nil {
		return -int64(g.ID)
	}
	return id
}

// entityRect is the place a head takes together with its group.
func entityRect(id int64) (rect, bool) {
	hc := headCfgOf(id)
	if hc == nil {
		return rect{}, false
	}
	s := headSize()
	g := groupOf(id)
	if g == nil {
		return rect{hc.X, hc.Y, hc.X + s, hc.Y + s}, true
	}
	m := groupMembers(g.ID)
	f := headCfgOf(m[0])
	n := int32(len(m))
	if g.Fold {
		n = 1
	}
	r := rect{f.X, f.Y, f.X + s, f.Y + s}
	if g.Dir == "h" {
		r.right += (n - 1) * headStep()
	} else {
		r.bottom += (n - 1) * headStep()
	}
	return r, true
}

func entityMembers(id int64) []int64 {
	if g := groupOf(id); g != nil {
		return groupMembers(g.ID)
	}
	return []int64{id}
}

// entityMove takes a head to a new place, and its group with it.
func entityMove(id int64, x, y int32) {
	hc := headCfgOf(id)
	if hc == nil {
		return
	}
	dx, dy := x-hc.X, y-hc.Y
	for _, m := range entityMembers(id) {
		c := headCfgOf(m)
		c.X += dx
		c.Y += dy
	}
}

// entitySettle keeps a head or a group on the screen, and takes it to the left or right
// edge of the screen when it was left close to one.
func entitySettle(id int64) {
	r, ok := entityRect(id)
	if !ok {
		return
	}
	work := workAt(r) // the screen the head is on
	var dx, dy int32
	switch {
	case r.left-work.left < dp(30):
		dx = work.left + dp(8) - r.left
	case work.right-r.right < dp(30):
		dx = work.right - dp(8) - r.right
	}
	if r.top < work.top+dp(4) {
		dy = work.top + dp(4) - r.top
	}
	if r.bottom > work.bottom-dp(4) {
		dy = work.bottom - dp(4) - r.bottom
	}
	if dx == 0 && dy == 0 {
		return
	}
	hc := headCfgOf(entityMembers(id)[0])
	entityMove(hc.ID, hc.X+dx, hc.Y+dy)
}

// groupBump takes a head to the front of its group, which stays where it is: the
// conversation someone has just written in comes first.
func groupBump(id int64) bool {
	g := groupOf(id)
	if g == nil {
		return false
	}
	first := groupMembers(g.ID)[0]
	if first == id {
		return false
	}
	origin := *headCfgOf(first)
	headsReorder(id, first, false)
	hc := headCfgOf(id)
	hc.X, hc.Y = origin.X, origin.Y
	groupsLayout()
	return true
}

// headsReorder moves a head in the settings to just before or after another.
func headsReorder(id, anchor int64, after bool) {
	var moved headCfg
	rest := make([]headCfg, 0, len(cfg.Heads))
	for _, hc := range cfg.Heads {
		if hc.ID == id {
			moved = hc
		} else {
			rest = append(rest, hc)
		}
	}
	out := make([]headCfg, 0, len(cfg.Heads))
	for _, hc := range rest {
		if hc.ID == anchor && !after {
			out = append(out, moved)
		}
		out = append(out, hc)
		if hc.ID == anchor && after {
			out = append(out, moved)
		}
	}
	cfg.Heads = out
}

// groupJoin ties a head to another: to its group, or into a new one. Dropped above or
// below, the two make a stack; dropped beside, a strip. The head it was dropped at stays
// where it is.
func groupJoin(id, to int64) {
	h, o := headCfgOf(id), headCfgOf(to)
	if h == nil || o == nil || id == to {
		return
	}
	dx, dy := h.X-o.X, h.Y-o.Y
	abs := func(v int32) int32 {
		if v < 0 {
			return -v
		}
		return v
	}
	g := groupOf(to)
	dir := "h"
	if g != nil {
		dir = g.Dir
	} else if abs(dy) >= abs(dx) {
		dir = "v"
	}
	after := dx > 0
	if dir == "v" {
		after = dy > 0
	}
	gid := 0
	if g != nil {
		gid = g.ID
	} else {
		for _, x := range cfg.Groups {
			if x.ID > gid {
				gid = x.ID
			}
		}
		gid++
		cfg.Groups = append(cfg.Groups, groupCfg{ID: gid, Dir: dir})
		o.Group = gid
	}
	first := groupMembers(gid)[0]
	h.Group = gid
	if !after && to == first { // the new head leads: it takes the place before the old first one
		h.X, h.Y = o.X, o.Y
		if dir == "h" {
			h.X -= headStep()
		} else {
			h.Y -= headStep()
		}
	}
	headsReorder(id, to, after)
	groupsLayout()
}

// groupLeave takes a head out of its group and puts it beside the group.
func groupLeave(id int64) {
	hc := headCfgOf(id)
	if hc == nil || hc.Group == 0 {
		return
	}
	hc.Group = 0
	groupsLayout()
}

// nearestHead finds the head a dragged head would be tied to if it were dropped now.
func nearestHead(id int64) int64 {
	h := headCfgOf(id)
	if h == nil {
		return 0
	}
	s := int64(headSize())
	best, bestD := int64(0), int64(1)<<60
	for _, o := range cfg.Heads {
		if o.ID == id || headHidden(o.ID) {
			continue
		}
		dx, dy := int64(h.X-o.X), int64(h.Y-o.Y)
		d := dx*dx + dy*dy
		if lim := s + s/2; d < lim*lim && d < bestD { // closer than half a head apart
			best, bestD = o.ID, d
		}
	}
	return best
}

// headsApply moves the windows of the heads to where the settings say they are, and what
// stands beside them with them, in one go so that nothing lags behind.
func headsApply() {
	batch, _, _ := pBeginDeferWindowPos.Call(uintptr(len(cfg.Heads) + 1))
	move := func(hwnd uintptr, x, y, w, ht int32, flags uintptr) {
		if batch != 0 {
			batch, _, _ = pDeferWindowPos.Call(batch, hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), flags)
		}
		if batch == 0 { // no batch, or it broke: one by one
			pSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), flags)
		}
	}
	for _, hc := range cfg.Heads {
		if h := heads[hc.ID]; h != nil {
			move(h.hwnd, hc.X, hc.Y, 0, 0, swpQuiet)
		}
	}
	if vis, _, _ := pIsWindowVisible.Call(hMain); compact.on && vis != 0 {
		if h := heads[compact.forID]; h != nil {
			x, y, w, ht := panelPlace(h)
			move(hMain, x, y, w, ht, swpNoZOrder|swpNoActivate)
		}
	}
	if batch != 0 {
		pEndDeferWindowPos.Call(batch)
	}
	panesPlace()
}

// headDragged is the end of a drag: tie the head to the one it was dropped at, keep it on
// the screen, and remember where everything is.
func headDragged(h *head) {
	if groupOf(h.id) == nil {
		if to := nearestHead(h.id); to != 0 {
			groupJoin(h.id, to)
		}
	}
	entitySettle(h.id)
	for _, o := range heads {
		o.target = false
	}
	headsApply()
	saveConfig()
	headsSync(false)
	headsZ()
}

// headMenu is the menu of a head.
func headMenu(h *head) {
	menu, _, _ := pCreatePopupMenu.Call()
	add := func(id uintptr, label string) {
		pAppendMenuW.Call(menu, mfString, id, uintptr(unsafe.Pointer(u16(label))))
	}
	add(idHeadOpen, "Otwórz rozmowę")
	g := groupOf(h.id)
	if g != nil {
		pAppendMenuW.Call(menu, mfSeparator, 0, 0)
		add(idHeadLeave, "Odepnij od grupy")
		if g.Fold {
			add(idHeadFold, "Rozwiń grupę")
		} else {
			add(idHeadFold, "Zwiń grupę")
		}
		if g.Dir == "h" {
			add(idHeadDir, "Ułóż w stos")
		} else {
			add(idHeadDir, "Ułóż w pasek")
		}
	}
	pAppendMenuW.Call(menu, mfSeparator, 0, 0)
	add(idHeadUnpin, "Odepnij główkę")
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(hMain)
	cmd, _, _ := pTrackPopupMenu.Call(menu, tpmRightBtn|tpmReturnCmd|tpmNoNotify, uintptr(pt.x), uintptr(pt.y), 0, hMain, 0)
	pDestroyMenu.Call(menu)
	id := h.id
	switch cmd {
	case idHeadOpen:
		if g != nil && g.Fold {
			g.Fold = false
		}
		openHeadFor(h)
	case idHeadUnpin:
		unpinHead(id)
		return
	case idHeadLeave:
		hc := headCfgOf(id)
		groupLeave(id)
		if hc = headCfgOf(id); hc != nil { // beside the group it left, not on top of it
			hc.X += headStep() + dp(8)
		}
	case idHeadFold:
		g.Fold = !g.Fold
	case idHeadDir:
		if g.Dir == "h" {
			g.Dir = "v"
		} else {
			g.Dir = "h"
		}
	default:
		return
	}
	groupsLayout()
	entitySettle(id)
	saveConfig()
	headsRedraw()
	headsApply()
	headsZ()
}

// ---- the heads and the ordinary window of the program

var (
	pGetTopWindow          = user32.NewProc("GetTopWindow")
	pGetWindow             = user32.NewProc("GetWindow")
	pGetWindowThreadProcID = user32.NewProc("GetWindowThreadProcessId")
	pDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
)

const timerHeadsZ = 8

// mainShowsAt reports whether the ordinary window of the program is what the screen shows
// at a point: no window of another program lies over it there.
func mainShowsAt(pt point) bool {
	var own uint32
	pGetWindowThreadProcID.Call(hMain, uintptr(unsafe.Pointer(&own)))
	w, _, _ := pGetTopWindow.Call(0)
	for i := 0; w != 0 && w != hMain && i < 2000; i++ {
		var pid uint32
		pGetWindowThreadProcID.Call(w, uintptr(unsafe.Pointer(&pid)))
		if vis, _, _ := pIsWindowVisible.Call(w); vis != 0 && pid != own {
			var r rect
			var cloaked uint32
			pGetWindowRect.Call(w, uintptr(unsafe.Pointer(&r)))
			pDwmGetWindowAttribute.Call(w, 14, uintptr(unsafe.Pointer(&cloaked)), 4) // DWMWA_CLOAKED: there, but not shown
			ex := windowLong(w, gwlExStyle)
			if min, _, _ := pIsIconic.Call(w); min == 0 && cloaked == 0 && ex&wsExTransparent == 0 &&
				pt.x >= r.left && pt.x < r.right && pt.y >= r.top && pt.y < r.bottom {
				return false
			}
		}
		w, _, _ = pGetWindow.Call(w, 2) // GW_HWNDNEXT: the one below
	}
	return w == hMain
}

// headsZ keeps the heads off the ordinary window of the program: the window comes first,
// and a head that stands where the window shows goes under it. Everywhere else, and
// whenever the window is put away or covered by another program, the heads stay on top.
// Other programs come and go over the window without a word to this one, so while the
// window is up the question is asked again every second.
func headsZ() {
	var mr rect
	normal := false
	if vis, _, _ := pIsWindowVisible.Call(hMain); vis != 0 && !compact.on {
		if min, _, _ := pIsIconic.Call(hMain); min == 0 {
			normal = true
			pGetWindowRect.Call(hMain, uintptr(unsafe.Pointer(&mr)))
		}
	}
	if normal && len(heads) > 0 {
		pSetTimer.Call(hMain, timerHeadsZ, 1000, 0)
	} else {
		pKillTimer.Call(hMain, timerHeadsZ)
	}
	const flags = swpNoMove | swpNoSize | swpNoActivate
	for _, h := range heads {
		var r rect
		pGetWindowRect.Call(h.hwnd, uintptr(unsafe.Pointer(&r)))
		under := normal && r.left < mr.right && r.right > mr.left && r.top < mr.bottom && r.bottom > mr.top &&
			mainShowsAt(point{(r.left + r.right) / 2, (r.top + r.bottom) / 2})
		if !under && !h.under {
			continue
		}
		h.under = under
		if under {
			hidePeek(h)
			pSetWindowPos.Call(h.hwnd, hwndNoTop, 0, 0, 0, 0, flags)
			pSetWindowPos.Call(h.hwnd, hMain, 0, 0, 0, 0, flags) // right behind the window
		} else {
			pSetWindowPos.Call(h.hwnd, hwndTopmost, 0, 0, 0, 0, flags)
		}
	}
}

// headOpenNow reports whether the conversation of a head is open beside it.
func headOpenNow(id int64) bool {
	if p := paneFor(id); p != nil {
		return !p.min
	}
	if vis, _, _ := pIsWindowVisible.Call(hMain); !compact.on || vis == 0 {
		return false
	}
	if tabsMode() {
		return selectedID == id
	}
	return compact.forID == id
}

// openHeadFor opens the conversation of a head, or brings it forward when it is open: for
// the bubble with a new message, which must never put a conversation away.
func openHeadFor(h *head) {
	if !headOpenNow(h.id) {
		openHead(h)
		return
	}
	if p := paneFor(h.id); p != nil {
		paneFront(p)
		return
	}
	pSetForegroundWindow.Call(hMain)
}

// openInApp leaves the windows beside the heads for the ordinary window of the program,
// with the conversation it was asked from open there and what was being typed taken along.
func openInApp(id int64, draft string) {
	if inPane() {
		defer enterBank(&mainBank)()
	}
	t, ok := threadByID(id)
	panesCloseAll()
	headTabs.ids, headTabs.drafts = nil, nil
	showMain()
	if ok {
		if nav == navSettings {
			setNav(navChats)
		}
		selectThread(t, true)
		if draft != "" {
			setText(hInput, draft)
			inputChanged()
		}
	}
	layout()
	invalidate(hMain)
	headsSync(false)
	headsZ()
}
