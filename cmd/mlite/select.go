//go:build windows

package main

// Text of a message as something the mouse can work with: web addresses that open on a
// click, and a selection dragged with the mouse that Ctrl+C or the menu copies.

import (
	"math"
	"strings"
	"unicode/utf16"
	"unsafe"
)

const (
	idMsgCopySel = 406
	wmSetCursor  = 0x0020
	idcIBeam     = 32513
	idcHand      = 32649
)

var pSetCursor = user32.NewProc("SetCursor")

// linkSpan is one web address inside the text: where it is and where it is drawn.
type linkSpan struct {
	url     string
	from, n int    // UTF-16 units
	rects   []rect // relative to the start of the text
}

// sel is the dragged selection: inside one message, in UTF-16 units of its text.
type selState struct {
	id      string
	a, b    int
	drag    bool
	moved   bool
	x0, y0  int32
	started bool
}

var sel = &mainBank.sel

type hitMetrics struct {
	pos, length              uint32
	left, top, width, height float32
	bidi                     uint32
	isText, isTrimmed        int32
}

// bodyLayout lays the text of a message out exactly as it is drawn; the caller releases it.
func bodyLayout(it *vitem) uintptr {
	if it.body == "" || !dwReady() {
		return 0
	}
	p := dwFormat(fnt.body)
	if p == 0 {
		return 0
	}
	dwApply(p, fnt.body, dtWordBreak)
	t := u16s(it.body)
	var layout uintptr
	com(dw.dwf, 18, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), p,
		uintptr(math.Float32bits(float32(it.textW+1))), uintptr(math.Float32bits(100000)), uintptr(unsafe.Pointer(&layout)))
	return layout
}

func layoutHit(layout uintptr, x, y int32) int {
	var trailing, inside int32
	var m hitMetrics
	com(layout, 64, uintptr(math.Float32bits(float32(x))), uintptr(math.Float32bits(float32(y))),
		uintptr(unsafe.Pointer(&trailing)), uintptr(unsafe.Pointer(&inside)), uintptr(unsafe.Pointer(&m))) // HitTestPoint
	pos := int(m.pos)
	if trailing != 0 {
		pos += int(m.length)
	}
	return pos
}

func layoutRange(layout uintptr, from, n int) []rect {
	if n <= 0 {
		return nil
	}
	ms := make([]hitMetrics, 48)
	var count uint32
	com(layout, 66, uintptr(from), uintptr(n), 0, 0, uintptr(unsafe.Pointer(&ms[0])), uintptr(len(ms)), uintptr(unsafe.Pointer(&count))) // HitTestTextRange
	if int(count) > len(ms) {
		count = uint32(len(ms))
	}
	out := make([]rect, 0, count)
	for _, m := range ms[:count] {
		out = append(out, rect{int32(m.left), int32(m.top), int32(math.Ceil(float64(m.left + m.width))), int32(math.Ceil(float64(m.top + m.height)))})
	}
	return out
}

// bodyOffset is where the text starts inside the bubble; it follows the order paintView draws in.
func bodyOffset(it *vitem) (int32, int32) {
	dy := dp(6)
	if it.showName {
		dy += sp(18)
	}
	if it.m.ReplyText != "" {
		dy += sp(40) + dp(4)
	}
	if it.media != "" {
		dy += fnt.lineH
	}
	if it.img {
		dy += it.imgH + dp(6)
	}
	return dp(12), dy
}

// bodyOrigin is the start of the text in view coordinates.
func bodyOrigin(it *vitem) (int32, int32) {
	dx, dy := bodyOffset(it)
	return it.bx + dx, it.y + it.top + dy - mv.scroll
}

func u16len(s string) int { return len(utf16.Encode([]rune(s))) }

// findLinks lists the web addresses in the text and measures where each is drawn.
func findLinks(it *vitem) []linkSpan {
	if it.m.Unsent || !strings.Contains(it.body, "http") {
		return nil
	}
	var out []linkSpan
	rest, base := it.body, 0
	for {
		i := strings.Index(rest, "http")
		if i < 0 {
			break
		}
		end := i
		for end < len(rest) && !strings.ContainsRune(" \t\r\n", rune(rest[end])) {
			end++
		}
		word := strings.TrimRight(rest[i:end], ".,;:!?)\"'")
		if u := firstURL(word); u != "" && (i == 0 || strings.ContainsRune(" \t\r\n(", rune(rest[i-1]))) {
			out = append(out, linkSpan{url: u, from: u16len(it.body[:base+i]), n: u16len(u)})
		}
		rest, base = rest[end:], base+end
	}
	if len(out) == 0 {
		return nil
	}
	if layout := bodyLayout(it); layout != 0 {
		for k := range out {
			out[k].rects = layoutRange(layout, out[k].from, out[k].n)
		}
		com(layout, 2)
	}
	return out
}

// linkAt returns the address drawn under a point of the view, or "".
func linkAt(it *vitem, x, y int32) string {
	ox, oy := bodyOrigin(it)
	for _, l := range it.links {
		for _, r := range l.rects {
			if x >= ox+r.left && x < ox+r.right && y >= oy+r.top && y < oy+r.bottom {
				return l.url
			}
		}
	}
	return ""
}

// openLink is what the preview picture of a link does: a video plays inside the window,
// any other page opens in the default browser. The address itself always opens the browser.
func openLink(u string) {
	switch {
	case youtubeID(u) != "":
		playInWindow("https://www.youtube.com/watch?v=" + youtubeID(u))
	case playableLink(u):
		playInWindow(u)
	default:
		openURL(u)
	}
}

