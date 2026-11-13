//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// config holds the settings that the program itself applies. Theme, accent and text size
// come from Windows and are deliberately not duplicated here.
type config struct {
	Toasts     bool `json:"toasts"`
	Body       bool `json:"body"`
	Background bool `json:"background"`
	ShowMem    bool `json:"show_mem"`
	DND        bool `json:"dnd"`
	// Heads are the conversations pinned to the desktop, with their positions.
	Heads       []headCfg `json:"heads"`
	HeadPreview bool      `json:"head_preview"`
	// Quiet are the conversations this program does not notify about (its own switch,
	// separate from muting in Messenger).
	Quiet []int64 `json:"quiet"`
	// VideoSession hands the embedded video player this program's Facebook session.
	VideoSession bool `json:"video_session"`
	// Sound plays the Windows message sound with a notification.
	Sound bool `json:"sound"`
	// SoundFile is which sound that is: a file of the Windows sounds, a path to one of your
	// own, or empty for whatever the system sets for a message.
	SoundFile string `json:"sound_file"`
	// Tone is one of the program's own sounds; it comes before SoundFile when set.
	Tone string `json:"tone"`
	// Theme is "" to follow Windows, "light" or "dark".
	Theme string `json:"theme"`
	// HeadMode is how conversations opened from heads are shown: "" one at a time, "tabs".
	HeadMode string `json:"head_mode"`
	// HeadSize is the size of the heads at 100%: 48, 64 or 80; 0 means 64.
	HeadSize int `json:"head_size,omitempty"`
	// HeadPanes is how many conversations Kolumny and Dok keep open at once: 3 or 4; 0 means 3.
	HeadPanes int `json:"head_panes,omitempty"`
	// Groups are the groups heads are tied into; a head names its group in its own entry.
	Groups []groupCfg `json:"groups,omitempty"`
	// The size the windows of conversations beside the heads were dragged to, at 100%; the
	// dock keeps its own. 0 means the size they have always had.
	PanelW int32 `json:"panel_w,omitempty"`
	PanelH int32 `json:"panel_h,omitempty"`
	DockW  int32 `json:"dock_w,omitempty"`
	DockH  int32 `json:"dock_h,omitempty"`
}

var (
	cfg     = config{Toasts: true, Body: true, Background: true, ShowMem: true, HeadPreview: true, VideoSession: true, Sound: true}
	cfgPath string
	dataDir string
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func loadConfig(dir string) {
	dataDir = dir
	go sweepOutbox()
	cfgPath = filepath.Join(dir, "settings.json")
	if raw, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(raw, &cfg)
	}
}

func saveConfig() {
	if raw, err := json.MarshalIndent(cfg, "", "  "); err == nil {
		_ = os.WriteFile(cfgPath, raw, 0o600)
	}
}

func autostartOn() bool {
	return len(regBytes(runKey, appName, 2048)) > 0
}

