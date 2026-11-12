//go:build windows

package main

// Signing in inside the window. The core walks through Meta's own steps (name and password,
// a code, approval on another device); this file shows each step and hands the answers back.
// What is typed here goes to Meta only; the computer keeps nothing but the resulting session.

import (
	"errors"
	"strings"
	"unsafe"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/session"
)

const (
	wmAppLogin     = wmApp + 7
	wmAppLoginStep = wmApp + 8

	idLoginNext   = 340
	idLoginCancel = 341
	idLoginField  = 350

	esPassword        = 0x0020
	wsExControlParent = 0x00010000

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
	login          *meta.Login
	step           meta.LoginStep
	started, busy  bool
	err            string
	edits          []uintptr
	hNext, hCancel uintptr
	results        chan loginResult
	w, h           int32
	// laid out by loginLayout, used by the painting
	card, headR, textR, errR rect
	labelR, fieldR           []rect
	text                     string
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
	for _, e := range lg.edits {
		pDestroyWindow.Call(e)
	}
	lg.edits = nil
	lg.active = false
	layout()
}

func fieldLabel(f meta.LoginField) string {
	name := strings.ToLower(f.Name + " " + f.ID)
	switch {
	case strings.Contains(name, "password"):
		return "Hasło"
	case strings.Contains(name, "code") || strings.Contains(name, "otp"):
		return "Kod weryfikacyjny"
	case strings.Contains(name, "user") || strings.Contains(name, "email") || strings.Contains(name, "phone"):
		return "E-mail lub numer telefonu"
	}
	return f.Name
}

// loginFields rebuilds the text fields for the current step.
func loginFields() {
	for _, e := range lg.edits {
		pDestroyWindow.Call(e)
	}
	lg.edits = nil
	if lg.step.Kind == meta.LoginInput && lg.started {
		for i, f := range lg.step.Fields {
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
	if len(lg.edits) > 0 {
		pSetFocus.Call(lg.edits[0])
	}
}

func loginText() string {
	switch {
	case lg.busy:
		return "Łączenie z Messengerem…"
	case !lg.started && lg.expired:
		return "Poprzednia sesja wygasła. Zaloguj się ponownie, a rozmowy wrócą na swoje miejsce."
	case !lg.started:
		return "Program łączy się z Messengerem tak jak aplikacja Messenger Lite. To, co wpiszesz, trafia wyłącznie do Mety; na tym komputerze zostaje zapisana sama sesja."
	case lg.step.Kind == meta.LoginWait:
		return "Potwierdź logowanie na innym urządzeniu, na którym masz Facebooka. Ten ekran sam przejdzie dalej."
	case lg.step.Kind == meta.LoginUnsupported:
		return "To logowanie wymaga dodatkowej strony w przeglądarce, której program nie potrafi pokazać. Dokończ je w przeglądarce lub w oficjalnej aplikacji i spróbuj ponownie."
	case lg.step.Instructions != "":
		return lg.step.Instructions
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
	fieldH, labelH := dp(36), sp(20)
	total := in + sp(34) + dp(8) + th + dp(20) + int32(len(lg.edits))*(labelH+fieldH+dp(14)) + dp(36) + in
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
	lg.labelR, lg.fieldR = nil, nil
	for _, e := range lg.edits {
		lg.labelR = append(lg.labelR, rect{x + in, cy, x + cw - in, cy + labelH})
		cy += labelH
		fr := rect{x + in, cy, x + cw - in, cy + fieldH}
		lg.fieldR = append(lg.fieldR, fr)
		moveTo(e, rect{fr.left + dp(10), fr.top + (fieldH-lay.editH)/2, fr.right - dp(10), fr.top + (fieldH-lay.editH)/2 + lay.editH})
		cy += fieldH + dp(14)
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
	show(lg.hCancel, lg.started && !lg.busy)
	invalidate(lg.hwnd)
	invalidate(lg.hNext)
}

// loginNext starts signing in or answers the current step.
func loginNext() {
	if lg.busy {
		return
	}
	lg.err = ""
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
		if i < len(lg.edits) {
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

// loginBack returns to the first screen.
func loginBack() {
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
	return "Logowanie się nie udało: " + err.Error()
}

// loginStepArrived runs on the window thread when the core answered.
func loginStepArrived() {
	for {
		select {
		case r := <-lg.results:
			lg.busy = false
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

func paintLogin(target uintptr) {
	buf := newBuffer(target, lg.w, lg.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, lg.w, lg.h}, pal.layer)
	g := newGfx(buf.hdc)
	k := lg.card
	g.round(k.left, k.top+dp(2), k.right-k.left, k.bottom-k.top, dp(8), dp(8), dp(8), dp(8), mix(0x000000, pal.layer, 14))
	g.round(k.left, k.top, k.right-k.left, k.bottom-k.top, dp(8), dp(8), dp(8), dp(8), pal.card)
	focus, _, _ := pGetFocus.Call()
	for i, r := range lg.fieldR {
		edge := pal.fg2
		inset := int32(1)
		if i < len(lg.edits) && lg.edits[i] == focus {
			edge, inset = pal.accent, 2
		}
		g.round(r.left, r.top, r.right-r.left, r.bottom-r.top, dp(4), dp(4), dp(4), dp(4), mix(pal.fg, pal.card, 14))
		g.round(r.left, r.bottom-dp(5), r.right-r.left, dp(5), 1, 1, dp(4), dp(4), edge)
		g.round(r.left+1, r.top+1, r.right-r.left-2, r.bottom-r.top-1-inset, dp(3), dp(3), dp(3), dp(3), pal.card)
	}
	g.free()
	c.font(fnt.title)
	c.text("Zaloguj się do Messengera", lg.headR, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
	c.font(fnt.body)
	c.text(lg.text, lg.textR, pal.fg2, dtWordBreak)
	c.font(fnt.small)
	for i, r := range lg.labelR {
		if i < len(lg.step.Fields) {
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
		case idLoginCancel:
			drawLoginButton(d, "Wróć", false)
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
		case id >= idLoginField && (code == enSetFocus || code == enKillFocus):
			invalidate(hwnd)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
