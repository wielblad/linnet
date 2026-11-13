//go:build windows

package main

// How conversations opened from heads are shown is a choice in the settings. "Jedna naraz"
// is one conversation in the panel, replaced by the next head clicked. "Zakładki" keeps the
// opened conversations as tabs along the top of the same panel: one frame, one place, and
// what was being typed stays with its conversation.

const (
	headModeOne  = ""     // one conversation at a time
	headModeTabs = "tabs" // tabs in one panel

	maxHeadTabs = 5
)

var headModes = []struct{ id, label string }{
	{headModeOne, "Jedna naraz"},
	{headModeTabs, "Zakładki"},
}

var headTabs struct {
	ids    []int64
	drafts map[int64]string // what was typed and not sent, by conversation
	hit    []rect           // where each tab was drawn
	cross  []rect           // and its cross
}

func headModeLabel() string {
	for _, m := range headModes {
		if m.id == cfg.HeadMode {
			return m.label + " ▾"
		}
	}
	return headModes[0].label + " ▾"
}

// pickHeadMode lists the ways of showing conversations beside the heads.
func pickHeadMode() {
	labels, current := []string{}, 0
	for i, m := range headModes {
		labels = append(labels, m.label)
		if m.id == cfg.HeadMode {
			current = i
		}
	}
	i := settingsPick(labels, current)
	if i < 0 || headModes[i].id == cfg.HeadMode {
		return
	}
	cfg.HeadMode = headModes[i].id
	if cfg.HeadMode != headModeTabs {
		headTabs.ids, headTabs.drafts = nil, nil
	}
	saveConfig()
	layout()
	invalidate(hMain)
	invalidate(sv.hwnd)
}

func tabsMode() bool { return cfg.HeadMode == headModeTabs }

// tabsShown reports whether the strip of tabs is on the screen: only in the panel of a head.
func tabsShown() bool {
	return compact.on && tabsMode() && len(headTabs.ids) > 0 && nav != navSettings
}

func tabIndex(id int64) int {
	for i, t := range headTabs.ids {
		if t == id {
			return i
		}
	}
	return -1
}

// tabAdd opens a tab for a conversation. When there is no room, the first tab that is not
// the one being read makes way.
func tabAdd(id int64) {
	if tabIndex(id) >= 0 {
		return
	}
	if len(headTabs.ids) >= maxHeadTabs {
		for i, t := range headTabs.ids {
			if t != selectedID {
				delete(headTabs.drafts, t)
				headTabs.ids = append(headTabs.ids[:i], headTabs.ids[i+1:]...)
				break
			}
		}
	}
	headTabs.ids = append(headTabs.ids, id)
}

// tabSwitch shows the conversation of a tab. What was being typed is put aside with the
// conversation it was meant for and comes back with it.
func tabSwitch(id int64) {
	t, ok := threadByID(id)
	if !ok {
		return
	}
	if headTabs.drafts == nil {
		headTabs.drafts = map[int64]string{}
	}
	changed := selectedID != id
	if changed && selectedID != 0 && compose.mode == cmNone {
		headTabs.drafts[selectedID] = getText(hInput)
	}
	selectThread(t, true)
	if changed {
		setText(hInput, headTabs.drafts[id])
		inputChanged()
	}
	layout()
	invalidate(hMain)
}

// tabClose closes one tab; the last one takes the panel with it, and the head stays.
func tabClose(id int64) {
	i := tabIndex(id)
	if i < 0 {
		return
	}
	headTabs.ids = append(headTabs.ids[:i], headTabs.ids[i+1:]...)
	if len(headTabs.ids) == 0 {
		delete(headTabs.drafts, id)
		hideOverlay()
		return
	}
	if id == selectedID {
		if i >= len(headTabs.ids) {
			i = len(headTabs.ids) - 1
		}
		tabSwitch(headTabs.ids[i])
	}
	delete(headTabs.drafts, id)
	layout()
	invalidate(hMain)
}

// tabNext goes to the tab after the current one, round and round.
func tabNext(step int) {
	n := len(headTabs.ids)
	if n < 2 {
		return
	}
	i := tabIndex(selectedID)
	if i < 0 {
		i = 0
	}
	tabSwitch(headTabs.ids[((i+step)%n+n)%n])
}

// paintTabs draws the strip: a small face, the name and a cross for each conversation, the
// one being read lit and marked along its top.
func paintTabs(c canvas, hdc uintptr) {
	s := lay.tabs
	headTabs.hit, headTabs.cross = headTabs.hit[:0], headTabs.cross[:0]
	if s.bottom <= s.top {
		return
	}
	c.fill(s, pal.mica)
	c.fill(rect{s.left, s.bottom - 1, s.right, s.bottom}, pal.line)
	n := int32(len(headTabs.ids))
	w := (s.right - s.left) / n
	if w > dp(170) {
		w = dp(170)
	}
	g := newGfx(hdc)
	d := dp(22)
	type label struct {
		text     string
		r        rect
		initials string
		face     rect
		cur      bool
	}
	var labels []label
	for i, id := range headTabs.ids {
		x := s.left + int32(i)*w
		r := rect{x, s.top, x + w, s.bottom}
		cur := id == selectedID
		if cur {
			c.fill(rect{r.left, r.top, r.right, r.bottom}, pal.card)
			c.fill(rect{r.left, r.top, r.right, r.top + dp(2)}, pal.accent)
		}
		c.fill(rect{r.right - 1, r.top + dp(6), r.right, r.bottom - dp(6)}, pal.line)
		t, _ := threadByID(id)
		fx, fy := r.left+dp(8), r.top+(s.bottom-s.top-d)/2
		l := label{text: threadTitle(t), cur: cur, face: rect{fx, fy, fx + d, fy + d}}
		if !g.avatar(id, fx, fy, d) {
			g.ellipse(fx, fy, d, d, mix(pal.accent, pal.card, 30))
			l.initials = initials(threadTitle(t))
		}
		cross := rect{r.right - dp(26), r.top + dp(6), r.right - dp(4), r.bottom - dp(6)}
		l.r = rect{fx + d + dp(6), r.top, cross.left - dp(2), r.bottom}
		labels = append(labels, l)
		headTabs.hit = append(headTabs.hit, r)
		headTabs.cross = append(headTabs.cross, cross)
	}
	g.free()
	for i, l := range labels {
		ink := pal.fg2
		if l.cur {
			ink = pal.fg
		}
		if l.initials != "" {
			c.font(fnt.small)
			c.text(l.initials, l.face, pal.fg, dtCenter|dtVCenter|dtSingleLine)
		}
		c.font(fnt.small)
		c.text(l.text, l.r, ink, dtSingleLine|dtVCenter|dtEndEllipsis)
		c.font(fnt.iconSm)
		c.text(gClose, headTabs.cross[i], pal.fg2, dtCenter|dtVCenter|dtSingleLine)
	}
}

// tabsClick answers a click on the strip: the cross closes a tab, the rest of it shows it.
func tabsClick(x, y int32) bool {
	in := func(r rect) bool { return x >= r.left && x < r.right && y >= r.top && y < r.bottom }
	if !in(lay.tabs) {
		return false
	}
	for i := range headTabs.hit {
		if i >= len(headTabs.ids) {
			break
		}
		id := headTabs.ids[i]
		switch {
		case in(headTabs.cross[i]):
			tabClose(id)
			return true
		case in(headTabs.hit[i]):
			tabSwitch(id)
			return true
		}
	}
	return true
}
