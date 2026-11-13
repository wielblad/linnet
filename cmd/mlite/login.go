//go:build windows

package main

// Signing in inside the window. The core walks through Meta's own steps (name and password,
// a code, approval on another device); this file shows each step and hands the answers back.
// What is typed here goes to Meta only; the computer keeps nothing but the resulting session.

import (
	"errors"
	"math"
	"strings"
	"unsafe"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/session"
)

const (
	wmAppLogin     = wmApp + 7
	wmAppLoginStep = wmApp + 8
	wmAppHistory   = wmApp + 13

	idLoginNext   = 340
	idLoginCancel = 341
	idLoginField  = 350
	idLoginOption = 370 // the choices of a field with options, one button each

	esPassword        = 0x0020
	wsExControlParent = 0x00010000

	timerLoginSpin = 1

	loginExpired = 1
)

type loginResult struct {
	step meta.LoginStep
	err  error
}

var lg struct {
	hwnd           uintptr
	active         bool
	expired        bool
	pin            bool // the screen asks for the PIN of the encrypted chat history, not a login
	checking       bool // on the PIN card: asking Meta whether there is a backup at all
	info           bool // the card only tells something; one button closes it
	login          *meta.Login
	step           meta.LoginStep
	started, busy  bool
	err            string
	edits          []uintptr // one per field of the step; 0 for a field answered by choices
	opts           []loginOpt
	picked         map[int]string // field index -> chosen option
	spin           int            // frame of the waiting dots
	hNext, hCancel uintptr
	results        chan loginResult
	w, h           int32
	// laid out by loginLayout, used by the painting
	card, headR, textR, errR rect
	labelR, fieldR           []rect
	spinR                    rect
	text                     string
}

// loginOpt is one choice of a field with options, shown as a tile of its own.
type loginOpt struct {
	hwnd  uintptr
	field int
	value string
}

func initLogin() {
	lg.results = make(chan loginResult, 4)
	lg.hwnd = createWindow(wsExControlParent, "MliteLogin", "Logowanie", wsChild|wsClipChildren, hMain, 0)
	lg.hNext = createWindow(0, "BUTTON", "Dalej", wsChild|wsVisible|wsTabStop|bsOwnerDraw, lg.hwnd, idLoginNext)
	lg.hCancel = createWindow(0, "BUTTON", "Wróć", wsChild|wsTabStop|bsOwnerDraw, lg.hwnd, idLoginCancel)
	for _, h := range []uintptr{lg.hNext, lg.hCancel} {
		oldBtnProc = subclass(h, btnProc)
	}
}

// showLogin puts the sign-in screen over the window.
func showLogin(expired bool) {
	if lg.active {
		return
	}
	lg.active, lg.expired, lg.started, lg.busy, lg.err = true, expired, false, false, ""
	lg.step = meta.LoginStep{}
	loginFields()
	layout()
}

func loginClose() {
	if lg.login != nil {
		lg.login.Close()
		lg.login = nil
	}
	loginClearControls()
	lg.active, lg.pin, lg.busy, lg.checking, lg.info = false, false, false, false, false
	pKillTimer.Call(lg.hwnd, timerLoginSpin)
	layout()
}

func loginClearControls() {
	for _, e := range lg.edits {
		if e != 0 {
			pDestroyWindow.Call(e)
		}
	}
	for _, o := range lg.opts {
		pDestroyWindow.Call(o.hwnd)
	}
	lg.edits, lg.opts, lg.picked = nil, nil, map[int]string{}
}

// showHistoryPIN asks for the PIN that unlocks the history of end-to-end encrypted chats.
// Without it those chats show only what arrived after signing in.
func showHistoryPIN() {
	if lg.active {
		return
	}
	lg.active, lg.pin, lg.expired, lg.started, lg.busy, lg.err = true, true, false, true, false, ""
	lg.step = meta.LoginStep{
		Kind:         meta.LoginInput,
		Instructions: "Czaty szyfrowane end-to-end mają historię w kopii zapasowej Messengera. Podaj sześciocyfrowy PIN tej kopii, aby ją wczytać, albo pomiń — wtedy zobaczysz tylko nowe wiadomości.",
		Fields:       []meta.LoginField{{ID: "pin", Name: "PIN", Secret: true}},
	}
	loginFields()
	layout()
}

