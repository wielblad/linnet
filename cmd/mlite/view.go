//go:build windows

package main

import (
	"fmt"
	"os"
	"hash/fnv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"

	"github.com/rs/zerolog/log"

	"lazyfmess/internal/store"
)

const (
	itDay = iota
	itMsg
	itNew
	itTop // the top of what is loaded: fetching older messages, or the beginning
)

const (
	stNone = iota
	stPending
	stSent
	stRead
	stFailed
	stDelivered
)

type vitem struct {
	kind       int
	y, h       int32
	label      string
	m          store.Message
	me         bool
	cont, next bool
	showName   bool
	state      int
	top        int32
	bx, bw, bh int32
	textW      int32
	textH      int32
	body       string
	inlineMeta bool
	readers    []int64 // participants whose reading stopped at this message
	photo      bool    // the sender's picture was drawn, so no initials
	img        bool    // the message shows a picture
	imgReady   bool    // the picture itself can be drawn
	imgW, imgH int32
	imgTop     int32 // offset of the picture from the top of the bubble
	imgMsg     store.Message // what the picture is fetched for: the message, or a video preview
	preview    bool          // the picture stands for a link, not for a photo
	og         string        // link whose preview is fetched from its page when it is seen
	play       bool          // the link is a video: it opens in a small window
	link       string        // first web address in the text
	chip       string        // line that opens an attachment or the link
	chipIcon   string
	chipAct    int
	chipTop    int32
	links      []linkSpan // web addresses in the text
	metaW      int32
	media      string
	rx         string
	rxW        int32
	timeS      string
}

var mv struct {
	hwnd     uintptr
	items    []vitem
	total    int32
	scroll   int32
	w, h     int32
	key      uint64
	thread   store.Thread
	has      bool
	group    bool
	msgs     []store.Message
	receipts []store.Receipt
	nicks    map[int64]string
	self     int64
	newAfter time.Time
	hasNew   bool
	focus    bool
	sc       scroller
	hover    int // index of the message under the pointer, -1 when none
	flash    string // message marked for a moment after a jump from the search
	toast    string // short confirmation shown over the bottom of the view
	limit     int   // how many of the newest messages are loaded
	limitFor  int64 // the thread that limit belongs to
	localMore bool  // the store holds older messages than the loaded ones
	spin      int   // step of the fetching mark
	hotAct   int // which of its actions is under the pointer
}

func isGroup(t store.Thread) bool { return !table.ThreadType(t.ThreadType).IsOneToOne() }
func isE2EE(t store.Thread) bool  { return table.ThreadType(t.ThreadType).IsWhatsApp() }

func initials(name string) string {
	parts := strings.Fields(name)
	out := ""
	for _, p := range parts {
		r := []rune(p)
		if len(r) > 0 {
			out += strings.ToUpper(string(r[0]))
		}
		if len([]rune(out)) == 2 {
			break
		}
	}
	if out == "" {
		out = "?"
	}
	return out
}

func avatarColor(id int64) uint32 {
	if id < 0 {
		id = -id
	}
	return pal.avatars[id%6]
}

func dayLabel(t time.Time) string {
	now := time.Now()
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return "Dzisiaj"
	}
	yy, ym, yd := now.AddDate(0, 0, -1).Date()
	if yy == y2 && ym == m2 && yd == d2 {
		return "Wczoraj"
	}
	return t.Format("02.01.2006")
}

