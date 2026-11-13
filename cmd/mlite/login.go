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
	wmAppHistory   = wmApp + 13 // Meta answered whether there is a history backup

	idLoginNext   = 340
	idLoginCancel = 341
	idLoginEye    = 342
	timerLoginDot = 31 // the three dots while the program waits for Meta

	emSetPasswordChar = 0x00CC
	vkCapital         = 0x14
	gEye              = "" // Segoe Fluent / MDL2: an eye
	gLock             = "" // and a padlock
	gWarn             = ""
	idLoginField      = 350
	idLoginOption     = 360 // a choice of one step: idLoginOption + field*optionsPerField + option

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
	hEye           uintptr // shows what was typed in a secret field
	reveal         bool    // and whether it is shown now
	caps           bool    // Caps Lock was on while something was being typed
	dots           int     // the three dots that turn while the program waits
	results        chan loginResult
	w, h           int32
	// laid out by loginLayout, used by the painting
	card, headR, textR, errR, brandR, footR rect
	labelR, fieldR                          []rect
	text                                    string
}

// loginOptionsAbout reports whether any choice of this step carries a line of explanation.
func loginOptionsAbout() bool {
	for i := range lg.opts {
		if len(lg.opts[i]) == 0 || i >= len(lg.step.Fields) {
			continue
		}
		for _, v := range lg.step.Fields[i].Options {
			if optionAbout(v) != "" {
				return true
			}
		}
	}
	return false
}

// secretField is the field of this step that is typed blind, or -1 when there is none.
func secretField() int {
	for i := range lg.edits {
		if lg.edits[i] != 0 && i < len(lg.step.Fields) && lg.step.Fields[i].Secret {
			return i
		}
	}
	return -1
}

