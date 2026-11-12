//go:build windows

package main

// The conversation list as a window of its own: pixel-smooth scrolling, an overlay scroll
// bar, rows drawn by drawRow. It still answers the few list-box messages the rest of the
// program and the test scripts use.

import (
	"strings"
	"time"
	"unsafe"

	"lazyfmess/internal/store"
)

var ls struct {
	scroll     int32
	sel, down  int
	w, h       int32
	focus, kbd bool
	sc         scroller
}

var (
	receiptsBy = map[int64][]store.Receipt{} // read marks of every thread; also who takes part
	lastSender = map[int64]int64{}           // sender of the newest message by thread
)

func listRowH() int32  { return int32(rowHeight()) }
func listTotal() int32 { return int32(len(rows)) * listRowH() }

func initList() {
	ls.sel, ls.down = -1, -1
	ls.sc = scroller{hwnd: hList, pos: &ls.scroll, total: listTotal, page: func() int32 { return ls.h }}
}

func rowAt(lp uintptr) int {
	_, y := lpPoint(lp)
	i := int((y + ls.scroll) / listRowH())
	if y+ls.scroll < 0 || i >= len(rows) {
		return -1
	}
	return i
}

func invalidateRow(i int) {
	if i >= 0 {
		invalidate(hList)
	}
}

func curSel() int { return ls.sel }

func listEnsureVisible(i int) {
	if i < 0 {
		return
	}
	top := int32(i) * listRowH()
	switch {
	case top < ls.scroll:
		ls.sc.stop()
		ls.sc.set(top)
	case top+listRowH() > ls.scroll+ls.h:
		ls.sc.stop()
		ls.sc.set(top + listRowH() - ls.h)
	}
}

func paintList(target uintptr) {
	buf := newBuffer(target, ls.w, ls.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, ls.w, ls.h}, pal.mica)
	rh := listRowH()
	for i := int(ls.scroll / rh); i < len(rows); i++ {
		y := int32(i)*rh - ls.scroll
		if y >= ls.h {
			break
		}
		d := drawItem{hDC: buf.hdc, itemID: uint32(i), rcItem: rect{0, y, ls.w, y + rh}, itemState: odsNoFocusRect}
		if i == ls.sel {
			d.itemState |= odsSelected
			if ls.focus && ls.kbd {
				d.itemState = odsSelected | odsFocus
			}
		}
		drawRow(&d)
	}
	g := newGfx(buf.hdc)
	ls.sc.paint(g, ls.w, ls.h, pal.mica)
	g.free()
	buf.flush(0, 0)
	if len(rows) == 0 {
		paintEmptyList()
	}
}

func listMove(to int) {
	if len(rows) == 0 {
		return
	}
	if to < 0 {
		to = 0
	}
	if to >= len(rows) {
		to = len(rows) - 1
	}
	ls.sel, ls.kbd = to, true
	listEnsureVisible(to)
	invalidate(hList)
	listAnnounce()
	if nav == navChats && !lay.narrow {
		commitRow(to, false)
	}
}

func listWndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		paintList(hdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSize:
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		ls.w, ls.h = rc.right, rc.bottom
		ls.sc.set(ls.scroll)
		invalidate(hwnd)
		return 0

	// what a list box would answer
	case lbSetCurSel:
		ls.sel = int(int32(wp))
		if ls.sel >= len(rows) {
			ls.sel = -1
		}
		listAnnounce()
		listEnsureVisible(ls.sel)
		invalidate(hwnd)
		return 0
	case lbGetCurSel:
		return uintptr(ls.sel)
	case lbResetContent:
		ls.sel = -1
		return 0
	case lbGetTopIndex:
		return uintptr(ls.scroll)
	case lbSetTopIndex:
		ls.scroll = int32(wp)
		return 0
	case lbAddString, lbSetItemHeight:
		return 0
	case wmSetRedraw:
		if wp != 0 {
			ls.sc.set(ls.scroll)
			if m := ls.sc.max(); ls.scroll > m {
				ls.scroll = m
			}
			invalidate(hwnd)
		}
		return 0

	case wmMouseWheel:
		ls.sc.wheel(int32(int16(wp >> 16)))
		return 0
	case wmMouseMove:
		x, y := lpPoint(lp)
		if ls.sc.mouse(msg, x, y, ls.w, ls.h) {
			return 0
		}
		setHotRow(rowAt(lp))
		return 0
	case wmMouseLeave:
		ls.sc.leave()
		setHotRow(-1)
		return 0
	case wmLButtonDown:
		pSetFocus.Call(hwnd)
		x, y := lpPoint(lp)
		if ls.sc.mouse(msg, x, y, ls.w, ls.h) {
			return 0
		}
		ls.kbd = false
		ls.down = rowAt(lp)
		if ls.down >= 0 {
			ls.sel = ls.down
			listAnnounce()
			invalidate(hwnd)
		}
		return 0
	case wmLButtonUp:
		x, y := lpPoint(lp)
		if ls.sc.mouse(msg, x, y, ls.w, ls.h) {
			return 0
		}
		if i := rowAt(lp); i >= 0 && i == ls.down {
			commitRow(i, true)
		}
		ls.down = -1
		return 0
	case wmRButtonUp:
		rowMenu(rowAt(lp), lp)
		return 0
	case wmGetDlgCode:
		return 0x0001 // arrows belong to the list
	case wmKeyDown:
		per := int(ls.h / listRowH())
		if per < 1 {
			per = 1
		}
		switch wp {
		case vkUp:
			listMove(ls.sel - 1)
		case vkDown:
			listMove(ls.sel + 1)
		case vkPrior:
			listMove(ls.sel - per)
		case vkNext:
			listMove(ls.sel + per)
		case vkHome:
			listMove(0)
		case vkEnd:
			listMove(len(rows) - 1)
		}
		return 0
	case wmSetFocus, wmKillFocus:
		ls.focus = msg == wmSetFocus
		invalidate(hwnd)
		return 0
	case wmTimer:
		if ls.sc.timer(wp) {
			return 0
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

// groupAvatar draws a group without a picture of its own as two of its members, the way
// Messenger does. It reports false when no member has a picture.
func groupAvatar(g gfx, threadID int64, x, y, d int32, ring uint32) bool {
	self := be.accountID()
	var ids []int64
	for _, r := range receiptsBy[threadID] {
		if r.ContactID != self && av.urls[r.ContactID] != "" {
			ids = append(ids, r.ContactID)
			if len(ids) == 2 {
				break
			}
		}
	}
	switch len(ids) {
	case 0:
		return false
	case 1:
		g.avatar(ids[0], x, y, d)
		return true
	}
	small := d * 66 / 100
	g.avatar(ids[0], x+d-small, y, small)
	g.ellipse(x-dp(2), y+d-small-dp(2), small+dp(4), small+dp(4), ring)
	g.avatar(ids[1], x, y+d-small, small)
	return true
}

// threadAvatar draws the picture of a conversation: its own, the members' for a group
// without one, or the coloured circle (false: the caller adds initials).
func threadAvatar(g gfx, t store.Thread, x, y, d int32, ring uint32) bool {
	if avatarImage(t.ID, d) == 0 && isGroup(t) && groupAvatar(g, t.ID, x, y, d, ring) {
		return true
	}
	return g.avatar(t.ID, x, y, d)
}

// seenBy lists who has read the conversation up to its last message, when that message is ours.
func seenBy(t store.Thread, self int64) []int64 {
	if self == 0 || lastSender[t.ID] != self {
		return nil
	}
	var out []int64
	for _, r := range receiptsBy[t.ID] {
		if r.ContactID != self && !r.Read.Before(t.UpdatedAt.Add(-time.Second)) {
			out = append(out, r.ContactID)
		}
	}
	return out
}

var emoticons = map[string]string{
	":)": "🙂", ":-)": "🙂", ":D": "😀", ":-D": "😀", ":(": "😞", ":-(": "😞", ";)": "😉", ";-)": "😉",
	":P": "😛", ":p": "😛", ":-P": "😛", ":O": "😮", ":o": "😮", ":*": "😘", ":-*": "😘", "<3": "❤️",
	":/": "😕", ":-/": "😕", ":'(": "😢", "^_^": "😊", "-_-": "😑", "8)": "😎", "B)": "😎", "o.O": "😳", "O.o": "😳",
}

// emojify shows typed emoticons such as :D as emoji, like Messenger does. Only whole words
// are replaced, so addresses and code stay as they are.
func emojify(s string) string {
	if !strings.ContainsAny(s, ":;<^-8Bo") {
		return s
	}
	lines := strings.Split(s, "\n")
	for li, line := range lines {
		words := strings.Split(line, " ")
		for wi, w := range words {
			if e, ok := emoticons[w]; ok {
				words[wi] = e
			}
		}
		lines[li] = strings.Join(words, " ")
	}
	return strings.Join(lines, "\n")
}