func setAutostart(on bool) {
	key, name := uintptr(unsafe.Pointer(u16(runKey))), uintptr(unsafe.Pointer(u16(appName)))
	if !on {
		pRegDeleteKeyValueW.Call(hkcu, key, name)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := syscall.StringToUTF16(fmt.Sprintf(`"%s" -data-dir "%s" -tray`, exe, dataDir))
	pRegSetKeyValueW.Call(hkcu, key, name, regSz, uintptr(unsafe.Pointer(&cmd[0])), uintptr(len(cmd)*2))
}

const (
	srHeader = iota
	srToggle
	srInfo
	srAction // a row with a button on the right
)

type setRow struct {
	kind        int
	title, desc string
	live        func() string // description computed at paint time
	label       func() string // text of the button of an action row, "" for none
	action      func()
	whole       bool   // the whole row acts, not only its button: a choice that is easily undone
	key         string // a name for a row that code needs to find
	btn         rect   // where that button was drawn
	get         func() bool
	set         func(bool)
	y, h        int32
	th, dh      int32 // heights of the wrapped title and description
}

var sv struct {
	hwnd    uintptr
	rows    []setRow
	w, h    int32
	scroll  int32
	total   int32
	focus   int
	active  bool
	hotRow  int // row whose button is under the pointer, -1 when none
	hotLine int // row under the pointer that acts as a whole, -1 when none
}

func buildSettings() {
	sv.hotRow, sv.hotLine = -1, -1
	// From what the program does every day to what concerns the program itself.
	sv.rows = []setRow{
		{kind: srHeader, title: "Powiadomienia"},
		{kind: srToggle, title: "Powiadomienia Windows", desc: "Nowa wiadomość pojawia się w rogu ekranu.",
			get: func() bool { return cfg.Toasts }, set: func(v bool) { cfg.Toasts = v }},
		{kind: srToggle, title: "Pokazuj treść wiadomości", desc: "Po wyłączeniu widać tylko, kto napisał.",
			get: func() bool { return cfg.Body }, set: func(v bool) { cfg.Body = v }},
		{kind: srToggle, title: "Dźwięk nowej wiadomości", desc: "Krótki dźwięk razem z powiadomieniem.",
			get: func() bool { return cfg.Sound }, set: func(v bool) { cfg.Sound = v }},
		{kind: srAction, title: soundRowTitle, desc: "Wybrany dźwięk od razu zagra, żeby go posłuchać.",
			live: soundLine, label: soundLabel, action: pickSound, whole: true},
		{kind: srToggle, title: "Nie przeszkadzać", desc: "Wstrzymuje powiadomienia i miganie. Licznik w zasobniku działa dalej.",
			get: func() bool { return cfg.DND }, set: func(v bool) { cfg.DND = v; trayRefresh() }},
		{kind: srHeader, title: "Główki rozmów"},
		{kind: srToggle, title: "Pokazuj nową wiadomość obok główki", desc: "Krótki dymek na kilka sekund. Wymaga włączonej treści w powiadomieniach.",
			get: func() bool { return cfg.HeadPreview }, set: func(v bool) { cfg.HeadPreview = v }},
		{kind: srAction, title: "Rozmowy przy główkach", desc: "Jedna naraz: kolejna główka podmienia rozmowę. Zakładki: rozmowy na pasku u góry panelu. Kolumny: do trzech rozmów obok siebie. Dok: do trzech mniejszych okien u dołu ekranu.",
			label: headModeLabel, action: pickHeadMode, whole: true},
		{kind: srAction, title: "Rozmiar główek", label: headSizeLabel, action: pickHeadSize, whole: true},
		{kind: srAction, title: "Rozmów naraz", desc: "Ile rozmów trzymają otwartych Kolumny i Dok. Kolejna zamyka tę najdawniej używaną.",
			label: headPanesLabel, action: pickHeadPanes, whole: true},
		{kind: srInfo, title: "Rozmowę przypniesz jako główkę prawym przyciskiem na liście rozmów. Główkę upuszczoną przy innej główce program z nią spina; Shift przy przeciąganiu ją wyjmuje."},
		{kind: srHeader, title: "Uruchamianie i tło"},
		{kind: srToggle, title: "Uruchamiaj razem z Windows", get: autostartOn, set: setAutostart},
		{kind: srToggle, title: "Po zamknięciu okna działaj w tle", desc: "Program zostaje w zasobniku i dalej odbiera wiadomości.",
			get: func() bool { return cfg.Background }, set: func(v bool) { cfg.Background = v }},
		{kind: srHeader, title: "Wygląd"},
		{kind: srAction, title: "Motyw", desc: "Auto: tak jak w ustawieniach Windows.", label: themeLabel, action: pickTheme, whole: true},
		{kind: srToggle, title: "Pokazuj zużycie pamięci na pasku bocznym",
			get: func() bool { return cfg.ShowMem }, set: func(v bool) { cfg.ShowMem = v }},
		{kind: srAction, title: "Kolor akcentu i wielkość tekstu", desc: "Linnet bierze je z Windows.",
			label: func() string { return "Otwórz ustawienia Windows" }, action: openWindowsColours},
		{kind: srHeader, title: "Konto"},
		{kind: srAction, title: "Konto Messengera", live: signOutLine, label: signOutButton, action: signOutAction},
		{kind: srAction, title: "Historia rozmów szyfrowanych", live: historyLine, label: historyButton, action: historyAction},
		{kind: srToggle, title: "Odtwarzaj filmy jako zalogowany", desc: "Odtwarzacz używa Twojej sesji Facebooka, więc rolki nie zatrzymują się na logowaniu. Po wyłączeniu zagrają tylko filmy publiczne.",
			get: func() bool { return cfg.VideoSession }, set: func(v bool) { cfg.VideoSession = v }},
		{kind: srHeader, title: "O programie " + appName},
		{kind: srAction, key: "update", title: "Aktualizacje", live: updateLine, label: updateButton, action: updateAction},
		{kind: srInfo, title: appName + " " + appVersion, desc: "Klient wiadomości Messenger na Windows. Niezależna aplikacja, niepowiązana z Meta Platforms, Inc. Messenger i Facebook są znakami ich właścicieli."},
		{kind: srInfo, title: "Zużycie pamięci", live: func() string {
			return fmt.Sprintf("Teraz %d MB.", workingSetMB())
		}},
	}
}

// openWindowsColours opens the page of Windows' own settings where the accent colour is;
// the size of text is one page away from there.
func openWindowsColours() {
	pShellExecuteW.Call(hMain, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16("ms-settings:colors"))), 0, 0, 1)
}