// textItem finds the message whose text is under the point.
func textItem(x, y int32) *vitem {
	for i := range mv.items {
		it := &mv.items[i]
		if it.kind != itMsg || it.body == "" || it.m.Unsent {
			continue
		}
		ox, oy := bodyOrigin(it)
		if x >= ox-dp(6) && x < ox+it.textW+dp(6) && y >= oy && y < oy+it.textH {
			return it
		}
	}
	return nil
}

func selItem() *vitem {
	if sel.id == "" {
		return nil
	}
	for i := range mv.items {
		if mv.items[i].kind == itMsg && mv.items[i].m.ID == sel.id {
			return &mv.items[i]
		}
	}
	return nil
}

func hitText(it *vitem, x, y int32) int {
	layout := bodyLayout(it)
	if layout == 0 {
		return 0
	}
	defer com(layout, 2)
	ox, oy := bodyOrigin(it)
	return layoutHit(layout, x-ox, y-oy)
}

// selDown starts a possible selection; it reports whether the press landed on text.
func selDown(x, y int32) bool {
	had := sel.id != "" && sel.a != sel.b
	sel.id, sel.a, sel.b, sel.drag, sel.moved = "", 0, 0, false, false
	it := textItem(x, y)
	if it != nil {
		sel.id, sel.drag, sel.x0, sel.y0 = it.m.ID, true, x, y
		sel.a = hitText(it, x, y)
		sel.b = sel.a
		pSetCapture.Call(mv.hwnd)
	}
	if had {
		invalidate(mv.hwnd)
	}
	return it != nil
}

func selMove(x, y int32) {
	it := selItem()
	if it == nil {
		return
	}
	if !sel.moved && abs32(x-sel.x0) < dp(3) && abs32(y-sel.y0) < dp(3) {
		return // a click that trembled is still a click
	}
	sel.moved = true
	if b := hitText(it, x, y); b != sel.b {
		sel.b = b
		invalidate(mv.hwnd)
	}
}

// selUp ends the drag; true means text was selected, so the release is not a click.
func selUp() bool {
	if !sel.drag {
		return false
	}
	sel.drag = false
	pReleaseCapture.Call()
	if sel.a == sel.b {
		sel.id = ""
		return false
	}
	return true
}

func selClear() {
	if sel.id != "" {
		sel.id, sel.a, sel.b, sel.drag = "", 0, 0, false
		invalidate(mv.hwnd)
	}
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func selBounds() (int, int) {
	if sel.a < sel.b {
		return sel.a, sel.b
	}
	return sel.b, sel.a
}

// selText is the selected text, "" when nothing is selected.
func selText() string {
	it := selItem()
	if it == nil || sel.a == sel.b {
		return ""
	}
	t := utf16.Encode([]rune(it.body))
	from, to := selBounds()
	if to > len(t) {
		to = len(t)
	}
	if from >= to {
		return ""
	}
	return string(utf16.Decode(t[from:to]))
}

// selIn reports whether the selection is inside the given message.
func selIn(id string) bool { return sel.id == id && sel.a != sel.b }

// paintTextMarks draws, under the text of one message, the selection and the lines under
// its web addresses. It is called in the shapes pass, before any text.
func paintTextMarks(g gfx, it *vitem, fill uint32) {
	if it.body == "" {
		return
	}
	ox, oy := bodyOrigin(it)
	selFill, line := mix(pal.accent, fill, 42), mix(pal.fg, fill, 70)
	if it.me { // on one's own bubble both are drawn in its text colour
		selFill, line = mix(pal.ownFg, fill, 34), mix(pal.ownFg, fill, 78)
	}
	if selIn(it.m.ID) {
		if layout := bodyLayout(it); layout != 0 {
			from, to := selBounds()
			for _, r := range layoutRange(layout, from, to-from) {
				g.round(ox+r.left, oy+r.top, r.right-r.left, r.bottom-r.top, dp(2), dp(2), dp(2), dp(2), selFill)
			}
			com(layout, 2)
		}
	}
	for _, l := range it.links {
		for _, r := range l.rects {
			g.round(ox+r.left, oy+r.bottom-dp(2), r.right-r.left, 1, 0, 0, 0, 0, line)
		}
	}
}

// viewCursor picks the pointer for a place of the view: a hand over what opens on a click,
// the text bar over text, the arrow elsewhere. It reports whether it set one.
func viewCursor(x, y int32) bool {
	if onJump(x, y) {
		cur, _, _ := pLoadCursorW.Call(0, idcHand)
		pSetCursor.Call(cur)
		return true
	}
	id := uintptr(0)
	sy := y + mv.scroll
	for i := range mv.items {
		it := &mv.items[i]
		if it.kind != itMsg || sy < it.y || sy >= it.y+it.h {
			continue
		}
		switch {
		case linkAt(it, x, y) != "":
			id = idcHand
		case it.chip != "" && x >= it.bx && x < it.bx+it.bw && sy >= it.y+it.top+it.chipTop && sy < it.y+it.top+it.chipTop+fnt.lineH:
			id = idcHand
		case it.img && x >= it.bx+dp(12) && x < it.bx+dp(12)+it.imgW && sy >= it.y+it.top+it.imgTop && sy < it.y+it.top+it.imgTop+it.imgH:
			id = idcHand
		case mv.hotAct >= 0:
			id = idcHand
		case textItem(x, y) == it:
			id = idcIBeam
		}
		break
	}
	if id == 0 {
		return false
	}
	cur, _, _ := pLoadCursorW.Call(0, id)
	pSetCursor.Call(cur)
	return true
}
