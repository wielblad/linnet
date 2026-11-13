//go:build windows

package main

// A picture from the conversation shown large inside the program's own window: it fills
// the window on a dark ground, with the neighbouring pictures one arrow away and buttons
// to save it, to open it in the Windows viewer, and to close.

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/rs/zerolog/log"

	"lazyfmess/internal/session"
	"lazyfmess/internal/store"
	"lazyfmess/internal/webview"
)

const (
	gDownload = ""
	gOpenOut  = ""
	gCloseBig = ""
	gPrev     = ""
	gNext     = ""

	viewerGround = 0x0E0F12
	viewerInk    = 0xF2F2F4
	viewerInk2   = 0xA9ABB3
	vkLeft       = 0x25
	vkRight      = 0x27
)

const (
	vbSave = iota
	vbOpen
	vbClose
	vbPrev
	vbNext
	vbCount
)

var (
	pGetSaveFileNameW = comdlg32.NewProc("GetSaveFileNameW")

	vw struct {
		hwnd   uintptr
		list   []store.Message // the pictures of the conversation, oldest first
		idx    int
		path   string // file drawn (a PNG copy for WebP)
		iw, ih int32  // its size in pixels
		scaled uintptr
		sw, sh int32
		hot    int
		w, h   int32
		note   string
		btn    [vbCount]rect
		pic    rect
		host   uintptr   // the window it covers: the program's, or a pane
		bank   *viewBank // the view it was opened from
		video  string    // address played in the embedded browser; "" while pictures are shown
		web    *webview.View
		webMsg string // what the player area says until the browser covers it
	}
)

// openViewer shows the picture of a message large, inside the window.
func openViewer(m store.Message) {
	if vw.video != "" || (vw.hwnd != 0 && vw.host != hostWnd()) {
		closeViewer()
	}
	vw.list = vw.list[:0]
	vw.idx = 0
	for i := range mv.items {
		it := &mv.items[i]
		if it.kind == itMsg && it.img && !it.preview {
			if it.imgMsg.ID == m.ID {
				vw.idx = len(vw.list)
			}
			vw.list = append(vw.list, it.imgMsg)
		}
	}
	if len(vw.list) == 0 {
		return
	}
	vw.hot, vw.note = -1, ""
	if vw.hwnd == 0 {
		vw.host, vw.bank = hostWnd(), curBank
		vw.hwnd = createWindow(0, "MliteViewer", "Podgląd zdjęcia", wsChild|wsVisible|wsTabStop|wsClipChildren, vw.host, 0)
	}
	viewerLoad()
	hostLayout()
	pSetFocus.Call(vw.hwnd)
}

func closeViewer() {
	if vw.hwnd == 0 {
		return
	}
	if vw.web != nil { // the browser goes first: its processes end with it
		vw.web.Close()
		vw.web = nil
	}
	playerStop()
	vw.video = ""
	h := vw.hwnd
	vw.hwnd = 0
	viewerDrop()
	vw.list = nil
	pDestroyWindow.Call(h)
	b := vw.bank // back to the conversation it covered
	if b == nil {
		b = &mainBank
	}
	done := enterBank(b)
	hostLayout()
	pSetFocus.Call(mv.hwnd)
	done()
}

func viewerDrop() {
	if vw.scaled != 0 {
		pGdipDisposeImage.Call(vw.scaled)
	}
	vw.scaled, vw.sw, vw.sh = 0, 0, 0
}

// viewerLoad finds the file of the current picture; while it is still being fetched the
// viewer says so and tries again when a download ends.
func viewerLoad() {
	viewerDrop()
	vw.path, vw.iw, vw.ih = "", 0, 0
	if vw.idx < 0 || vw.idx >= len(vw.list) {
		return
	}
	if p, ok := mediaFile(vw.list[vw.idx]); ok {
		vw.path, vw.iw, vw.ih = drawable(p)
	}
	invalidate(vw.hwnd)
}

// viewerRefresh is called when a download ends.
func viewerRefresh() {
	if vw.hwnd != 0 && vw.path == "" {
		viewerLoad()
	}
}

func viewerStep(by int) {
	if n := vw.idx + by; n >= 0 && n < len(vw.list) {
		vw.idx, vw.note = n, ""
		viewerLoad()
	}
}