// settingLabel is what a screen reader is told about a row: its name, what kind of control
// it is, and its state.
func settingLabel(i int) string {
	if i < 0 || i >= len(sv.rows) {
		return "Ustawienia"
	}
	r := sv.rows[i]
	switch r.kind {
	case srToggle:
		state := "wyłączone"
		if r.get() {
			state = "włączone"
		}
		return r.title + ", przełącznik, " + state
	case srAction:
		if l := strings.TrimSuffix(r.label(), " ▾"); l != "" {
			return r.title + ", przycisk " + l
		}
	}
	return r.title
}

// settingsAnnounce names the panel after the row the keyboard is on and reports the change,
// the way the list of conversations does: that is what a screen reader reads out.
func settingsAnnounce() {
	setText(sv.hwnd, settingLabel(sv.focus))
	pNotifyWinEvent.Call(0x800C, sv.hwnd, ^uintptr(3), 0)
}

var themes = []struct{ id, label string }{{"", "Auto"}, {"light", "Jasny"}, {"dark", "Ciemny"}}

func themeLabel() string {
	for _, t := range themes {
		if t.id == cfg.Theme {
			return t.label + " ▾"
		}
	}
	return themes[0].label + " ▾"
}

// settingsPick shows a list of choices under the button of the row, the current one ticked,
// and gives back the chosen index or -1. A line goes before each index in breaks.
func settingsPick(labels []string, current int, breaks ...int) int {
	const idFirst = 1
	menu, _, _ := pCreatePopupMenu.Call()
	for i, l := range labels {
		for _, b := range breaks {
			if b == i {
				pAppendMenuW.Call(menu, mfSeparator, 0, 0)
			}
		}
		flags := uintptr(mfString)
		if i == current {
			flags |= mfChecked
		}
		pAppendMenuW.Call(menu, flags, uintptr(idFirst+i), uintptr(unsafe.Pointer(u16(l))))
	}
	pt := point{dp(28), 0}
	if sv.focus >= 0 && sv.focus < len(sv.rows) {
		if b := sv.rows[sv.focus].btn; b.right > b.left {
			pt = point{b.left, b.bottom + dp(4)}
		}
	}
	pClientToScreen.Call(sv.hwnd, uintptr(unsafe.Pointer(&pt)))
	cmd, _, _ := pTrackPopupMenu.Call(menu, tpmRightBtn|tpmReturnCmd|tpmNoNotify, uintptr(pt.x), uintptr(pt.y), 0, hMain, 0)
	pDestroyMenu.Call(menu)
	if cmd == 0 {
		return -1
	}
	return int(cmd) - idFirst
}

// pickTheme lists the themes; the chosen one repaints everything as after a change in Windows.
func pickTheme() {
	labels, current := []string{}, 0
	for i, t := range themes {
		labels = append(labels, t.label)
		if t.id == cfg.Theme {
			current = i
		}
	}
	i := settingsPick(labels, current)
	if i < 0 || themes[i].id == cfg.Theme {
		return
	}
	cfg.Theme = themes[i].id
	saveConfig()
	sendMsg(hMain, wmSettingChange, 0, 0)
}