// historyCheck opens the PIN card at once, showing that it asks Meta about the backup.
func historyCheck() {
	showHistoryPIN()
	if !lg.pin {
		return // a sign-in is on the screen
	}
	lg.busy, lg.checking = true, true
	loginLayout()
	go be.offerHistory()
}

// historyChecked runs on the window thread with Meta's answer about the backup.
func historyChecked() {
	var msg string
	select {
	case msg = <-historyChecks:
	default:
		return
	}
	if !lg.pin || !lg.checking {
		return
	}
	lg.busy, lg.checking = false, false
	if msg != "" {
		lg.step = meta.LoginStep{Kind: meta.LoginInput, Instructions: msg}
		lg.info = true
	}
	loginFields()
}

// historyPINNext checks the PIN with Messenger; the core then fetches the history.
func historyPINNext() {
	if len(lg.edits) == 0 {
		return
	}
	pin := strings.TrimSpace(getText(lg.edits[0]))
	setText(lg.edits[0], "")
	if len(pin) != 6 || strings.Trim(pin, "0123456789") != "" {
		lg.err = "PIN ma sześć cyfr."
		loginLayout()
		return
	}
	client := be.current()
	if client == nil {
		lg.err = "Program nie jest połączony z Messengerem."
		loginLayout()
		return
	}
	lg.busy = true
	loginLayout()
	go func() {
		err := client.UnlockHistory(be.ctx, pin)
		lg.results <- loginResult{meta.LoginStep{Kind: meta.LoginDone}, err}
		postMsg(hMain, wmAppLoginStep, 0, 0)
	}()
}

func fieldLabel(f meta.LoginField) string {
	name := strings.ToLower(f.Name + " " + f.ID)
	switch {
	case lg.pin && f.ID == "pin":
		return "PIN kopii zapasowej czatów"
	case strings.Contains(name, "mfa") || strings.Contains(name, "method"):
		return "Sposób potwierdzenia logowania"
	case strings.Contains(name, "captcha"):
		return "Znaki z obrazka"
	case strings.Contains(name, "backup"):
		return "Kod zapasowy"
	case strings.Contains(name, "password"):
		return "Hasło"
	case strings.Contains(name, "code") || strings.Contains(name, "otp"):
		return "Kod weryfikacyjny"
	case strings.Contains(name, "user") || strings.Contains(name, "email") || strings.Contains(name, "phone"):
		return "E-mail lub numer telefonu"
	}
	return f.Name
}

// loginFields rebuilds the fields for the current step: a text field each, or a column of
// tiles when Meta offers a fixed set of answers (such as the way to confirm the login).
func loginFields() {
	loginClearControls()
	if lg.step.Kind == meta.LoginInput && lg.started {
		for i, f := range lg.step.Fields {
			if len(f.Options) > 0 {
				for _, o := range f.Options {
					h := createWindow(0, "BUTTON", optionLabel(o), wsChild|wsVisible|wsTabStop|bsOwnerDraw, lg.hwnd, uintptr(idLoginOption+len(lg.opts)))
					oldBtnProc = subclass(h, btnProc)
					lg.opts = append(lg.opts, loginOpt{h, i, o})
				}
				lg.edits = append(lg.edits, 0)
				continue
			}
			style := uint32(wsChild | wsVisible | wsTabStop | esAutoHScroll)
			if f.Secret {
				style |= esPassword
			}
			e := createWindow(0, "EDIT", "", style, lg.hwnd, uintptr(idLoginField+i))
			sendMsg(e, wmSetFont, fnt.body, 1)
			lg.edits = append(lg.edits, e)
		}
	}
	loginLayout()
	for _, e := range lg.edits {
		if e != 0 {
			pSetFocus.Call(e)
			return
		}
	}
	if len(lg.opts) > 0 {
		pSetFocus.Call(lg.opts[0].hwnd)
	}
}

// loginOnlyChoices reports whether every field of the step is answered by a tile, so that a
// click on a tile answers the step and the "Dalej" button has nothing to do.
func loginOnlyChoices() bool {
	if lg.step.Kind != meta.LoginInput || len(lg.step.Fields) == 0 {
		return false
	}
	for _, f := range lg.step.Fields {
		if len(f.Options) == 0 {
			return false
		}
	}
	return true
}