func viewerMessage() string {
	m := vw.list[vw.idx]
	switch {
	case vw.path != "" && vw.iw > 0:
		return ""
	case md.busy[m.ID]:
		return "Pobieranie zdjęcia…"
	case av.offline:
		return "Zdjęcie nie jest pobrane"
	}
	return "Nie udało się pobrać zdjęcia"
}

// viewerLayout places the picture and the buttons for the current window size.
func viewerLayout(c canvas) {
	bar, pad, s := dp(56), dp(12), dp(36)
	x := vw.w - pad
	vw.btn[vbClose] = rect{x - s, (bar - s) / 2, x, (bar + s) / 2}
	x -= s + dp(8)
	c.font(fnt.body)
	for _, b := range []struct {
		id    int
		label string
	}{{vbOpen, "Otwórz w Windows"}, {vbSave, "Pobierz"}} {
		w := dp(14) + dp(20) + dp(6) + c.width(b.label) + dp(14)
		vw.btn[b.id] = rect{x - w, (bar - s) / 2, x, (bar + s) / 2}
		x -= w + dp(8)
	}
	n := dp(44)
	mid := bar + (vw.h-bar)/2
	vw.btn[vbPrev] = rect{dp(12), mid - n/2, dp(12) + n, mid + n/2}
	vw.btn[vbNext] = rect{vw.w - dp(12) - n, mid - n/2, vw.w - dp(12), mid + n/2}
	// the picture: as large as fits, never larger than it is
	availW, availH := vw.w-2*(dp(12)+n+dp(12)), vw.h-bar-dp(40)
	vw.pic = rect{}
	if vw.iw > 0 && vw.ih > 0 && availW > 0 && availH > 0 {
		w, h := vw.iw, vw.ih
		if w > availW {
			h, w = h*availW/w, availW
		}
		if h > availH {
			w, h = w*availH/h, availH
		}
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		left, top := (vw.w-w)/2, bar+(vw.h-bar-dp(28)-h)/2
		vw.pic = rect{left, top, left + w, top + h}
	}
}

func viewerHas(id int) bool {
	if vw.video != "" {
		return id == vbOpen || id == vbClose
	}
	switch id {
	case vbPrev:
		return vw.idx > 0
	case vbNext:
		return vw.idx < len(vw.list)-1
	case vbSave, vbOpen:
		return vw.path != ""
	}
	return true
}