func layoutSettings() {
	y := dp(20) + sp(44)
	for i := range sv.rows {
		r := &sv.rows[i]
		switch r.kind {
		case srHeader:
			r.h = sp(44)
		default:
			// Title and description wrap, so the row grows instead of cutting text off.
			avail := settingsWidth() - dp(32)
			if r.kind == srToggle {
				avail -= dp(52)
			}
			if r.kind == srAction && r.label() != "" {
				withFont(fnt.name, func(c canvas) { avail -= c.width(r.label()) + dp(44) })
			}
			if r.kind == srInfo { // plain words on the ground: no card, so no room lost to one
				avail = settingsWidth() - dp(16)
			}
			if avail < dp(80) {
				avail = dp(80)
			}
			desc := r.desc
			if r.live != nil {
				desc = r.live()
			}
			r.th, r.dh = sp(20), 0
			withFont(fnt.body, func(c canvas) {
				if _, h := c.measure(r.title, avail); h > r.th {
					r.th = h
				}
			})
			if desc != "" {
				withFont(fnt.small, func(c canvas) { _, r.dh = c.measure(desc, avail) })
				if r.dh < sp(18) {
					r.dh = sp(18)
				}
				r.h = dp(20) + r.th + r.dh + sp(2)
			} else {
				r.h = dp(28) + r.th
			}
		}
		r.y = y
		y += r.h
		if r.kind != srHeader {
			y += dp(3)
		}
	}
	sv.total = y + dp(24)
	if sv.focus <= 0 || sv.focus >= len(sv.rows) || sv.rows[sv.focus].kind == srHeader || sv.rows[sv.focus].kind == srInfo {
		sv.focus = 1
	}
}

func clampSettingsScroll() {
	if max := sv.total - sv.h; sv.scroll > max {
		sv.scroll = max
	}
	if sv.scroll < 0 {
		sv.scroll = 0
	}
	si := scrollInfo{fMask: sifRange | sifPage | sifPos, nMin: 0, nMax: sv.total - 1, nPage: uint32(sv.h), nPos: sv.scroll}
	if sv.total <= sv.h {
		si.nMax, si.nPage = 0, 1
	}
	si.cbSize = uint32(unsafe.Sizeof(si))
	pSetScrollInfo.Call(sv.hwnd, sbVert, uintptr(unsafe.Pointer(&si)), 1)
	invalidate(sv.hwnd)
}

func settingsWidth() int32 {
	w := sv.w - dp(56)
	if w > dp(640) {
		w = dp(640)
	}
	return w
}