func mediaLabel(m store.Message) string {
	switch m.MediaKind {
	case "":
		return ""
	case "image":
		return "Zdjęcie"
	case "video":
		return "Wideo"
	case "audio", "voice":
		return "Nagranie głosowe"
	case "sticker":
		return "Naklejka"
	case "gif":
		return "GIF"
	}
	if m.MediaName != "" {
		return "Plik: " + oneLine(m.MediaName, 40)
	}
	return "Załącznik"
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

// loadView reads the selected thread from the store and lays it out when something changed.
func loadView(force bool) {
	defer panelSync()
	if !mv.has {
		if len(mv.items) > 0 || force {
			mv.items, mv.total, mv.scroll, mv.key = nil, 0, 0, 0
			syncScrollbar()
			invalidate(mv.hwnd)
		}
		return
	}
	if mv.limitFor != mv.thread.ID || mv.limit < windowStart {
		mv.limit, mv.limitFor = windowStart, mv.thread.ID
	}
	msgs, localMore, err := be.db.RecentMessages(be.ctx, mv.thread.ID, mv.limit)
	if err != nil {
		return
	}
	mv.localMore = localMore
	defer olderSoon()
	self := be.accountID()
	receipts, _ := be.db.Receipts(be.ctx, mv.thread.ID, self)
	nicks, _ := be.db.Nicknames(be.ctx, mv.thread.ID)
	hash := fnv.New64a()
	fmt.Fprintf(hash, "%d|%d|%v|%d|%v;", mv.thread.ID, mv.w, pal.dark, textScale, nicks)
	fmt.Fprintf(hash, "%d|%v|%d;", len(msgs), localMore, olderStateFor(localMore))
	for _, r := range receipts {
		fmt.Fprintf(hash, "%d:%d:%d,", r.ContactID, r.Read.UnixMilli(), r.Delivered.UnixMilli())
	}
	for _, m := range msgs {
		fmt.Fprintf(hash, "%s|%s|%v|%s|%v|%v|%d;", m.ID, m.Text, m.Pending, m.Error, m.Unsent, m.Edited, len(m.Reactions))
	}
	key := hash.Sum64()
	if key == mv.key && !force {
		return
	}
	stick := mv.key == 0 || mv.scroll >= mv.total-mv.h-dp(48)
	// What the reader is looking at stays put when messages are added above it: the first
	// message on screen is found again after the layout and put back at the same height.
	anchor, anchorOff := "", int32(0)
	if !stick {
		for i := range mv.items {
			if it := &mv.items[i]; it.kind == itMsg && it.y+it.h > mv.scroll {
				anchor, anchorOff = it.m.ID, it.y-mv.scroll
				break
			}
		}
	}
	mv.key, mv.msgs, mv.receipts, mv.nicks, mv.self = key, msgs, receipts, nicks, self
	mv.group = isGroup(mv.thread)
	started := time.Now()
	layoutView()
	if timing { // measurements only
		log.Warn().Int("messages", len(msgs)).Dur("layout", time.Since(started)).Msg("timing")
	}
	if stick {
		mv.scroll = mv.total - mv.h
	} else if anchor != "" {
		for i := range mv.items {
			if it := &mv.items[i]; it.kind == itMsg && it.m.ID == anchor {
				mv.sc.stop()
				mv.scroll = it.y - anchorOff
				break
			}
		}
	}
	clampScroll()
	syncScrollbar()
	invalidate(mv.hwnd)
}

func layoutView() {
	mv.hover, mv.hotAct = -1, -1
	hdc, _, _ := pGetDC.Call(mv.hwnd)
	defer pReleaseDC.Call(mv.hwnd, hdc)
	c := canvas{hdc}
	padL, padR, padT, padB := dp(12), dp(10), dp(6), dp(7)
	side := dp(16)
	lead := int32(0)
	if mv.group {
		lead = dp(36)
	}
	maxBubble := (mv.w - 2*side - lead) * 76 / 100
	if maxBubble > dp(460) {
		maxBubble = dp(460)
	}
	if maxBubble < dp(120) {
		maxBubble = dp(120)
	}
	inner := maxBubble - padL - padR
	items := make([]vitem, 0, len(mv.msgs)+8)
	y := dp(8)
	if len(mv.msgs) > 0 { // room at the top for the fetching mark; always there, so nothing jumps
		label := ""
		switch olderState() {
		case 1:
			label = "Wczytywanie starszych wiadomości"
		case 2:
			label = "Początek rozmowy"
		}
		items = append(items, vitem{kind: itTop, y: y, h: sp(40), label: label})
		y += sp(40)
	}
	lastDay := ""
	newDone := !mv.hasNew
	// where each other participant stopped reading: the index of the last message they have seen
	readersAt := map[int][]int64{}
	for _, r := range mv.receipts {
		last := -1
		for i := range mv.msgs {
			if mv.msgs[i].Timestamp.After(r.Read) {
				break
			}
			last = i
		}
		if last >= 0 && mv.msgs[last].SenderID != r.ContactID {
			readersAt[last] = append(readersAt[last], r.ContactID)
		}
	}
	for i := range mv.msgs {
		m := mv.msgs[i]
		me := m.SenderID == mv.self && mv.self != 0
		day := dayLabel(m.Timestamp)
		prevOK := false
		if day != lastDay {
			items = append(items, vitem{kind: itDay, y: y, h: sp(34), label: day})
			y += sp(34)
			lastDay = day
		} else if i > 0 {
			p := mv.msgs[i-1]
			prevOK = p.SenderID == m.SenderID && m.Timestamp.Sub(p.Timestamp) < 5*time.Minute && p.Error == ""
		}
		if !newDone && !me && m.Timestamp.After(mv.newAfter) {
			items = append(items, vitem{kind: itNew, y: y, h: sp(30), label: "Nowe wiadomości"})
			y += sp(30)
			newDone = true
			prevOK = false
		}
		nextOK := false
		if i+1 < len(mv.msgs) {
			n := mv.msgs[i+1]
			nextOK = n.SenderID == m.SenderID && dayLabel(n.Timestamp) == day && n.Timestamp.Sub(m.Timestamp) < 5*time.Minute && m.Error == ""
		}
		it := vitem{kind: itMsg, m: m, me: me, cont: prevOK, next: nextOK}
		it.showName = mv.group && !me && !prevOK
		it.body = emojify(m.Text)
		if m.Unsent {
			it.body = "Wiadomość usunięta"
		}
		it.media = mediaLabel(m)
		if lbl := placeholderLabel(m.Text); lbl != "" && m.MediaKind == "" {
			it.body, it.media = "", lbl // kept by the core as a bare word, before attachments were stored
		}
		if showsInline(m) {
			it.img, it.media, it.imgMsg = true, "", m
			it.imgW, it.imgH, it.imgReady = mediaBox(m, inner)
		} else if hasAttachment(m) { // video, recording, file: a line that fetches and opens it
			it.chip, it.chipIcon, it.chipAct, it.media = attachmentLabel(m), attachmentIcon(m), chipFile, ""
		} else if u := linkOf(m); u != "" && !m.Unsent {
			// A link: the picture Messenger attached to it (for YouTube the video's own
			// preview picture when there is none), and a line with the title that opens it.
			it.link = u
			id := youtubeID(u)
			it.play = id != "" || playableLink(u)
			title := oneLine(m.LinkTitle, 90)
			switch {
			case m.LinkImage != "" || m.LinkRef != "":
				it.imgMsg = linkMessage(m)
			case id != "":
				it.imgMsg = ytMessage(id)
			case metaHost(u): // no preview came with the message: the page's own, once fetched
				if pm, t, ok := ogHave(u); ok {
					it.imgMsg = pm
					if title == "" {
						title = oneLine(t, 90)
					}
				} else {
					it.og = u // fetched when the message comes into view
				}
			}
			if it.imgMsg.ID != "" {
				it.imgW, it.imgH, it.imgReady = mediaBox(it.imgMsg, inner)
				if max := dp(220); it.imgH > max { // a preview stays smaller than a photo
					it.imgW, it.imgH = it.imgW*max/it.imgH, max
				}
				// a preview that cannot be fetched is left out rather than shown as a failed photo
				it.img = it.imgReady || md.busy[it.imgMsg.ID] || id != ""
				it.preview = it.img
			}
			switch {
			case title != "" && it.play:
				it.chip, it.chipIcon, it.chipAct = title+" · "+linkHost(u), gPlay, chipPlay
			case title != "":
				it.chip, it.chipIcon, it.chipAct = title+" · "+linkHost(u), gLink, chipLink
			case id != "" && it.img: // the picture with the play sign says it all
			case it.play:
				it.chip, it.chipIcon, it.chipAct = "Odtwórz w okienku · "+linkHost(u), gPlay, chipPlay
			default:
				it.chip, it.chipIcon, it.chipAct = "Otwórz "+linkHost(u), gLink, chipLink
			}
		}
		it.rx = reactionSummary(m.Reactions)
		switch {
		case !me:
			it.state = stNone
		case m.Pending:
			it.state = stPending
		case m.Error != "":
			it.state = stFailed
		default:
			it.state = stSent
			for _, r := range mv.receipts {
				if !r.Read.Before(m.Timestamp) {
					it.state = stRead
					break
				}
				if !r.Delivered.Before(m.Timestamp) {
					it.state = stDelivered
				}
			}
		}
		it.timeS = m.Timestamp.Format("15:04")
		if m.Edited {
			it.timeS = "edytowano " + it.timeS
		}
		c.font(fnt.small)
		it.metaW = c.width(it.timeS)
		if me && it.state != stFailed {
			it.metaW += dp(18)
		}
		c.font(fnt.body)
		it.textW, it.textH = c.measure(it.body, inner)
		it.links = findLinks(&it)
		contentW := it.textW
		contentH := it.textH
		if it.body != "" && it.textH <= fnt.lineH+2 && it.media == "" && !it.img && it.chip == "" && m.ReplyText == "" && it.textW+dp(12)+it.metaW <= inner {
			it.inlineMeta = true
			contentW = it.textW + dp(12) + it.metaW
			if contentH < fnt.lineH {
				contentH = fnt.lineH
			}
		} else {
			contentH += sp(18)
			if it.metaW > contentW {
				contentW = it.metaW
			}
		}
		if it.showName {
			c.font(fnt.bodySemi)
			if w := c.width(senderName(m)); w > contentW {
				contentW = w
			}
			contentH += sp(18)
		}
		if m.ReplyText != "" {
			contentH += sp(40) + dp(4)
			if contentW < dp(150) {
				contentW = dp(150)
			}
		}
		if it.media != "" {
			c.font(fnt.prev)
			if w := c.width(it.media) + dp(24); w > contentW {
				contentW = w
			}
			contentH += fnt.lineH
		}
		if it.img {
			if it.imgW > contentW {
				contentW = it.imgW
			}
			contentH += it.imgH + dp(6)
		}
		if it.chip != "" {
			c.font(fnt.prev)
			if w := c.width(it.chip) + dp(28); w > contentW {
				contentW = w
			}
			contentH += fnt.lineH + dp(6)
		}
		if it.rx != "" { // drawn on the edge of the bubble, so the bubble keeps its size
			c.font(fnt.small)
			it.rxW = c.width(it.rx) + dp(14)
		}
		if contentW > inner {
			contentW = inner
		}
		it.bw = contentW + padL + padR
		it.bh = contentH + padT + padB
		if me {
			it.bx = mv.w - side - it.bw
		} else {
			it.bx = side + lead
		}
		it.top = dp(8)
		if prevOK {
			it.top = dp(2)
		}
		it.y = y
		it.h = it.top + it.bh
		if it.state == stFailed {
			it.h += sp(22)
		}
		if it.rx != "" {
			it.h += dp(18)
		}
		if rs := readersAt[i]; len(rs) > 0 {
			it.readers = rs
			it.h += dp(22)
		}
		y += it.h
		items = append(items, it)
	}
	mv.items = items
	mv.total = y + dp(12)
}

// reactionSummary groups equal reactions: one emoji each, with a count when used more than once.
func reactionSummary(rs []store.Reaction) string {
	var order []string
	count := map[string]int{}
	for _, r := range rs {
		if count[r.Emoji] == 0 {
			order = append(order, r.Emoji)
		}
		count[r.Emoji]++
	}
	parts := make([]string, 0, len(order))
	for _, e := range order {
		if count[e] > 1 {
			parts = append(parts, fmt.Sprintf("%s %d", e, count[e]))
		} else {
			parts = append(parts, e)
		}
	}
	return strings.Join(parts, "   ")
}

func senderName(m store.Message) string {
	if n := mv.nicks[m.SenderID]; n != "" {
		return n
	}
	if m.SenderName != "" {
		return m.SenderName
	}
	return "Nieznana osoba"
}

func clampScroll() {
	max := mv.total - mv.h
	if max < 0 {
		max = 0
	}
	if mv.scroll > max {
		mv.scroll = max
	}
	if mv.scroll < 0 {
		mv.scroll = 0
	}
}

// syncScrollbar is kept for the callers: the bar is drawn over the content from the position itself.
func syncScrollbar() {}

// scrollBy scrolls the conversation smoothly.
func scrollBy(d int32) { mv.sc.by(d) }

// viewScrolled runs whenever the conversation moved.
func viewScrolled() {
	viewHover(-1, -1)
	olderSoon()
}

// olderStateFor is olderState for a load in progress, before its result is in place.
func olderStateFor(localMore bool) int {
	switch {
	case older.busy == mv.thread.ID:
		return 1
	case !localMore && older.done[mv.thread.ID]:
		return 2
	}
	return 0
}

func paintView(target uintptr) {
	buf := newBuffer(target, mv.w, mv.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, mv.w, mv.h}, pal.layer)
	if len(mv.items) == 0 {
		msg := "Wybierz rozmowę z listy"
		hint := "Ctrl+K szuka rozmowy lub osoby."
		if mv.has {
			msg, hint = "Nie ma tu jeszcze wiadomości", "Napisz pierwszą."
		}
		c.font(fnt.name)
		c.text(msg, rect{0, mv.h/2 - sp(22), mv.w, mv.h / 2}, pal.fg, dtCenter|dtSingleLine)
		c.font(fnt.prev)
		c.text(hint, rect{0, mv.h / 2, mv.w, mv.h/2 + sp(22)}, pal.fg2, dtCenter|dtSingleLine)
		buf.flush(0, 0)
		return
	}
	padL, padR, padT := dp(12), dp(10), dp(6)
	R, r := dp(14), dp(5)
	shadow := mix(0x000000, pal.layer, 12)

	// pass 1: shapes
	g := newGfx(buf.hdc)
	for i := range mv.items {
		it := &mv.items[i]
		y := it.y - mv.scroll
		if y+it.h < 0 || y > mv.h {
			continue
		}
		switch it.kind {
		case itTop:
			if olderState() == 1 { // three dots, one after another brighter
				for k := int32(0); k < 3; k++ {
					col := mix(pal.fg2, pal.layer, 35)
					if int32(mv.spin%3) == k {
						col = pal.accent
					}
					g.ellipse(mv.w/2-dp(15)+k*dp(12), y+sp(4), dp(6), dp(6), col)
				}
			}
		case itDay:
			c.font(fnt.small)
			w := c.width(it.label) + dp(20)
			g.pill((mv.w-w)/2, y+sp(10)+1, w, sp(20), shadow)
			g.pill((mv.w-w)/2, y+sp(10), w, sp(20), pal.card)
		case itMsg:
			by := y + it.top
			fill := pal.card
			tl, tr, br, bl := R, R, R, R
			if it.me {
				fill = pal.own
				if it.cont {
					tr = r
				}
				if it.next {
					br = r
				}
			} else {
				if it.cont {
					tl = r
				}
				if it.next {
					bl = r
				}
			}
			if mv.flash != "" && it.m.ID == mv.flash { // found by the search: a frame for a moment
				g.round(it.bx-dp(3), by-dp(3), it.bw+dp(6), it.bh+dp(6), tl+dp(3), tr+dp(3), br+dp(3), bl+dp(3), pal.accent)
			}
			g.round(it.bx, by+1, it.bw, it.bh, tl, tr, br, bl, shadow)
			g.round(it.bx, by, it.bw, it.bh, tl, tr, br, bl, fill)
			paintTextMarks(g, it, fill)
			if it.og != "" {
				ogKick(it.og)
			}
			rxDrop := int32(0)
			if it.rx != "" {
				rxDrop = dp(18)
			}
			for k, id := range it.readers {
				if k == 6 {
					break
				}
				g.avatar(id, mv.w-dp(16)-int32(k+1)*dp(18)+dp(2), by+it.bh+dp(4)+rxDrop, dp(16))
			}
			cy := by + padT
			if it.showName {
				cy += sp(18)
			}
			if it.m.ReplyText != "" {
				g.round(it.bx+padL, cy, it.bw-padL-padR, sp(40), dp(4), dp(4), dp(4), dp(4), mix(pal.accent, fill, 12))
				g.round(it.bx+padL, cy, dp(3), sp(40), dp(2), dp(2), dp(2), dp(2), pal.accent)
				cy += sp(40) + dp(4)
			}
			if it.img {
				it.imgTop = cy - by
				var th uintptr
				if it.imgReady {
					th = mediaThumb(it.imgMsg, it.imgW, it.imgH)
				}
				if th != 0 {
					g.image(th, it.bx+padL, cy, it.imgW, it.imgH)
					if it.preview && it.play { // a dark disc for the play sign
						g.ellipseA(it.bx+padL+it.imgW/2-dp(22), cy+it.imgH/2-dp(22), dp(44), dp(44), 0xB0000000)
					}
				} else {
					it.imgReady = false
					g.round(it.bx+padL, cy, it.imgW, it.imgH, dp(10), dp(10), dp(10), dp(10), mix(pal.fg, fill, 9))
				}
			}
			if it.rx != "" {
				rx, ry := it.bx+it.bw-it.rxW-dp(6), by+it.bh-dp(6)
				g.pill(rx-dp(2), ry-dp(2), it.rxW+dp(4), dp(22)+dp(4), pal.layer)
				g.pill(rx, ry, it.rxW, dp(22), pal.card)
			}
			if mv.group && !it.me && !it.next {
				it.photo = g.avatar(it.m.SenderID, dp(16), by+it.bh-dp(28), dp(28))
			}
		}
	}
	g.free()

	// pass 2: text
	for i := range mv.items {
		it := &mv.items[i]
		y := it.y - mv.scroll
		if y+it.h < 0 || y > mv.h {
			continue
		}
		switch it.kind {
		case itTop:
			if it.label != "" {
				c.font(fnt.small)
				c.text(it.label, rect{0, y + sp(14), mv.w, y + sp(36)}, pal.fg2, dtCenter|dtVCenter|dtSingleLine)
			}
		case itDay:
			c.font(fnt.small)
			c.text(it.label, rect{0, y + sp(10), mv.w, y + sp(30)}, pal.fg2, dtCenter|dtVCenter|dtSingleLine)
		case itNew:
			c.font(fnt.smallSemi)
			w := c.width(it.label)
			mid := y + it.h/2
			c.fill(rect{dp(16), mid, (mv.w-w)/2 - dp(10), mid + 1}, mix(pal.accent, pal.layer, 50))
			c.fill(rect{(mv.w+w)/2 + dp(10), mid, mv.w - dp(16), mid + 1}, mix(pal.accent, pal.layer, 50))
			c.text(it.label, rect{0, y, mv.w, y + it.h}, pal.accentText, dtCenter|dtVCenter|dtSingleLine)
		case itMsg:
			by := y + it.top
			x0 := it.bx + padL
			x1 := it.bx + it.bw - padR
			cy := by + padT
			if it.showName {
				c.font(fnt.bodySemi)
				c.text(senderName(it.m), rect{x0, cy, x1, cy + sp(18)}, pal.accentText, dtSingleLine|dtEndEllipsis)
				cy += sp(18)
			}
			if it.m.ReplyText != "" {
				who := it.m.ReplySenderName
				if n := mv.nicks[it.m.ReplySenderID]; n != "" {
					who = n
				}
				if it.m.ReplySenderID == mv.self && mv.self != 0 {
					who = "Ty"
				}
				c.font(fnt.smallSemi)
				c.text(who, rect{x0 + dp(10), cy + dp(3), x1 - dp(6), cy + sp(20)}, pal.fg, dtSingleLine|dtEndEllipsis)
				c.font(fnt.small)
				c.text(oneLine(it.m.ReplyText, 120), rect{x0 + dp(10), cy + sp(20), x1 - dp(6), cy + sp(38)}, pal.fg2, dtSingleLine|dtEndEllipsis)
				cy += sp(40) + dp(4)
			}
			if it.media != "" {
				c.font(fnt.icon)
				c.text(gPhoto, rect{x0, cy, x0 + dp(18), cy + fnt.lineH}, pal.fg2, dtSingleLine|dtVCenter)
				c.font(fnt.prev)
				c.text(it.media, rect{x0 + dp(24), cy, x1, cy + fnt.lineH}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
				cy += fnt.lineH
			}
			if it.img {
				if !it.imgReady {
					c.font(fnt.small)
					c.text(mediaNote(it.imgMsg), rect{x0 + dp(8), cy, x0 + it.imgW - dp(8), cy + it.imgH}, pal.fg2, dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
					} else if it.preview && it.play {
						c.font(fnt.iconLg)
						c.text(gPlay, rect{x0, cy, x0 + it.imgW, cy + it.imgH}, 0xFFFFFF, dtCenter|dtVCenter|dtSingleLine)
					}
				cy += it.imgH + dp(6)
			}
			col := pal.fg
			if it.m.Unsent {
				col = pal.fg2
			}
			c.font(fnt.body)
			c.text(it.body, rect{x0, cy, x0 + it.textW + 1, cy + it.textH}, col, dtWordBreak|dtEditControl)
			if it.chip != "" {
				ty := cy
				if it.body != "" {
					ty += it.textH + dp(4)
				}
				it.chipTop = ty - by
				chipCol := pal.accentText
				if it.me {
					chipCol = pal.ownAccent
				}
				c.font(fnt.icon)
				c.text(it.chipIcon, rect{x0, ty, x0 + dp(20), ty + fnt.lineH}, chipCol, dtSingleLine|dtVCenter)
				c.font(fnt.prev)
				c.text(it.chip, rect{x0 + dp(24), ty, x1, ty + fnt.lineH}, chipCol, dtSingleLine|dtVCenter|dtEndEllipsis)
			}
			if it.rx != "" {
				c.font(fnt.small)
				rx, ry := it.bx+it.bw-it.rxW-dp(6), by+it.bh-dp(6)
				c.text(it.rx, rect{rx, ry, rx + it.rxW, ry + dp(22)}, pal.fg, dtCenter|dtVCenter|dtSingleLine)
			}
			// time and delivery state
			my := by + it.bh - dp(7) - sp(16)
			if it.inlineMeta {
				my = cy + (fnt.lineH-sp(16))/2
			}
			mx := x1 - it.metaW
			c.font(fnt.small)
			tw := c.width(it.timeS)
			meta := pal.fg2
			if it.me {
				meta = pal.ownMeta
			}
			c.text(it.timeS, rect{mx, my, mx + tw, my + sp(16)}, meta, dtSingleLine|dtVCenter)
			if it.me && it.state != stFailed {
				glyph, gc := gTick, meta
				switch it.state {
				case stPending:
					glyph = gClock
				case stRead:
					gc = pal.ownAccent
				}
				c.font(fnt.iconSm)
				c.text(glyph, rect{mx + tw + dp(4), my, x1 + dp(2), my + sp(16)}, gc, dtSingleLine|dtVCenter)
				if it.state == stRead || it.state == stDelivered {
					c.text(glyph, rect{mx + tw + dp(8), my, x1 + dp(6), my + sp(16)}, gc, dtSingleLine|dtVCenter)
				}
			}
			if it.state == stFailed {
				c.font(fnt.smallSemi)
				c.text("Nie wysłano. Kliknij, aby ponowić.", rect{dp(16), by + it.bh + dp(2), mv.w - dp(16), by + it.bh + sp(22)}, pal.signal, dtRight|dtSingleLine|dtVCenter)
			}
			if mv.group && !it.me && !it.next && !it.photo {
				c.font(fnt.avSm)
				c.text(initials(senderName(it.m)), rect{dp(16), by + it.bh - dp(28), dp(44), by + it.bh}, pal.fg, dtCenter|dtVCenter|dtSingleLine)
			}
		}
	}
	var toast rect
	if mv.toast != "" {
		c.font(fnt.small)
		tw, th := c.width(mv.toast)+dp(28), sp(18)+dp(14)
		toast = rect{(mv.w - tw) / 2, mv.h - th - dp(14), (mv.w + tw) / 2, mv.h - dp(14)}
	}
	bar := newGfx(buf.hdc)
	paintActions(bar, c, true)
	mv.sc.paint(bar, mv.w, mv.h, pal.layer)
	if mv.toast != "" { // drawn over the messages: it takes no room of its own
		bar.pill(toast.left, toast.top+1, toast.right-toast.left, toast.bottom-toast.top, shadow)
		bar.pill(toast.left, toast.top, toast.right-toast.left, toast.bottom-toast.top, mix(pal.fg, pal.layer, 92))
	}
	bar.free()
	paintActions(0, c, false)
	if mv.toast != "" {
		c.font(fnt.small)
		c.text(mv.toast, toast, pal.layer, dtCenter|dtVCenter|dtSingleLine)
	}
	buf.flush(0, 0)
}

// relayoutView lays the conversation out again and keeps the reader where they were.
func relayoutView() {
	if !mv.has {
		return
	}
	atBottom := mv.scroll >= mv.total-mv.h-dp(48)
	anchor, off := -1, int32(0)
	for i := range mv.items {
		if mv.items[i].y+mv.items[i].h > mv.scroll {
			anchor, off = i, mv.scroll-mv.items[i].y
			break
		}
	}
	layoutView()
	switch {
	case atBottom:
		mv.scroll = mv.total - mv.h
	case anchor >= 0 && anchor < len(mv.items):
		mv.scroll = mv.items[anchor].y + off
	}
	clampScroll()
	syncScrollbar()
	invalidate(mv.hwnd)
}

// viewToast shows a short confirmation over the bottom of the conversation; it goes away
// by itself and moves nothing.
func viewToast(text string) {
	mv.toast = text
	pSetTimer.Call(mv.hwnd, timerToast, 1800, 0)
	invalidate(mv.hwnd)
}

func viewClick(x, y int32) {
	if actionClick() {
		return
	}
	for i := range mv.items {
		if it := &mv.items[i]; it.kind == itMsg {
			if u := linkAt(it, x, y); u != "" {
				openLink(u)
				return
			}
		}
	}
	y += mv.scroll
	for i := range mv.items {
		it := &mv.items[i]
		if it.kind == itMsg && it.img {
			ix, iy := it.bx+dp(12), it.y+it.top+it.imgTop
			if x >= ix && x < ix+it.imgW && y >= iy && y < iy+it.imgH {
				if it.preview {
					openLink(it.link)
				} else {
					openViewer(it.imgMsg)
				}
				return
			}
		}
		if it.kind == itMsg && it.chip != "" {
			cy := it.y + it.top + it.chipTop
			if x >= it.bx && x < it.bx+it.bw && y >= cy && y < cy+fnt.lineH {
				switch it.chipAct {
				case chipFile:
					openAttachment(it.m)
				case chipLink:
					openURL(it.link)
				case chipPlay:
					playInWindow(it.link)
				}
				return
			}
		}
		if it.kind == itMsg && it.state == stFailed && y >= it.y && y < it.y+it.h {
			be.retry(mv.thread, it.m)
			return
		}
	}
}

func viewProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		paintView(hdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSize:
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		if rc.right != mv.w || rc.bottom != mv.h {
			atBottom := mv.scroll >= mv.total-mv.h-dp(48)
			widthChanged := rc.right != mv.w
			mv.w, mv.h = rc.right, rc.bottom
			if widthChanged && mv.has {
				layoutView()
			}
			if atBottom {
				mv.scroll = mv.total - mv.h
			}
			clampScroll()
			syncScrollbar()
			invalidate(hwnd)
		}
		return 0
	case wmMouseWheel:
		mv.sc.wheel(int32(int16(wp >> 16)))
		return 0
	case wmMouseMove:
		x, y := lpPoint(lp)
		if sel.drag {
			selMove(x, y)
			return 0
		}
		if !mv.sc.mouse(msg, x, y, mv.w, mv.h) {
			viewHover(x, y)
		}
		return 0
	case wmSetCursor:
		if lp&0xFFFF == 1 && wp == hwnd { // over the client area
			var pt point
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			pScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
			if viewCursor(pt.x, pt.y) {
				return 1
			}
		}
	case wmMouseLeave:
		if keepHover { // checks drive the pointer with messages; the real one is elsewhere
			return 0
		}
		mv.sc.leave()
		viewHover(-1, -1)
		return 0
	case wmTimer:
		if wp == timerOlder {
			pKillTimer.Call(hwnd, timerOlder)
			olderCheck()
			return 0
		}
		if wp == timerSpin {
			mv.spin++
			invalidate(hwnd)
			return 0
		}
		if wp == timerToast {
			pKillTimer.Call(hwnd, timerToast)
			mv.toast = ""
			invalidate(hwnd)
			return 0
		}
		if wp == timerFlash {
			pKillTimer.Call(hwnd, timerFlash)
			mv.flash = ""
			invalidate(hwnd)
			return 0
		}
		if mv.sc.timer(wp) {
			return 0
		}
	case wmVScroll:
		switch wp & 0xFFFF {
		case sbLineUp:
			scrollBy(-dp(40))
		case sbLineDown:
			scrollBy(dp(40))
		case sbPageUp:
			scrollBy(-mv.h * 9 / 10)
		case sbPageDown:
			scrollBy(mv.h * 9 / 10)
		case sbTop:
			scrollBy(-mv.total)
		case sbBottom:
			scrollBy(mv.total)
		case sbThumbTrack:
			si := scrollInfo{fMask: sifTrackPos}
			si.cbSize = uint32(unsafe.Sizeof(si))
			pGetScrollInfo.Call(hwnd, sbVert, uintptr(unsafe.Pointer(&si)))
			scrollBy(si.nTrack - mv.scroll)
		}
		return 0
	case wmGetDlgCode:
		return 0x0001 // DLGC_WANTARROWS
	case wmKeyDown:
		switch wp {
		case vkUp:
			scrollBy(-dp(40))
		case vkDown:
			scrollBy(dp(40))
		case vkPrior:
			scrollBy(-mv.h * 9 / 10)
		case vkNext:
			scrollBy(mv.h * 9 / 10)
		case vkHome:
			scrollBy(-mv.total)
		case vkEnd:
			scrollBy(mv.total)
		}
		return 0
	case wmSetFocus, wmKillFocus:
		mv.focus = msg == wmSetFocus
		invalidate(hwnd)
		return 0
	case wmLButtonDown:
		pSetFocus.Call(hwnd)
		if x, y := lpPoint(lp); mv.sc.mouse(msg, x, y, mv.w, mv.h) {
			return 0
		} else if mv.hotAct < 0 {
			selDown(x, y)
		}
		return 0
	case wmLButtonUp:
		if x, y := lpPoint(lp); mv.sc.mouse(msg, x, y, mv.w, mv.h) {
			return 0
		}
		if selUp() {
			return 0
		}
		viewClick(int32(int16(lp)), int32(int16(lp>>16)))
		return 0
	case wmRButtonUp:
		viewMenu(int32(int16(lp)), int32(int16(lp>>16)), false)
		return 0
	case wmContextMenu:
		if int32(lp) == -1 { // asked for from the keyboard
			viewMenu(0, 0, true)
			return 0
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func registerClass(name string, proc func(uintptr, uint32, uintptr, uintptr) uintptr) {
	hinst, _, _ := pGetModuleHandleW.Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, idcArrow)
	wc := wndClassEx{lpfnWndProc: syscall.NewCallback(proc), hInstance: hinst, hCursor: cursor, lpszClassName: u16(name)}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
}

// keepHover makes the conversation ignore the pointer leaving; set only for automated checks.
var keepHover = os.Getenv("MLITE_KEEPHOVER") != ""
var timing = os.Getenv("MLITE_TIMING") != ""