// loginWaiting is the state in which the card shows the turning dots instead of fields.
func loginWaiting() bool {
	return lg.busy || (lg.started && lg.step.Kind == meta.LoginWait)
}

func loginChoose(j int) {
	if lg.busy || j < 0 || j >= len(lg.opts) {
		return
	}
	o := lg.opts[j]
	lg.picked[o.field] = o.value
	if loginOnlyChoices() {
		loginNext()
		return
	}
	for _, p := range lg.opts {
		invalidate(p.hwnd)
	}
}

func loginText() string {
	switch {
	case lg.busy && lg.checking:
		return "Sprawdzanie, czy konto ma kopię historii…"
	case lg.busy && lg.pin:
		return "Sprawdzanie PIN-u…"
	case lg.busy && lg.started && lg.login != nil:
		return "Logowanie…"
	case lg.busy:
		return "Łączenie z Messengerem…"
	case !lg.started && lg.expired:
		return "Sesja wygasła. Zaloguj się ponownie, aby kontynuować."
	case !lg.started:
		return "Program łączy się z Messengerem tak jak aplikacja Messenger Lite. To, co wpiszesz, trafia wyłącznie do Mety; na tym komputerze zostaje zapisana sama sesja."
	case lg.step.Kind == meta.LoginWait:
		return "Potwierdź logowanie na innym urządzeniu, na którym masz Facebooka. Ten ekran sam przejdzie dalej."
	case lg.step.Kind == meta.LoginUnsupported:
		return "To logowanie wymaga dodatkowej strony w przeglądarce, której program nie potrafi pokazać. Dokończ je w przeglądarce lub w oficjalnej aplikacji i spróbuj ponownie."
	case lg.step.Instructions != "":
		return plLogin(lg.step.Instructions)
	}
	return "Podaj dane swojego konta."
}

func loginLayout() {
	if lg.hwnd == 0 {
		return
	}
	cw := dp(420)
	if cw > lg.w-dp(32) {
		cw = lg.w - dp(32)
	}
	x := (lg.w - cw) / 2
	in := dp(28)
	lg.text = loginText()
	var th, eh int32
	withFont(fnt.body, func(c canvas) { _, th = c.measure(lg.text, cw-2*in) })
	if lg.err != "" {
		withFont(fnt.small, func(c canvas) { _, eh = c.measure(lg.err, cw-2*in) })
	}
	fieldH, labelH, optH := dp(36), sp(20), dp(56)
	waiting := loginWaiting()
	body := int32(0)
	if waiting {
		body = dp(56)
	} else {
		for i := range lg.edits {
			body += labelH + dp(14)
			if n := loginOptCount(i); n > 0 {
				body += int32(n)*(optH+dp(8)) - dp(8)
			} else {
				body += fieldH
			}
		}
	}
	buttons := dp(36)
	if waiting && !(lg.started && !lg.busy) {
		buttons = 0 // nothing to press while the core works
	}
	total := in + sp(34) + dp(8) + th + dp(20) + body + buttons + in
	if eh > 0 {
		total += eh + dp(12)
	}
	y := (lg.h - total) / 2
	if y < dp(16) {
		y = dp(16)
	}
	lg.card = rect{x, y, x + cw, y + total}
	cy := y + in
	lg.headR = rect{x + in, cy, x + cw - in, cy + sp(34)}
	cy += sp(34) + dp(8)
	lg.textR = rect{x + in, cy, x + cw - in, cy + th}
	cy += th + dp(20)
	lg.labelR, lg.fieldR, lg.spinR = nil, nil, rect{}
	for _, e := range lg.edits {
		show(e, !waiting)
	}
	for _, o := range lg.opts {
		show(o.hwnd, !waiting)
	}
	if waiting {
		lg.spinR = rect{x + in, cy, x + cw - in, cy + dp(40)}
		cy += dp(56)
		pSetTimer.Call(lg.hwnd, timerLoginSpin, 90, 0)
	} else {
		pKillTimer.Call(lg.hwnd, timerLoginSpin)
		k := 0
		for i, e := range lg.edits {
			lg.labelR = append(lg.labelR, rect{x + in, cy, x + cw - in, cy + labelH})
			cy += labelH
			if n := loginOptCount(i); n > 0 {
				lg.fieldR = append(lg.fieldR, rect{}) // tiles draw their own edges
				for ; k < len(lg.opts) && lg.opts[k].field == i; k++ {
					moveTo(lg.opts[k].hwnd, rect{x + in, cy, x + cw - in, cy + optH})
					cy += optH + dp(8)
				}
				cy += dp(14) - dp(8)
				continue
			}
			fr := rect{x + in, cy, x + cw - in, cy + fieldH}
			lg.fieldR = append(lg.fieldR, fr)
			moveTo(e, rect{fr.left + dp(10), fr.top + (fieldH-lay.editH)/2, fr.right - dp(10), fr.top + (fieldH-lay.editH)/2 + lay.editH})
			cy += fieldH + dp(14)
		}
	}
	lg.errR = rect{}
	if eh > 0 {
		lg.errR = rect{x + in, cy, x + cw - in, cy + eh}
		cy += eh + dp(12)
	}
	nextW := dp(140)
	moveTo(lg.hNext, rect{x + cw - in - nextW, cy, x + cw - in, cy + dp(36)})
	moveTo(lg.hCancel, rect{x + in, cy, x + in + dp(96), cy + dp(36)})
	canNext := !lg.busy && lg.step.Kind != meta.LoginWait && !(lg.started && lg.step.Kind == meta.LoginUnsupported)
	on := uintptr(0)
	if canNext {
		on = 1
	}
	pEnableWindow.Call(lg.hNext, on)
	show(lg.hNext, !waiting && !loginOnlyChoices() && !lg.info)
	show(lg.hCancel, lg.started && !lg.busy)
	invalidate(lg.hwnd)
	invalidate(lg.hNext)
}