func initLogin() {
	lg.results = make(chan loginResult, 4)
	lg.hwnd = createWindow(wsExControlParent, "MliteLogin", "Logowanie", wsChild|wsClipChildren, hMain, 0)
	lg.hNext = createWindow(0, "BUTTON", "Dalej", wsChild|wsVisible|wsTabStop|bsOwnerDraw, lg.hwnd, idLoginNext)
	lg.hCancel = createWindow(0, "BUTTON", "Wróć", wsChild|wsTabStop|bsOwnerDraw, lg.hwnd, idLoginCancel)
	lg.hEye = createWindow(0, "BUTTON", "Pokaż wpisane", wsChild|wsTabStop|bsOwnerDraw, lg.hwnd, idLoginEye)
	for _, h := range []uintptr{lg.hNext, lg.hCancel, lg.hEye} {
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

// loginFields rebuilds what the current step asks for: a text field, or a row of choices
// where the step offers a list (which way to confirm the login, for one).
func loginFields() {
	loginDropControls()
	lg.reveal, lg.caps = false, keyToggled(vkCapital)
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
		return plLogin(lg.step.Instructions)
	}
	return "Podaj dane swojego konta."
}

func loginLayout() {
	if lg.hwnd == 0 {
		return
	}
	cw := dp(440)
	if cw > lg.w-dp(32) {
		cw = lg.w - dp(32)
	}
	x := (lg.w - cw) / 2
	in := dp(32)
	lg.text = loginText()
	var th, eh int32
	withFont(fnt.body, func(c canvas) { _, th = c.measure(lg.text, cw-2*in) })
	if lg.err != "" {
		withFont(fnt.small, func(c canvas) { _, eh = c.measure(lg.err, cw-2*in-dp(42)) })
		eh += dp(16) // the error sits in a box of its own, below the fields it belongs to
	}
	fieldH, labelH, optGap := dp(36), sp(20), dp(8)
	optH := dp(44)
	if loginOptionsAbout() {
		optH = dp(58)
	}
	body := int32(0)
	for i := range lg.edits {
		body += labelH + dp(14)
		if n := int32(len(lg.opts[i])); n > 0 {
			body += n*optH + (n-1)*optGap
		} else {
			body += fieldH
		}
	}
	brandH, footH := dp(36), sp(16)+dp(20)
	total := in + brandH + dp(20) + sp(26) + dp(6) + th + dp(22) + body + dp(36) + dp(22) + footH + in
	if eh > 0 {
		total += eh + dp(14)
	}
	y := (lg.h - total) / 2
	if y < dp(16) {
		y = dp(16)
	}
	lg.card = rect{x, y, x + cw, y + total}
	cy := y + in
	lg.brandR = rect{x + in, cy, x + in + brandH, cy + brandH}
	cy += brandH + dp(20)
	lg.headR = rect{x + in, cy, x + cw - in, cy + sp(26)}
	cy += sp(26) + dp(6)
	lg.textR = rect{x + in, cy, x + cw - in, cy + th}
	cy += th + dp(22)
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
		right := fr.right - dp(10)
		if lg.step.Fields[i].Secret { // room for the button that shows what was typed
			right -= dp(34)
			moveTo(lg.hEye, rect{fr.right - dp(38), fr.top + dp(3), fr.right - dp(4), fr.bottom - dp(3)})
		}
		moveTo(e, rect{fr.left + dp(12), fr.top + (fieldH-lay.editH)/2, right, fr.top + (fieldH-lay.editH)/2 + lay.editH})
		cy += fieldH + dp(14)
	}
	lg.errR = rect{}
	if eh > 0 {
		lg.errR = rect{x + in, cy, x + cw - in, cy + eh}
		cy += eh + dp(14)
	}
	nextW := dp(150)
	moveTo(lg.hNext, rect{x + cw - in - nextW, cy, x + cw - in, cy + dp(36)})
	moveTo(lg.hCancel, rect{x + in, cy, x + in + dp(96), cy + dp(36)})
	cy += dp(36) + dp(22)
	lg.footR = rect{x + in, cy, x + cw - in, cy + footH}
	show(lg.hEye, secretField() >= 0)
	canNext := !lg.busy && lg.step.Kind != meta.LoginWait && !(lg.started && lg.step.Kind == meta.LoginUnsupported)
	on := uintptr(0)
	if canNext {
		on = 1
	}
	pEnableWindow.Call(lg.hNext, on)
	show(lg.hCancel, lg.started && !lg.busy)
	if lg.busy { // the dots turn only while something is really being waited for
		pSetTimer.Call(lg.hwnd, timerLoginDot, 140, 0)
	} else {
		pKillTimer.Call(lg.hwnd, timerLoginDot)
	}
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

var historyBusy bool // the settings are asking Meta about the backup

func historyLine() string {
	switch {
	case historyBusy:
		return "Pytam Messengera o kopię historii…"
	case session.HaveHistoryDevice(dataDir):
		return "Wczytana na tym komputerze: starsze wiadomości z rozmów szyfrowanych są pobierane."
	}
	return "Messenger trzyma starsze wiadomości z rozmów szyfrowanych w kopii zabezpieczonej kodem PIN. " +
		"Bez niej widać tylko to, co przyjdzie od teraz. Wczytuj ją spokojnie dopiero jakiś czas po zalogowaniu: " +
		"zgłoszenie nowego urządzenia zaraz po logowaniu potrafi zablokować konto."
}

func historyButton() string {
	switch {
	case historyBusy:
		return ""
	case session.HaveHistoryDevice(dataDir):
		return ""
	}
	return "Wczytaj"
}

// historyAction asks Meta whether there is a backup at all, and only then asks for the PIN.
// Nothing of this happens by itself: it is the person who asks, from the settings.
func historyAction() { historyAsk() }

func historyAsk() {
	if historyBusy {
		return
	}
	historyBusy = true
	layoutSettings()
	invalidate(sv.hwnd)
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
	historyBusy = false
	layoutSettings()
	invalidate(sv.hwnd)
	if msg != "" {
		historyNone(msg)
		return
	}
	promptBox("Historia rozmów szyfrowanych",
		"Podaj PIN kopii zapasowej z Messengera (ten, którym odblokowujesz wiadomości na nowym urządzeniu). Trafia wyłącznie do Mety; program go nigdzie nie zapisuje.",
		"PIN z Messengera", "Wczytaj", true, be.unlockHistory)
}

// historyNone says why there is nothing to load, without asking anything.
func historyNone(msg string) {
	infoBox("Historia rozmów szyfrowanych", msg)
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

// drawLoginOption draws one way of confirming the login as a tile: the mark, the name, a
// line saying what it means, and an arrow — the way the official application lists them.
func drawLoginOption(d *drawItem, value string, picked bool) {
	w, h := d.rcItem.right, d.rcItem.bottom
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.card)
	edge, fill, inset := mix(pal.fg2, pal.line, 65), pal.card, int32(1)
	switch {
	case picked:
		edge, fill, inset = pal.accent, mix(pal.accent, pal.card, 12), 2
	case d.itemState&odsSelected != 0:
		fill = mix(pal.fg, pal.card, 12)
	case hotBtn == d.hwndItem:
		fill = mix(pal.fg, pal.card, 7)
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
	label, about := optionLabel(value), optionAbout(value)
	tx, right := mx+s+dp(12), w-dp(26)
	if about == "" {
		c.font(fnt.body)
		c.text(label, rect{tx, 0, right, h}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
	} else {
		c.font(fnt.bodySemi)
		c.text(label, rect{tx, dp(8), right, dp(8) + sp(20)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
		c.font(fnt.small)
		c.text(about, rect{tx, dp(8) + sp(19), right, h - dp(6)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
	}
	c.font(fnt.iconSm)
	c.text(gChevron, rect{w - dp(24), 0, w - dp(8), h}, pal.fg2, dtCenter|dtVCenter|dtSingleLine)
	if focusCue(d) {
		fr := rect{3, 3, w - 3, h - 3}
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(0, 0)
}

// drawLoginDots fills the place of the button's label while the program waits for Meta:
// three dots, one after another brighter, the same sign the conversation uses.
func drawLoginDots(d *drawItem) {
	w, h := d.rcItem.right, d.rcItem.bottom
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.card)
	g := newGfx(buf.hdc)
	g.round(0, 0, w, h, dp(4), dp(4), dp(4), dp(4), mix(pal.accent, pal.card, 30))
	for k := int32(0); k < 3; k++ {
		col := mix(pal.accent, pal.card, 55)
		if int32(lg.dots%3) == k {
			col = pal.accent
		}
		g.ellipse(w/2-dp(13)+k*dp(10), h/2-dp(3), dp(6), dp(6), col)
	}
	g.free()
	buf.flush(0, 0)
}

// drawLoginEye draws the little button that shows what is typed blind.
func drawLoginEye(d *drawItem) {
	w, h := d.rcItem.right, d.rcItem.bottom
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.card)
	ink := pal.fg2
	if lg.reveal {
		ink = pal.accent
	}
	if hotBtn == d.hwndItem || d.itemState&odsSelected != 0 {
		g := newGfx(buf.hdc)
		g.round(0, dp(2), w, h-dp(4), dp(6), dp(6), dp(6), dp(6), mix(pal.card, pal.fg, 92))
		g.free()
		ink = pal.fg
	}
	c.font(fnt.icon)
	c.text(gEye, rect{0, 0, w, h}, ink, dtCenter|dtVCenter|dtSingleLine)
	if focusCue(d) {
		fr := rect{2, 2, w - 2, h - 2}
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(0, 0)
}

// toggleReveal shows or hides what was typed in the secret field.
func toggleReveal() {
	i := secretField()
	if i < 0 {
		return
	}
	lg.reveal = !lg.reveal
	ch := uintptr('●')
	if lg.reveal {
		ch = 0
	}
	sendMsg(lg.edits[i], emSetPasswordChar, ch, 0)
	invalidate(lg.edits[i])
	invalidate(lg.hEye)
	pSetFocus.Call(lg.edits[i])
}

// keyToggled reports whether a key such as Caps Lock is on.
func keyToggled(vk int) bool {
	r, _, _ := pGetKeyState.Call(uintptr(vk))
	return r&1 != 0
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
	// The ground is not a flat grey: three soft washes in the brand colours, so that the
	// card sits in light of its own. Nothing else, so the eye goes where the typing is.
	wash, cool := uint32(0x300866E8), uint32(0x2476ADFF)
	if !pal.dark {
		wash, cool = 0x240866E8, 0x1C76ADFF
	}
	k := lg.card
	g.glow(k.left-dp(40), k.top+dp(30), dp(230), wash)
	g.glow(k.right+dp(30), k.bottom-dp(40), dp(260), cool)
	g.glow(k.left+dp(120), k.bottom+dp(120), dp(180), wash)
	g.round(k.left, k.top+dp(4), k.right-k.left, k.bottom-k.top, dp(14), dp(14), dp(14), dp(14), mix(0x000000, pal.mica, 14))
	g.round(k.left, k.top, k.right-k.left, k.bottom-k.top, dp(14), dp(14), dp(14), dp(14), pal.line)
	g.round(k.left+1, k.top+1, k.right-k.left-2, k.bottom-k.top-2, dp(13), dp(13), dp(13), dp(13), pal.card)
	g.round(k.left+dp(14), k.top+1, k.right-k.left-dp(28), 1, 0, 0, 0, 0, mix(pal.fg, pal.card, 18)) // a lit top edge
	focus, _, _ := pGetFocus.Call()
	for i, r := range lg.fieldR {
		if r.right <= r.left {
			continue // that field is a choice: its rows draw themselves
		}
		edge := mix(pal.fg2, pal.line, 65)
		inset := int32(1)
		switch {
		case lg.err != "" && i == len(lg.fieldR)-1:
			edge, inset = pal.signal, 1
		case i < len(lg.edits) && lg.edits[i] == focus:
			edge, inset = pal.accent, 2
		}
		rr := dp(8)
		g.round(r.left, r.top, r.right-r.left, r.bottom-r.top, rr, rr, rr, rr, edge)
		g.round(r.left+inset, r.top+inset, r.right-r.left-2*inset, r.bottom-r.top-2*inset, rr-inset, rr-inset, rr-inset, rr-inset, pal.card)
	}
	if e := lg.errR; e.right > e.left {
		g.round(e.left, e.top, e.right-e.left, e.bottom-e.top, dp(8), dp(8), dp(8), dp(8), mix(pal.signal, pal.card, 12))
		g.round(e.left, e.top, dp(3), e.bottom-e.top, dp(2), 0, 0, dp(2), pal.signal)
	}
	if f := lg.footR; f.right > f.left {
		g.round(f.left, f.top, f.right-f.left, 1, 0, 0, 0, 0, pal.line)
	}
	g.free()
	drawBrand(buf.hdc, lg.brandR.left, lg.brandR.top, lg.brandR.right-lg.brandR.left)
	c.font(fnt.smallSemi)
	c.text("LINNET", rect{lg.brandR.right + dp(12), lg.brandR.top, lg.card.right - dp(32), lg.brandR.bottom}, pal.fg2, dtSingleLine|dtVCenter)
	c.font(fnt.title)
	c.text(loginHead(), lg.headR, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
	c.font(fnt.body)
	c.text(lg.text, lg.textR, pal.fg2, dtWordBreak)
	for i, r := range lg.labelR {
		if i >= len(lg.step.Fields) {
			break
		}
		c.font(fnt.small)
		c.text(fieldLabel(lg.step.Fields[i]), r, pal.fg, dtSingleLine|dtVCenter)
		if lg.caps && lg.step.Fields[i].Secret { // a password typed in capitals by mistake
			c.font(fnt.smallSemi)
			c.text("CAPS LOCK", r, mix(pal.signal, pal.fg2, 50), dtSingleLine|dtVCenter|dtRight)
		}
	}
	if e := lg.errR; e.right > e.left {
		c.font(fnt.icon)
		c.text(gWarn, rect{e.left + dp(12), e.top, e.left + dp(34), e.bottom}, pal.signal, dtSingleLine|dtVCenter)
		c.font(fnt.small)
		c.text(lg.err, rect{e.left + dp(40), e.top + dp(8), e.right - dp(10), e.bottom - dp(8)}, pal.signal, dtWordBreak)
	}
	if f := lg.footR; f.right > f.left {
		c.font(fnt.iconSm)
		c.text(gLock, rect{f.left, f.top + dp(12), f.left + dp(18), f.bottom}, pal.fg2, dtSingleLine|dtVCenter)
		c.font(fnt.small)
		c.text("Dane logowania trafiają tylko do Mety.",
			rect{f.left + dp(22), f.top + dp(12), f.right, f.bottom}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
	}
	buf.flush(0, 0)
}

// loginHead names the step the person is on, so that the card says where they are.
func loginHead() string {
	switch {
	case lg.busy:
		return "Łączenie…"
	case !lg.started:
		return "Zaloguj się do Messengera"
	case lg.step.Kind == meta.LoginWait:
		return "Potwierdź na telefonie"
	case len(lg.step.Fields) == 1 && len(lg.step.Fields[0].Options) > 0:
		return "Potwierdź logowanie"
	case lg.step.Kind == meta.LoginInput && secretField() < 0:
		return "Kod weryfikacyjny"
	}
	return "Zaloguj się do Messengera"
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
	case wmTimer:
		if wp == timerLoginDot {
			lg.dots++
			invalidate(lg.hNext)
		}
		return 0
	case wmDrawItem:
		d := (*drawItem)(unsafe.Pointer(lp))
		switch {
		case d.ctlID == idLoginNext && lg.busy:
			drawLoginDots(d)
		case d.ctlID == idLoginNext:
			label := "Dalej"
			if !lg.started {
				label = "Zaloguj się"
			}
			drawLoginButton(d, label, true)
		case d.ctlID == idLoginCancel:
			drawLoginButton(d, "Wróć", false)
		case d.ctlID == idLoginEye:
			drawLoginEye(d)
		case d.ctlID >= idLoginOption:
			i, j := int(d.ctlID-idLoginOption)/optionsPerField, int(d.ctlID-idLoginOption)%optionsPerField
			if i < len(lg.step.Fields) && j < len(lg.step.Fields[i].Options) {
				drawLoginOption(d, lg.step.Fields[i].Options[j], i < len(lg.choice) && lg.choice[i] == j)
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
		case id == idLoginEye && code == 0:
			toggleReveal()
		case id >= idLoginOption && code == 0:
			loginPick(int(id-idLoginOption)/optionsPerField, int(id-idLoginOption)%optionsPerField)
		case id >= idLoginField && (code == enSetFocus || code == enKillFocus || code == enChange):
			if caps := keyToggled(vkCapital); caps != lg.caps {
				lg.caps = caps
			}
			invalidate(hwnd)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
