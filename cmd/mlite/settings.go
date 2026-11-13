//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
}

var (
	cfg     = config{Toasts: true, Body: true, Background: true, ShowMem: true, HeadPreview: true, VideoSession: true}
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
)

type setRow struct {
	kind        int
	title, desc string
	live        func() string // description computed at paint time
	get         func() bool
	set         func(bool)
	y, h        int32
	th, dh      int32 // heights of the wrapped title and description
}

var sv struct {
	hwnd   uintptr
	rows   []setRow
	w, h   int32
	scroll int32
	total  int32
	focus  int
	active bool
}

func buildSettings() {
	sv.rows = []setRow{
		{kind: srHeader, title: "Powiadomienia"},
		{kind: srToggle, title: "Powiadomienia Windows", desc: "Nowa wiadomość pojawia się w rogu ekranu.",
			get: func() bool { return cfg.Toasts }, set: func(v bool) { cfg.Toasts = v }},
		{kind: srToggle, title: "Pokazuj treść wiadomości", desc: "Po wyłączeniu widać tylko, kto napisał.",
			get: func() bool { return cfg.Body }, set: func(v bool) { cfg.Body = v }},
			{kind: srToggle, title: "Nie przeszkadzać", desc: "Wstrzymuje powiadomienia i miganie. Licznik w zasobniku działa dalej.",
				get: func() bool { return cfg.DND }, set: func(v bool) { cfg.DND = v; trayRefresh() }},
		{kind: srHeader, title: "Główki rozmów"},
		{kind: srInfo, title: "Przypinanie rozmowy do pulpitu", desc: "Prawy przycisk na rozmowie na liście, potem „Przypnij jako główkę”."},
		{kind: srToggle, title: "Pokazuj nową wiadomość obok główki", desc: "Krótki dymek na kilka sekund. Wymaga włączonej treści w powiadomieniach.",
			get: func() bool { return cfg.HeadPreview }, set: func(v bool) { cfg.HeadPreview = v }},
		{kind: srHeader, title: "Filmy"},
		{kind: srToggle, title: "Filmy z Facebooka jako zalogowany", desc: "Odtwarzacz w oknie programu dostaje Twoją sesję Facebooka, żeby rolki nie zatrzymywały się na logowaniu. Po wyłączeniu gra tylko to, co publiczne.",
			get: func() bool { return cfg.VideoSession }, set: func(v bool) { cfg.VideoSession = v }},
		{kind: srHeader, title: "Uruchamianie"},
		{kind: srToggle, title: "Uruchamiaj razem z Windows", get: autostartOn, set: setAutostart},
		{kind: srToggle, title: "Po zamknięciu okna działaj w tle", desc: "Program zostaje w zasobniku i dalej odbiera wiadomości.",
			get: func() bool { return cfg.Background }, set: func(v bool) { cfg.Background = v }},
		{kind: srHeader, title: "Wygląd"},
		{kind: srInfo, title: "Motyw, kolor akcentu i wielkość tekstu", desc: "Zgodne z ustawieniami Windows."},
		{kind: srToggle, title: "Pokaż zużycie pamięci na pasku bocznym",
			get: func() bool { return cfg.ShowMem }, set: func(v bool) { cfg.ShowMem = v }},
		{kind: srHeader, title: "O programie"},
		{kind: srInfo, title: appName + " " + appVersion, live: func() string {
			return fmt.Sprintf("Pamięć teraz: %d MB. Program jest nieoficjalny i nie pochodzi od Mety.", workingSetMB())
		}},
	}
}

func layoutSettings() {
	y := dp(20) + sp(44)
	for i := range sv.rows {
		r := &sv.rows[i]
		switch r.kind {
		case srHeader:
			r.h = sp(36)
		default:
			// Title and description wrap, so the row grows instead of cutting text off.
			avail := settingsWidth() - dp(32)
			if r.kind == srToggle {
				avail -= dp(52)
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
	if sv.focus == 0 || sv.rows[sv.focus].kind != srToggle {
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
	c.fill(rect{0, 0, sv.w, sv.h}, pal.layer)
	x, w := dp(28), settingsWidth()
	g := newGfx(buf.hdc)
	for i := range sv.rows {
		r := &sv.rows[i]
		y := r.y - sv.scroll
		if r.kind == srHeader || y+r.h < 0 || y > sv.h {
			continue
		}
		g.round(x, y, w, r.h, dp(6), dp(6), dp(6), dp(6), mix(pal.fg, pal.card, 7))
		g.round(x+1, y+1, w-2, r.h-2, dp(5), dp(5), dp(5), dp(5), pal.card)
		if r.kind == srToggle {
			tx, ty := x+w-dp(16)-dp(40), y+(r.h-dp(20))/2
			if r.get() {
				g.pill(tx, ty, dp(40), dp(20), pal.accent)
				g.ellipse(tx+dp(24), ty+dp(4), dp(12), dp(12), pal.onAccent)
			} else {
				g.pill(tx, ty, dp(40), dp(20), pal.fg2)
				g.pill(tx+1, ty+1, dp(40)-2, dp(20)-2, pal.card)
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
			c.font(fnt.name)
			c.text(r.title, rect{x, y, x + w, y + r.h - dp(6)}, pal.fg, dtSingleLine|0x0008) // DT_BOTTOM
			continue
		}
		right := x + w - dp(16)
		if r.kind == srToggle {
			right -= dp(52)
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
	if i < 0 || i >= len(sv.rows) || sv.rows[i].kind != srToggle {
		return
	}
	r := &sv.rows[i]
	r.set(!r.get())
	saveConfig()
	sv.focus = i
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
		if sv.rows[i].kind == srToggle {
			sv.focus = i
			r := sv.rows[i]
			if r.y-sv.scroll < 0 {
				sv.scroll = r.y - dp(8)
			}
			if r.y+r.h-sv.scroll > sv.h {
				sv.scroll = r.y + r.h - sv.h + dp(8)
			}
			clampSettingsScroll()
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
	case wmMouseWheel:
		sv.scroll -= int32(int16(wp>>16)) * dp(60) / 120
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
		}
		return 0
	case wmSetFocus, wmKillFocus:
		sv.active = msg == wmSetFocus
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
				if r.kind == srToggle && y >= r.y && y < r.y+r.h {
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