func paintViewer(target uintptr) {
	buf := newBuffer(target, vw.w, vw.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, vw.w, vw.h}, viewerGround)
	viewerLayout(c)
	hotFill := mix(viewerInk, viewerGround, 16)
	edge := mix(viewerInk, viewerGround, 26)

	g := newGfx(buf.hdc)
	if pw, ph := vw.pic.right-vw.pic.left, vw.pic.bottom-vw.pic.top; pw > 0 && ph > 0 {
		if vw.scaled == 0 || vw.sw != pw || vw.sh != ph {
			viewerDrop()
			vw.scaled, vw.sw, vw.sh = thumbOf(vw.path, pw, ph, dp(4), false), pw, ph
		}
		if vw.scaled != 0 {
			g.image(vw.scaled, vw.pic.left, vw.pic.top, pw, ph)
		}
	}
	for id := 0; id < vbCount; id++ {
		if !viewerHas(id) {
			continue
		}
		r := vw.btn[id]
		w, h := r.right-r.left, r.bottom-r.top
		switch {
		case id == vbPrev || id == vbNext:
			fill := mix(viewerInk, viewerGround, 10)
			if vw.hot == id {
				fill = mix(viewerInk, viewerGround, 22)
			}
			g.ellipse(r.left, r.top, w, h, fill)
		case id == vbClose:
			if vw.hot == id {
				g.round(r.left, r.top, w, h, dp(6), dp(6), dp(6), dp(6), hotFill)
			}
		default:
			g.round(r.left, r.top, w, h, dp(6), dp(6), dp(6), dp(6), edge)
			fill := uint32(viewerGround)
			if vw.hot == id {
				fill = hotFill
			}
			g.round(r.left+1, r.top+1, w-2, h-2, dp(5), dp(5), dp(5), dp(5), fill)
		}
	}
	g.free()

	// text
	m := vw.list[vw.idx]
	who := senderName(m)
	if m.SenderID == mv.self && mv.self != 0 {
		who = "Ty"
	}
	when := m.Timestamp.Format("2.01.2006, 15:04")
	if len(vw.list) > 1 {
		when += fmt.Sprintf("  ·  %d z %d", vw.idx+1, len(vw.list))
	}
	textRight := vw.btn[vbSave].left - dp(16)
	c.font(fnt.bodySemi)
	c.text(who, rect{dp(20), dp(9), textRight, dp(9) + sp(20)}, viewerInk, dtSingleLine|dtVCenter|dtEndEllipsis)
	c.font(fnt.small)
	c.text(when, rect{dp(20), dp(9) + sp(20), textRight, dp(9) + sp(38)}, viewerInk2, dtSingleLine|dtVCenter|dtEndEllipsis)
	for id := 0; id < vbCount; id++ {
		if !viewerHas(id) {
			continue
		}
		r := vw.btn[id]
		switch id {
		case vbClose:
			c.font(fnt.icon)
			c.text(gCloseBig, r, viewerInk, dtCenter|dtVCenter|dtSingleLine)
		case vbPrev, vbNext:
			glyph := gPrev
			if id == vbNext {
				glyph = gNext
			}
			c.font(fnt.icon)
			c.text(glyph, r, viewerInk, dtCenter|dtVCenter|dtSingleLine)
		default:
			glyph, label := gDownload, "Pobierz"
			if id == vbOpen {
				glyph, label = gOpenOut, "Otwórz w Windows"
			}
			c.font(fnt.icon)
			c.text(glyph, rect{r.left + dp(14), r.top, r.left + dp(34), r.bottom}, viewerInk, dtSingleLine|dtVCenter)
			c.font(fnt.body)
			c.text(label, rect{r.left + dp(40), r.top, r.right - dp(10), r.bottom}, viewerInk, dtSingleLine|dtVCenter)
		}
	}
	if msg := viewerMessage(); msg != "" {
		c.font(fnt.body)
		c.text(msg, rect{0, dp(56), vw.w, vw.h}, viewerInk2, dtCenter|dtVCenter|dtSingleLine)
	}
	if vw.note != "" {
		c.font(fnt.small)
		c.text(vw.note, rect{dp(20), vw.h - dp(34), vw.w - dp(20), vw.h - dp(8)}, viewerInk2, dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
	}
	buf.flush(0, 0)
}

func viewerHit(x, y int32) int {
	for id := 0; id < vbCount; id++ {
		if r := vw.btn[id]; viewerHas(id) && x >= r.left && x < r.right && y >= r.top && y < r.bottom {
			return id
		}
	}
	return -1
}

func viewerAct(id int) {
	switch id {
	case vbClose:
		closeViewer()
	case vbPrev:
		viewerStep(-1)
	case vbNext:
		viewerStep(1)
	case vbOpen:
		if vw.video != "" {
			openURL(vw.video)
		} else if p, ok := mediaFile(vw.list[vw.idx]); ok {
			safeOpen(p)
		}
	case vbSave:
		viewerSave()
	}
}

// viewerSave asks where to put a copy of the picture and writes it there.
func viewerSave() {
	if vw.video != "" || vw.idx >= len(vw.list) {
		return
	}
	m := vw.list[vw.idx]
	src, ok := mediaFile(m)
	if !ok {
		return
	}
	ext := strings.ToLower(filepath.Ext(src))
	name := m.MediaName
	if name == "" || strings.ContainsAny(name, `\/:*?"<>|`) {
		name = "zdjecie-" + m.Timestamp.Format("2006-01-02-150405")
	}
	if !strings.EqualFold(filepath.Ext(name), ext) {
		name = strings.TrimSuffix(name, filepath.Ext(name)) + ext
	}
	buf := make([]uint16, 1024)
	copy(buf, syscall.StringToUTF16(name))
	filter := utf16.Encode([]rune("Zdjęcie (*" + ext + ")\x00*" + ext + "\x00Wszystkie pliki\x00*.*\x00\x00"))
	defExt := syscall.StringToUTF16(strings.TrimPrefix(ext, "."))
	ofn := openFileName{owner: vw.host, filter: &filter[0], file: &buf[0], maxFile: uint32(len(buf)),
		title: u16("Zapisz zdjęcie"), defExt: &defExt[0], flags: 0x00080000 | 0x00000800 | 0x00000008 | 0x00000002}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))
	dst := ""
	if dryOpen { // checks: no dialog, a fixed place
		dst = filepath.Join(dataDir, "zapisane-"+name)
	} else if r, _, _ := pGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); r != 0 {
		dst = syscall.UTF16ToString(buf)
	}
	if dst == "" {
		return
	}
	data, err := os.ReadFile(src)
	if err == nil {
		err = os.WriteFile(dst, data, 0o644)
	}
	if err != nil {
		vw.note = "Nie udało się zapisać: " + err.Error()
	} else {
		vw.note = "Zapisano: " + dst
	}
	invalidate(vw.hwnd)
	pSetFocus.Call(vw.hwnd)
}

func viewerProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if vw.hwnd == hwnd && vw.bank != nil && vw.bank != curBank {
		defer enterBank(vw.bank)()
	}
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if vw.hwnd == hwnd && vw.video != "" {
			paintVideo(hdc)
		} else if vw.hwnd == hwnd && len(vw.list) > 0 {
			paintViewer(hdc)
		}
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSize:
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		vw.w, vw.h = rc.right, rc.bottom
		if vw.web != nil {
			vw.web.SetBounds(0, dp(56), vw.w, vw.h)
		}
		invalidate(hwnd)
		return 0
	case wmMouseMove:
		x, y := lpPoint(lp)
		if hot := viewerHit(x, y); hot != vw.hot {
			vw.hot = hot
			invalidate(hwnd)
		}
		return 0
	case wmSetCursor:
		if lp&0xFFFF == 1 && vw.hot >= 0 {
			cur, _, _ := pLoadCursorW.Call(0, idcHand)
			pSetCursor.Call(cur)
			return 1
		}
	case wmLButtonDown:
		pSetFocus.Call(hwnd)
		return 0
	case wmLButtonUp:
		x, y := lpPoint(lp)
		if id := viewerHit(x, y); id >= 0 {
			viewerAct(id)
		} else if p := vw.pic; vw.video == "" && !(x >= p.left && x < p.right && y >= p.top && y < p.bottom) && y > dp(56) {
			closeViewer() // a click on the dark ground closes, like in other viewers
		}
		return 0
	case wmMouseWheel:
		if int16(wp>>16) > 0 {
			viewerStep(-1)
		} else {
			viewerStep(1)
		}
		return 0
	case wmGetDlgCode:
		return 0x0004 // every key comes here
	case wmKeyDown:
		switch {
		case wp == vkEscape:
			closeViewer()
		case wp == vkLeft:
			viewerStep(-1)
		case wp == vkRight:
			viewerStep(1)
		case wp == 'S' && keyDown(vkControl):
			viewerSave()
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

// ---- a video page played inside the window

// openVideo plays a video page in a browser embedded in the window, the way a picture is
// shown large. Without such a browser on this computer the page opens in a browser window.
func openVideo(u string) {
	pu, err := url.Parse(u)
	if err != nil || pu.Scheme != "https" || strings.ContainsAny(u, "\" ") {
		return
	}
	if dryOpen || !webview.Available() {
		playExternal(u)
		return
	}
	play := u // the video alone where its service has a player for it; else the whole page
	if e := embedURL(u); e != "" {
		play = e
		if id := youtubeID(u); id != "" {
			// YouTube refuses to play when its player is opened as a page of its own
			// (error 153), so it goes into a frame on a small page written here.
			if p, err := playerPage(id); err != nil {
				log.Warn().Err(err).Msg("player page")
				play = u
			} else {
				play = p
			}
		}
	}
	closeViewer()
	vw.video, vw.webMsg = u, "Ładowanie odtwarzacza…"
	vw.list, vw.hot, vw.note = nil, -1, ""
	vw.host, vw.bank = hostWnd(), curBank
	vw.hwnd = createWindow(0, "MliteViewer", "Odtwarzacz", wsChild|wsVisible|wsTabStop|wsClipChildren, vw.host, 0)
	hostLayout()
	pSetFocus.Call(vw.hwnd)
	failed := func(err error) {
		log.Warn().Err(err).Msg("video player")
		if vw.video == u {
			vw.webMsg = "Nie udało się uruchomić odtwarzacza. Użyj przycisku „Otwórz w przeglądarce”."
			invalidate(vw.hwnd)
		}
	}
	view, err := webview.Open(vw.hwnd, [4]int32{0, dp(56), vw.w, vw.h}, filepath.Join(dataDir, "webview"), play, videoCookies(u), func(err error) {
		if err != nil {
			failed(err)
		}
	})
	if err != nil {
		failed(err)
		return
	}
	vw.web = view
}

// videoCookies hands the embedded browser the Facebook session this program is signed in
// with, so that Facebook videos play as for a signed-in person. Only for Facebook's own
// addresses, and only while the setting allows it.
func videoCookies(u string) []webview.Cookie {
	host := linkHost(u)
	if !cfg.VideoSession || !(host == "facebook.com" || strings.HasSuffix(host, ".facebook.com") || host == "fb.watch") {
		return nil
	}
	creds, err := session.LoadCookies(dataDir)
	if err != nil || creds == nil || creds.Platform != "facebook" {
		return nil
	}
	var out []webview.Cookie
	for name, value := range creds.Values {
		if name == "" || value == "" {
			continue
		}
		out = append(out, webview.Cookie{Name: name, Value: value, Domain: ".facebook.com", Path: "/", Secure: true, HTTPOnly: name == "xs" || name == "datr" || name == "sb" || name == "fr"})
	}
	return out
}

func paintVideo(target uintptr) {
	buf := newBuffer(target, vw.w, vw.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, vw.w, vw.h}, viewerGround)
	bar, pad, s := dp(56), dp(12), dp(36)
	x := vw.w - pad
	vw.btn[vbClose] = rect{x - s, (bar - s) / 2, x, (bar + s) / 2}
	x -= s + dp(8)
	c.font(fnt.body)
	label := "Otwórz w przeglądarce"
	w := dp(14) + dp(20) + dp(6) + c.width(label) + dp(14)
	vw.btn[vbOpen] = rect{x - w, (bar - s) / 2, x, (bar + s) / 2}
	hotFill := mix(viewerInk, viewerGround, 16)
	g := newGfx(buf.hdc)
	if r := vw.btn[vbClose]; vw.hot == vbClose {
		g.round(r.left, r.top, s, s, dp(6), dp(6), dp(6), dp(6), hotFill)
	}
	r := vw.btn[vbOpen]
	g.round(r.left, r.top, r.right-r.left, s, dp(6), dp(6), dp(6), dp(6), mix(viewerInk, viewerGround, 26))
	fill := uint32(viewerGround)
	if vw.hot == vbOpen {
		fill = hotFill
	}
	g.round(r.left+1, r.top+1, r.right-r.left-2, s-2, dp(5), dp(5), dp(5), dp(5), fill)
	g.free()
	textRight := r.left - dp(16)
	c.font(fnt.bodySemi)
	c.text("Wideo · "+linkHost(vw.video), rect{dp(20), dp(9), textRight, dp(9) + sp(20)}, viewerInk, dtSingleLine|dtVCenter|dtEndEllipsis)
	c.font(fnt.small)
	c.text(strings.TrimPrefix(vw.video, "https://"), rect{dp(20), dp(9) + sp(20), textRight, dp(9) + sp(38)}, viewerInk2, dtSingleLine|dtVCenter|dtEndEllipsis)
	c.font(fnt.icon)
	c.text(gCloseBig, vw.btn[vbClose], viewerInk, dtCenter|dtVCenter|dtSingleLine)
	c.text(gOpenOut, rect{r.left + dp(14), r.top, r.left + dp(34), r.bottom}, viewerInk, dtSingleLine|dtVCenter)
	c.font(fnt.body)
	c.text(label, rect{r.left + dp(40), r.top, r.right - dp(10), r.bottom}, viewerInk, dtSingleLine|dtVCenter)
	c.text(vw.webMsg, rect{dp(24), bar, vw.w - dp(24), vw.h}, viewerInk2, dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
	buf.flush(0, 0)
}