// loginOptCount is the number of tiles of the i-th field of the step, 0 for a text field.
func loginOptCount(i int) int {
	if lg.step.Kind != meta.LoginInput || i >= len(lg.step.Fields) {
		return 0
	}
	return len(lg.step.Fields[i].Options)
}

// loginNext starts signing in or answers the current step.
func loginNext() {
	if lg.busy {
		return
	}
	lg.err = ""
	if lg.pin {
		historyPINNext()
		return
	}
	if !lg.started {
		lg.started, lg.busy = true, true
		loginLayout()
		go func() {
			l, step, err := meta.StartLogin(be.ctx, be.logW)
			if err == nil {
				lg.login = l
			}
			lg.results <- loginResult{step, err}
			postMsg(hMain, wmAppLoginStep, 0, 0)
		}()
		return
	}
	if lg.login == nil || lg.step.Kind != meta.LoginInput {
		return
	}
	input := map[string]string{}
	for i, f := range lg.step.Fields {
		if len(f.Options) > 0 {
			v, ok := lg.picked[i]
			if !ok {
				lg.err = "Wybierz jedną z możliwości."
				loginLayout()
				return
			}
			input[f.ID] = v
			continue
		}
		if i < len(lg.edits) && lg.edits[i] != 0 {
			input[f.ID] = getText(lg.edits[i])
			if f.Secret {
				setText(lg.edits[i], "") // a secret does not stay in the field
			}
		}
	}
	lg.busy = true
	loginLayout()
	l := lg.login
	go func() {
		step, err := l.Submit(be.ctx, input)
		lg.results <- loginResult{step, err}
		postMsg(hMain, wmAppLoginStep, 0, 0)
	}()
}

// loginBack returns to the first screen; on the PIN screen it skips the history.
func loginBack() {
	if lg.pin {
		loginClose()
		return
	}
	if lg.login != nil {
		lg.login.Close()
		lg.login = nil
	}
	lg.started, lg.busy, lg.err = false, false, ""
	lg.step = meta.LoginStep{}
	loginFields()
}

func loginError(err error) string {
	switch {
	case errors.Is(err, meta.ErrLoginCheckpoint):
		return "To konto wymaga sprawdzenia bezpieczeństwa. Otwórz Facebooka w przeglądarce albo w oficjalnej aplikacji, dokończ je i spróbuj ponownie."
	}
	msg := err.Error()
	for en, pl := range plLoginText {
		if strings.Contains(msg, en) {
			return "Logowanie się nie udało. " + pl
		}
	}
	return "Logowanie się nie udało: " + msg
}

