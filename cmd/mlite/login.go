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
	idLoginOption = 360 // a choice of one step: idLoginOption + field*optionsPerField + option

	optionsPerField = 8

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
	edits          []uintptr   // one per field of the step; 0 where the field is a choice
	opts           [][]uintptr // one row of buttons per field of a choice, else nil
	choice         []int       // which option of that field is picked
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
	loginDropControls()
	lg.active = false
	layout()
}

// loginDropControls takes away the fields and the choices of the step that was on the screen.
func loginDropControls() {
	for _, e := range lg.edits {
		if e != 0 {
			pDestroyWindow.Call(e)
		}
	}
	for _, row := range lg.opts {
		for _, b := range row {
			pDestroyWindow.Call(b)
		}
	}
	lg.edits, lg.opts, lg.choice = nil, nil, nil
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
	case strings.Contains(name, "method") || strings.Contains(name, "mfatype"):
		return "Sposób potwierdzenia"
	}
	return f.Name
}

// loginPhrase puts into Polish what the core writes in English. What Meta's own page says
// comes in the language of the account, so it is left alone.
func loginPhrase(s string) string {
	switch strings.TrimSpace(s) {
	case "Choose how to finish signing in":
		return "Wybierz, jak potwierdzić logowanie."
	case "Enter your Facebook credentials. The Messenger network will only work with Facebook accounts.":
		return "Podaj dane konta na Facebooku. Messenger działa tylko z kontami Facebooka."
	case "Choose the Facebook account to connect":
		return "Wybierz konto Facebooka, z którym ma się połączyć program."
	case "Sign in with your Google account.":
		return "To konto loguje się przez Google, czego program nie obsługuje. Dodaj inny sposób potwierdzania w ustawieniach Facebooka."
	}
	return s
}

// optionLabel names a way of confirming the login the way a person would say it.
func optionLabel(s string) string {
	n := strings.ToLower(strings.Join(strings.Fields(s), " "))
	switch {
	case strings.Contains(n, "authentication app"), strings.Contains(n, "authenticator app"):
		return "Aplikacja uwierzytelniająca"
	case strings.Contains(n, "text message"), strings.Contains(n, "sms"):
		return "Kod SMS-em"
	case strings.Contains(n, "backup code"):
		return "Kod zapasowy"
	case strings.Contains(n, "whatsapp"):
		return "Kod przez WhatsApp"
	case strings.Contains(n, "another device"):
		return "Potwierdzenie na innym urządzeniu"
	case strings.Contains(n, "notification"):
		return "Powiadomienie w aplikacji Facebook"
	case n == "email", strings.Contains(n, "email code"):
		return "Kod e-mailem"
	}
	return s
}

// loginFields rebuilds what the current step asks for: a text field, or a row of choices
// where the step offers a list (which way to confirm the login, for one).
func loginFields() {
	loginDropControls()
	if lg.step.Kind == meta.LoginInput && lg.started {
		for i, f := range lg.step.Fields {
			lg.edits = append(lg.edits, 0)
			lg.opts = append(lg.opts, nil)
			lg.choice = append(lg.choice, 0)
			if len(f.Options) > 0 {
				for j := range f.Options {
					if j >= optionsPerField {
						break
					}
					b := createWindow(0, "BUTTON", optionLabel(f.Options[j]),
						wsChild|wsVisible|wsTabStop|bsOwnerDraw, lg.hwnd, uintptr(idLoginOption+i*optionsPerField+j))
					oldBtnProc = subclass(b, btnProc)
					lg.opts[i] = append(lg.opts[i], b)
				}
				continue
			}
			style := uint32(wsChild | wsVisible | wsTabStop | esAutoHScroll)
			if f.Secret {
				style |= esPassword
			}
			e := createWindow(0, "EDIT", "", style, lg.hwnd, uintptr(idLoginField+i))
			sendMsg(e, wmSetFont, fnt.body, 1)
			lg.edits[i] = e
		}
	}
	loginLayout()
	for i := range lg.edits {
		if lg.edits[i] != 0 {
			pSetFocus.Call(lg.edits[i])
			return
		}
		if len(lg.opts[i]) > 0 {
			pSetFocus.Call(lg.opts[i][0])
			return
		}
	}
}