func paintSettings(target uintptr) {
	buf := newBuffer(target, sv.w, sv.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, sv.w, sv.h}, pal.mica)
	x, w := dp(28), settingsWidth()
	g := newGfx(buf.hdc)
	for i := range sv.rows {
		r := &sv.rows[i]
		y := r.y - sv.scroll
		if r.kind == srHeader || r.kind == srInfo || y+r.h < 0 || y > sv.h {
			continue // words only: a card would promise something to press
		}
		face := pal.card
		if sv.hotLine == i {
			face = mix(pal.fg, pal.card, 5)
		}
		g.round(x, y, w, r.h, dp(8), dp(8), dp(8), dp(8), pal.line)
		g.round(x+1, y+1, w-2, r.h-2, dp(7), dp(7), dp(7), dp(7), face)
		if r.kind == srAction {
			r.btn = rect{}
			if label := r.label(); label != "" {
				var bw int32
				withFont(fnt.name, func(c canvas) { bw = c.width(label) + dp(32) })
				bh := dp(32)
				if th := sp(20) + dp(10); th > bh { // the button grows with the size of text
					bh = th
				}
				r.btn = rect{x + w - dp(16) - bw, y + (r.h-bh)/2, x + w - dp(16), y + (r.h+bh)/2}
				fill := pal.accent
				if sv.hotRow == i {
					fill = pal.accentHover
				}
				g.round(r.btn.left, r.btn.top, bw, bh, dp(8), dp(8), dp(8), dp(8), fill)
			}
		}
		if r.kind == srToggle {
			tx, ty := x+w-dp(16)-dp(40), y+(r.h-dp(20))/2
			if r.get() {
				g.pill(tx, ty, dp(40), dp(20), pal.accent)
				g.ellipse(tx+dp(24), ty+dp(4), dp(12), dp(12), pal.onAccent)
			} else {
				g.pill(tx, ty, dp(40), dp(20), pal.fg2)
				g.pill(tx+1, ty+1, dp(40)-2, dp(20)-2, face)
				g.ellipse(tx+dp(4), ty+dp(4), dp(12), dp(12), pal.fg2)
			}
		}
	}
	g.free()
	c.font(fnt.title)
	c.text("Ustawienia", rect{x, dp(16) - sv.scroll, x + w, dp(16) + sp(36) - sv.scroll}, pal.fg, dtSingleLine|dtVCenter)
	for i := range sv.rows {
		r := &sv.rows[i]
		y := r.y - sv.scroll
		if y+r.h < 0 || y > sv.h {
			continue
		}
		if r.kind == srHeader {
			c.font(fnt.head)
			c.text(r.title, rect{x, y, x + w, y + r.h - dp(8)}, pal.fg, dtSingleLine|0x0008) // DT_BOTTOM
			continue
		}
		if r.kind == srInfo {
			desc := r.desc
			if r.live != nil {
				desc = r.live()
			}
			ink := pal.fg2
			if desc != "" {
				ink = pal.fg
			}
			c.font(fnt.body)
			c.text(r.title, rect{x + dp(4), y + dp(10), x + w - dp(12), y + dp(10) + r.th}, ink, dtWordBreak)
			if desc != "" {
				c.font(fnt.small)
				c.text(desc, rect{x + dp(4), y + dp(10) + r.th + sp(1), x + w - dp(12), y + r.h}, pal.fg2, dtWordBreak)
			}
			continue
		}
		right := x + w - dp(16)
		if r.kind == srToggle {
			right -= dp(52)
		}
		if r.kind == srAction && r.btn.right > r.btn.left {
			right = r.btn.left - dp(12)
			c.font(fnt.name)
			c.text(r.label(), r.btn, pal.onAccent, dtCenter|dtVCenter|dtSingleLine)
		}
		desc := r.desc
		if r.live != nil {
			desc = r.live()
		}
		if desc == "" {
			c.font(fnt.body)
			c.text(r.title, rect{x + dp(16), y + dp(14), right, y + dp(14) + r.th}, pal.fg, dtWordBreak)
		} else {
			c.font(fnt.body)
			c.text(r.title, rect{x + dp(16), y + dp(10), right, y + dp(10) + r.th}, pal.fg, dtWordBreak)
			c.font(fnt.small)
			c.text(desc, rect{x + dp(16), y + dp(10) + r.th + sp(1), right, y + r.h - dp(4)}, pal.fg2, dtWordBreak)
		}
		if sv.active && i == sv.focus {
			fr := rect{x + 2, y + 2, x + w - 2, y + r.h - 2}
			drawFocus(buf.hdc, &fr)
		}
	}
	buf.flush(0, 0)
}

func toggleSetting(i int) {
	if i < 0 || i >= len(sv.rows) {
		return
	}
	if r := &sv.rows[i]; r.kind == srAction {
		if r.label() != "" {
			sv.focus = i
			r.action()
			invalidate(sv.hwnd)
		}
		return
	}
	if sv.rows[i].kind != srToggle {
		return
	}
	r := &sv.rows[i]
	r.set(!r.get())
	saveConfig()
	sv.focus = i
	settingsAnnounce()
	invalidate(sv.hwnd)
	invalidate(hMain)
}

func moveSettingsFocus(d int) {
	i := sv.focus
	for {
		i += d
		if i < 0 || i >= len(sv.rows) {
			return
		}
		if k := sv.rows[i].kind; k == srToggle || (k == srAction && sv.rows[i].label() != "") {
			sv.focus = i
			r := sv.rows[i]
			if r.y-sv.scroll < 0 {
				sv.scroll = r.y - dp(8)
			}
			if r.y+r.h-sv.scroll > sv.h {
				sv.scroll = r.y + r.h - sv.h + dp(8)
			}
			clampSettingsScroll()
			settingsAnnounce()
			return
		}
	}
}

func settingsProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		paintSettings(hdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSize:
		var rc rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		if rc.right == sv.w && rc.bottom == sv.h {
			return 0
		}
		sv.w, sv.h = rc.right, rc.bottom
		layoutSettings()
		clampSettingsScroll()
		return 0
	case wmMouseMove:
		x, y := lpPoint(lp)
		hot, line := -1, -1
		for i := range sv.rows {
			r := sv.rows[i]
			if b := r.btn; b.right > b.left && x >= b.left && x < b.right && y >= b.top && y < b.bottom {
				hot = i
			}
			if ry := r.y - sv.scroll; x >= dp(28) && x < dp(28)+settingsWidth() && y >= ry && y < ry+r.h &&
				(r.kind == srToggle || (r.kind == srAction && r.whole && r.label() != "")) {
				line = i
			}
		}
		if hot != sv.hotRow || line != sv.hotLine {
			sv.hotRow, sv.hotLine = hot, line
			invalidate(hwnd)
			if hot >= 0 || line >= 0 {
				trackLeave(hwnd)
			}
		}
		return 0
	case wmMouseLeave:
		if sv.hotRow != -1 || sv.hotLine != -1 {
			sv.hotRow, sv.hotLine = -1, -1
			invalidate(hwnd)
		}
		return 0
	case wmSetCursor:
		if lp&0xFFFF == 1 && (sv.hotRow >= 0 || sv.hotLine >= 0) {
			cur, _, _ := pLoadCursorW.Call(0, idcHand)
			pSetCursor.Call(cur)
			return 1
		}
	case wmMouseWheel:
		lines := uint32(3)
		pSystemParametersInfo.Call(0x0068, 0, uintptr(unsafe.Pointer(&lines)), 0) // SPI_GETWHEELSCROLLLINES
		if lines == 0 || lines > 20 {                                             // "nothing" and "a page at a time" both become a sensible step
			lines = 3
		}
		sv.scroll -= int32(int16(wp>>16)) * int32(lines) * sp(20) / 120
		clampSettingsScroll()
		return 0
	case wmVScroll:
		switch wp & 0xFFFF {
		case sbLineUp:
			sv.scroll -= dp(40)
		case sbLineDown:
			sv.scroll += dp(40)
		case sbPageUp:
			sv.scroll -= sv.h * 9 / 10
		case sbPageDown:
			sv.scroll += sv.h * 9 / 10
		case sbThumbTrack:
			si := scrollInfo{fMask: sifTrackPos}
			si.cbSize = uint32(unsafe.Sizeof(si))
			pGetScrollInfo.Call(hwnd, sbVert, uintptr(unsafe.Pointer(&si)))
			sv.scroll = si.nTrack
		}
		clampSettingsScroll()
		return 0
	case wmGetDlgCode:
		return 0x0001
	case wmKeyDown:
		switch wp {
		case vkUp:
			moveSettingsFocus(-1)
		case vkDown:
			moveSettingsFocus(1)
		case vkSpace, vkReturn:
			toggleSetting(sv.focus)
		case vkHome:
			sv.focus = 0
			moveSettingsFocus(1)
			sv.scroll = 0
			clampSettingsScroll()
		case vkEnd:
			sv.focus = len(sv.rows)
			moveSettingsFocus(-1)
			sv.scroll = sv.total
			clampSettingsScroll()
		case vkPrior:
			sv.scroll -= sv.h * 9 / 10
			clampSettingsScroll()
		case vkNext:
			sv.scroll += sv.h * 9 / 10
			clampSettingsScroll()
		}
		return 0
	case wmSetFocus, wmKillFocus:
		sv.active = msg == wmSetFocus
		if sv.active {
			settingsAnnounce()
		}
		invalidate(hwnd)
		return 0
	case wmLButtonDown:
		pSetFocus.Call(hwnd)
		return 0
	case wmLButtonUp:
		x, y := int32(int16(lp)), int32(int16(lp>>16))+sv.scroll
		if x >= dp(28) && x < dp(28)+settingsWidth() {
			for i := range sv.rows {
				r := sv.rows[i]
				if (r.kind == srToggle || r.kind == srAction) && y >= r.y && y < r.y+r.h {
					// A row with a button acts only from the button: a click on the words
					// beside it, to read them, must not sign anyone out or start anything.
					if b, wy := r.btn, y-sv.scroll; r.kind == srAction && !r.whole && !(x >= b.left && x < b.right && wy >= b.top && wy < b.bottom) {
						break
					}
					toggleSetting(i)
					break
				}
			}
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