// loginStepArrived runs on the window thread when the core answered.
func loginStepArrived() {
	for {
		select {
		case r := <-lg.results:
			lg.busy = false
			if lg.pin {
				if r.err != nil {
					lg.err = "Nie udało się wczytać historii: " + r.err.Error()
					loginFields()
					continue
				}
				loginClose()
				go be.syncHistory()
				continue
			}
			if r.err != nil {
				lg.err = loginError(r.err)
				if lg.login == nil {
					lg.started = false
				}
				loginFields()
				continue
			}
			lg.step = r.step
			switch r.step.Kind {
			case meta.LoginDone:
				if r.step.Credentials != nil {
					if err := session.SaveCookies(dataDir, r.step.Credentials); err != nil {
						lg.err = "Nie można zapisać sesji: " + err.Error()
						loginFields()
						continue
					}
				}
				loginClose()
				go be.run()
			case meta.LoginWait:
				loginFields()
				l := lg.login
				go func() {
					step, err := l.Wait(be.ctx)
					lg.results <- loginResult{step, err}
					postMsg(hMain, wmAppLoginStep, 0, 0)
				}()
			default:
				loginFields()
			}
			continue
		default:
		}
		return
	}
}

func drawLoginButton(d *drawItem, label string, primary bool) {
	w, h := d.rcItem.right, d.rcItem.bottom
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.card)
	fill, ink := pal.accent, pal.onAccent
	if !primary {
		fill, ink = pal.card, pal.fg
	}
	switch {
	case d.itemState&odsDisabled != 0:
		fill, ink = mix(pal.fg, pal.card, 8), pal.fg2
	case d.itemState&odsSelected != 0:
		fill = mix(fill, pal.fg, 86)
	case hotBtn == d.hwndItem:
		fill = mix(fill, pal.fg, 93)
	}
	g := newGfx(buf.hdc)
	if !primary {
		g.round(0, 0, w, h, dp(4), dp(4), dp(4), dp(4), mix(pal.fg, pal.card, 22))
		g.round(1, 1, w-2, h-2, dp(3), dp(3), dp(3), dp(3), fill)
	} else {
		g.round(0, 0, w, h, dp(4), dp(4), dp(4), dp(4), fill)
	}
	g.free()
	c.font(fnt.bodySemi)
	c.text(label, rect{0, 0, w, h}, ink, dtCenter|dtVCenter|dtSingleLine)
	if focusCue(d) {
		fr := rect{3, 3, w - 3, h - 3}
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(0, 0)
}