func loginText() string {
	switch {
	case lg.busy:
		return "Łączenie z Messengerem… Potrafi to potrwać kilkanaście sekund."
	case !lg.started && lg.expired:
		return "Sesja wygasła. Zaloguj się ponownie, aby kontynuować."
	case !lg.started:
		return "Program łączy się z Messengerem tak jak aplikacja Messenger Lite. To, co wpiszesz, trafia wyłącznie do Mety; na tym komputerze zostaje zapisana sama sesja."
	case lg.step.Kind == meta.LoginWait:
		return "Potwierdź logowanie na innym urządzeniu, na którym masz Facebooka. Ten ekran sam przejdzie dalej."
	case lg.step.Kind == meta.LoginUnsupported:
		return "To logowanie wymaga dodatkowej strony w przeglądarce, której program nie potrafi pokazać. Dokończ je w przeglądarce lub w oficjalnej aplikacji i spróbuj ponownie."
	case lg.step.Instructions != "":
		return loginPhrase(lg.step.Instructions)
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
	fieldH, labelH, optH, optGap := dp(36), sp(20), dp(44), dp(8)
	body := int32(0)
	for i := range lg.edits {
		body += labelH + dp(14)
		if n := int32(len(lg.opts[i])); n > 0 {
			body += n*optH + (n-1)*optGap
		} else {
			body += fieldH
		}
	}
	total := in + sp(34) + dp(8) + th + dp(20) + body + dp(36) + in
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
	for i, e := range lg.edits {
		lg.labelR = append(lg.labelR, rect{x + in, cy, x + cw - in, cy + labelH})
		cy += labelH
		if row := lg.opts[i]; len(row) > 0 {
			lg.fieldR = append(lg.fieldR, rect{}) // a choice draws itself, with no box around it
			for _, b := range row {
				moveTo(b, rect{x + in, cy, x + cw - in, cy + optH})
				cy += optH + optGap
			}
			cy += dp(14) - optGap
			continue
		}
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
		if i >= len(lg.edits) {
			continue
		}
		if len(lg.opts[i]) > 0 { // a choice answers with the very words Meta offered
			if c := lg.choice[i]; c >= 0 && c < len(f.Options) {
				input[f.ID] = f.Options[c]
			}
			continue
		}
		input[f.ID] = getText(lg.edits[i])
		if f.Secret {
			setText(lg.edits[i], "") // a secret does not stay in the field
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

// ---- signing out, from the settings

func signOutLine() string {
	if !session.HaveCookies(dataDir) {
		return "Nie jesteś teraz zalogowany."
	}
	return "Program zapomni sesję na tym komputerze i pokaże ekran logowania. Pobrane rozmowy zostają na dysku."
}

func signOutButton() string {
	if !session.HaveCookies(dataDir) {
		return ""
	}
	return "Wyloguj się"
}

func historyLine() string {
	if session.HaveHistoryDevice(dataDir) {
		return "Odblokowana na tym komputerze: starsze wiadomości z rozmów szyfrowanych są pobierane."
	}
	return "Messenger trzyma starsze wiadomości z rozmów szyfrowanych w kopii zabezpieczonej kodem PIN. Bez niego widać tylko to, co przyjdzie od teraz."
}

func historyButton() string {
	if session.HaveHistoryDevice(dataDir) {
		return ""
	}
	return "Podaj PIN"
}

func historyAction() {
	promptBox("Historia rozmów szyfrowanych",
		"Podaj PIN kopii zapasowej z Messengera (ten, którym odblokowujesz wiadomości na nowym urządzeniu). Trafia wyłącznie do Mety; program go nigdzie nie zapisuje.",
		"PIN z Messengera", "Odblokuj", true, be.unlockHistory)
}

func signOutAction() {
	confirmBox("Wylogować się?", "Program zapomni sesję Messengera na tym komputerze i przestanie odbierać wiadomości, dopóki nie zalogujesz się ponownie. Rozmowy pobrane wcześniej zostają na dysku.", "Wyloguj się", true, signOut)
}

func signOut() {
	be.signOut()
	if err := session.ForgetCookies(dataDir); err != nil {
		be.notice("Nie można usunąć sesji: " + err.Error())
		return
	}
	setNav(navChats)
	showLogin(false)
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

// drawLoginOption draws one way of confirming the login: a row with a mark on the left, the
// way the official application lists them.
func drawLoginOption(d *drawItem, label string, picked bool) {
	w, h := d.rcItem.right, d.rcItem.bottom
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.card)
	edge, fill, inset := mix(pal.fg2, pal.line, 65), pal.card, int32(1)
	switch {
	case picked:
		edge, fill, inset = pal.accent, mix(pal.accent, pal.card, 12), 2
	case d.itemState&odsSelected != 0:
		fill = mix(pal.fg, pal.card, 90)
	case hotBtn == d.hwndItem:
		fill = mix(pal.fg, pal.card, 95)
	}
	g := newGfx(buf.hdc)
	rr := dp(8)
	g.round(0, 0, w, h, rr, rr, rr, rr, edge)
	g.round(inset, inset, w-2*inset, h-2*inset, rr-inset, rr-inset, rr-inset, rr-inset, fill)
	s, mx := dp(18), dp(14)
	my := (h - s) / 2
	ring := mix(pal.fg2, pal.card, 55)
	if picked {
		ring = pal.accent
	}
	g.round(mx, my, s, s, s/2, s/2, s/2, s/2, ring)
	g.round(mx+2, my+2, s-4, s-4, (s-4)/2, (s-4)/2, (s-4)/2, (s-4)/2, fill)
	if picked {
		dot := s - 8
		g.round(mx+4, my+4, dot, dot, dot/2, dot/2, dot/2, dot/2, pal.accent)
	}
	g.free()
	c.font(fnt.body)
	c.text(label, rect{mx + s + dp(12), 0, w - dp(12), h}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
	if focusCue(d) {
		fr := rect{3, 3, w - 3, h - 3}
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(0, 0)
}

// loginPick answers the step with the chosen way, or waits for „Dalej” when the step asks
// for something else as well.
func loginPick(field, opt int) {
	if field >= len(lg.choice) || field >= len(lg.opts) || opt >= len(lg.opts[field]) {
		return
	}
	lg.choice[field] = opt
	for _, b := range lg.opts[field] {
		invalidate(b)
	}
	if len(lg.step.Fields) == 1 {
		loginNext()
	}
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
	for i, r := range lg.fieldR {
		if r.right <= r.left {
			continue // that field is a choice: its rows draw themselves
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
		switch {
		case d.ctlID == idLoginNext:
			label := "Dalej"
			if !lg.started {
				label = "Zaloguj się"
			}
			drawLoginButton(d, label, true)
		case d.ctlID == idLoginCancel:
			drawLoginButton(d, "Wróć", false)
		case d.ctlID >= idLoginOption:
			i, j := int(d.ctlID-idLoginOption)/optionsPerField, int(d.ctlID-idLoginOption)%optionsPerField
			if i < len(lg.step.Fields) && j < len(lg.step.Fields[i].Options) {
				drawLoginOption(d, optionLabel(lg.step.Fields[i].Options[j]), i < len(lg.choice) && lg.choice[i] == j)
			}
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
		case id >= idLoginOption && code == 0:
			loginPick(int(id-idLoginOption)/optionsPerField, int(id-idLoginOption)%optionsPerField)
		case id >= idLoginField && (code == enSetFocus || code == enKillFocus):
			invalidate(hwnd)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