// drawLoginOption draws one choice as a tile: its name, a line about it, an arrow.
func drawLoginOption(d *drawItem, o loginOpt) {
	w, h := d.rcItem.right, d.rcItem.bottom
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.card)
	edge, fill := mix(pal.fg, pal.card, 22), pal.card
	picked := lg.picked[o.field] == o.value
	switch {
	case picked:
		edge, fill = pal.accent, mix(pal.accent, pal.card, 10)
	case d.itemState&odsSelected != 0:
		fill = mix(pal.fg, pal.card, 14)
	case hotBtn == d.hwndItem:
		edge, fill = mix(pal.accent, pal.card, 60), mix(pal.fg, pal.card, 6)
	}
	g := newGfx(buf.hdc)
	r := dp(8)
	g.round(0, 0, w, h, r, r, r, r, edge)
	g.round(1, 1, w-2, h-2, r-1, r-1, r-1, r-1, fill)
	g.free()
	pad := dp(16)
	about := optionAbout(o.value)
	c.font(fnt.bodySemi)
	if about == "" {
		c.text(optionLabel(o.value), rect{pad, 0, w - dp(40), h}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
	} else {
		c.text(optionLabel(o.value), rect{pad, dp(8), w - dp(40), h/2 + dp(2)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
		c.font(fnt.small)
		c.text(about, rect{pad, h/2 + dp(2), w - dp(40), h - dp(8)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
	}
	c.font(fnt.title)
	c.text("›", rect{w - dp(36), 0, w - dp(12), h}, pal.fg2, dtSingleLine|dtVCenter|dtCenter)
	if focusCue(d) {
		fr := rect{3, 3, w - 3, h - 3}
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(0, 0)
}

func paintLogin(target uintptr) {
	buf := newBuffer(target, lg.w, lg.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, lg.w, lg.h}, pal.mica)
	g := newGfx(buf.hdc)
	k := lg.card
	g.round(k.left, k.top, k.right-k.left, k.bottom-k.top, dp(12), dp(12), dp(12), dp(12), pal.line)
	g.round(k.left+1, k.top+1, k.right-k.left-2, k.bottom-k.top-2, dp(11), dp(11), dp(11), dp(11), pal.card)
	focus, _, _ := pGetFocus.Call()
	if s := lg.spinR; s.right > s.left {
		// Eight dots on a ring; the brightest one walks round, the others fade behind it.
		cx, cy, rad, dot := (s.left+s.right)/2, (s.top+s.bottom)/2, dp(14), dp(6)
		for k := 0; k < 8; k++ {
			a := float64(k) * math.Pi / 4
			age := (lg.spin - k + 8) % 8
			alpha := uint32(255 - age*28)
			px := cx + int32(float64(rad)*math.Cos(a)) - dot/2
			py := cy + int32(float64(rad)*math.Sin(a)) - dot/2
			g.ellipseA(px, py, dot, dot, alpha<<24|pal.accent&0xFFFFFF)
		}
	}
	for i, r := range lg.fieldR {
		if r.right <= r.left {
			continue
		}
		edge := mix(pal.fg2, pal.line, 65)
		inset := int32(1)
		if i < len(lg.edits) && lg.edits[i] == focus {
			edge, inset = pal.accent, 2
		}
		rr := dp(8)
		g.round(r.left, r.top, r.right-r.left, r.bottom-r.top, rr, rr, rr, rr, edge)
		g.round(r.left+inset, r.top+inset, r.right-r.left-2*inset, r.bottom-r.top-2*inset, rr-inset, rr-inset, rr-inset, rr-inset, pal.card)
	}
	g.free()
	c.font(fnt.title)
	head := "Zaloguj się do Messengera"
	if lg.pin {
		head = "Historia szyfrowanych czatów"
	}
	c.text(head, lg.headR, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
	c.font(fnt.body)
	c.text(lg.text, lg.textR, pal.fg2, dtWordBreak)
	c.font(fnt.small)
	for i, r := range lg.labelR {
		if i < len(lg.step.Fields) && !loginWaiting() {
			c.text(fieldLabel(lg.step.Fields[i]), r, pal.fg, dtSingleLine|dtVCenter)
		}
	}
	if lg.err != "" {
		c.text(lg.err, lg.errR, pal.signal, dtWordBreak)
	}
	buf.flush(0, 0)
}

func loginProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmTimer:
		if wp == timerLoginSpin {
			lg.spin = (lg.spin + 1) % 8
			invalidate(hwnd)
		}
		return 0
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		paintLogin(hdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSize:
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		lg.w, lg.h = rc.right, rc.bottom
		loginLayout()
		return 0
	case wmDrawItem:
		d := (*drawItem)(unsafe.Pointer(lp))
		switch d.ctlID {
		case idLoginNext:
			label := "Dalej"
			if !lg.started {
				label = "Zaloguj się"
			}
			drawLoginButton(d, label, true)
		default:
			if j := int(d.ctlID) - idLoginOption; j >= 0 && j < len(lg.opts) {
				drawLoginOption(d, lg.opts[j])
			}
			return 1
		case idLoginCancel:
			label := "Wróć"
			switch {
			case lg.info:
				label = "Zamknij"
			case lg.pin:
				label = "Pomiń"
			}
			drawLoginButton(d, label, false)
		}
		return 1
	case wmCtlColorEdit:
		pSetTextColor.Call(wp, cref(pal.fg))
		pSetBkColor.Call(wp, cref(pal.card))
		return brCard
	case 0x0135: // WM_CTLCOLORBTN
		return brCard
	case wmCommand:
		id, code := wp&0xFFFF, (wp>>16)&0xFFFF
		switch {
		case id == idLoginNext && code == 0:
			loginNext()
		case id == idLoginCancel && code == 0:
			loginBack()
		case id >= idLoginOption && id < idLoginOption+uintptr(len(lg.opts)) && code == 0:
			loginChoose(int(id - idLoginOption))
		case id >= idLoginField && id < idLoginOption && (code == enSetFocus || code == enKillFocus):
			invalidate(hwnd)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
