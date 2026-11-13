//go:build windows

// Linnet (working name was mlite): a small native Windows window for Messenger, built on the lazyfmess core.
package main

import (
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/store"
)

const (
	wmAppRefresh = wmApp + 1
	wmAppStatus  = wmApp + 2
	wmAppTray    = wmApp + 3
	wmAppOpen    = wmApp + 4
	wmAppAvatar  = wmApp + 5

	idNavChats    = 301
	idNavPeople   = 302
	idNavSettings = 303
	idSearch      = 310
	idList        = 311
	idInput       = 330
	idSend        = 331

	idTrayShow = 201
	idTrayQuit = 202
	idTrayDND  = 203

	timerOpen    = 1
	timerMem     = 2
	timerSearch  = 3
	ninBalloonCk = 0x0405

	navChats    = 0
	navPeople   = 1
	navSettings = 2

	maxInputLines = 6
	statusOK      = "Połączono"
)

type rowData struct {
	t                         store.Thread
	person                    *meta.Person
	title, preview, when, sub string
	icon                      string // sign before the preview when the last message is an attachment
	unread                    int
	seen                      []int64 // who has read up to our last message
}

var (
	be      *backend
	hMain   uintptr
	hNav    [3]uintptr
	hSearch uintptr
	hList   uintptr
	hView   uintptr
	hInput  uintptr
	hSend   uintptr
	hSet    uintptr
	brMica  uintptr
	brCard  uintptr

	oldInputProc, oldBtnProc uintptr

	nav          int
	threads      []store.Thread
	rows         []rowData
	selectedID   int64
	listKey      uint64
	query        string
	unreadTotal  int
	prevUpdated  = map[int64]int64{}
	lastMarked   = map[int64]int64{}
	tray         notifyIconData
	hintShown    bool
	quitting     bool
	focusEdit    uintptr
	memMB        int
	hotRow       = -1
	hotBtn       uintptr
	listTracking bool
	compLines    = 1
	inputEmpty   = true
	searchBusy   bool
	titleNick    = map[int64]string{} // nickname of the other person by one-to-one thread
	notifyID     int64                // conversation of the last notification
	inChat       bool                 // the narrow layout shows the conversation rather than the list

	lay struct {
		w, h, editH                                             int32
		rail, pane, content, header, comp, search, input, strip rect
		bar                                                     rect // reply or edit bar above the text field
		narrow, chat                                            bool // one-column layout, and whether it shows the conversation
		hx                                                      int32
		hbInfo, hbSearch                                        rect // buttons at the right end of the conversation header
		back, navBar                                            rect
		stripText                                               string
		stripClick                                              bool // the bar leads to the settings
	}
)

func main() {
	runtime.LockOSThread()
	debug.SetGCPercent(40) // a small heap matters more here than a little processor time
	dirFlag := flag.String("data-dir", "", "katalog z sesją i bazą wiadomości")
	selfFlag := flag.Int64("self", 0, "identyfikator własnego konta, gdy brak połączenia (do testów)")
	trayFlag := flag.Bool("tray", false, "uruchom bez pokazywania okna")
	offlineFlag := flag.Bool("offline", false, "nie łącz się z Messengerem (do testów wyglądu)")
	loginDemo := flag.Int("login-demo", 0, "pokaż ekran logowania bez łączenia: 1 start, 2 pola, 3 błąd, 4 wybór potwierdzenia (do testów wyglądu)")
	noReadFlag := flag.Bool("no-read", false, "nie oznaczaj rozmów jako przeczytanych (do sprawdzeń)")
	installFlag := flag.Bool("install", false, "zainstaluj dla bieżącego użytkownika")
	uninstallFlag := flag.Bool("uninstall", false, "odinstaluj")
	quietFlag := flag.Bool("quiet", false, "instaluj i odinstalowuj bez pytań")
	launchFlag := flag.Bool("launch", false, "uruchom program po instalacji (używane przy aktualizacji)")
	nameFlag := flag.String("name", "", "inna nazwa programu (do testów instalacji)")
	flag.Parse()
	if *nameFlag != "" {
		appName = *nameFlag
	}
	dir := *dirFlag
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			fatal("Nie można ustalić katalogu danych: " + err.Error())
		}
		dir = filepath.Join(base, appName)
	}
	dir, _ = filepath.Abs(dir)

	// A file named like "Linnet-Setup.exe" is the installer; so is -install.
	setup := strings.Contains(strings.ToLower(filepath.Base(os.Args[0])), "setup")
	launchAfter = *launchFlag
	if *installFlag || *uninstallFlag || setup {
		pSetProcessDpiCtx.Call(^uintptr(3))
		enableVisualStyles()
		gdipInit()
		loadPalette()
		if *uninstallFlag {
			runUninstall(*quietFlag, dir)
		} else {
			runInstall(*quietFlag)
		}
		return
	}
	if _, err := os.Stat(shortcutPath()); err == nil {
		setAppID() // installed: notifications carry the shortcut's name and icon
	}

	if !onlyInstance(dir) { // one program per account data: the one already running is shown instead
		return
	}
	var err error
	be, err = newBackend(dir)
	if err != nil {
		fatal("Nie można uruchomić: " + err.Error())
	}
	be.selfOverride = *selfFlag
	be.noRead = *noReadFlag
	if *offlineFlag {
		be.status = statusOK
		av.offline = true
	}
	loadConfig(dir)

	pSetProcessDpiCtx.Call(^uintptr(3))
	enableVisualStyles()
	gdipInit()
	allowDarkMenus()
	loadPalette()
	buildSettings()
	if dir := os.Getenv("MLITE_ICONDUMP"); dir != "" { // for looking at the icons only
		dumpIcons(dir)
	}
	registerClass("MliteMain", wndProc)
	registerClass("MliteView", viewProc)
	registerClass("MliteViewer", viewerProc)
	registerClass("MlitePanel", panelProc)
	registerClass("MliteDialog", dialogProc)
	registerClass("MliteList", listWndProc)
	registerClass("MliteReact", pickerProc)
	registerClass("MliteLogin", loginProc)
	registerClass("MliteSettings", settingsProc)
	registerClass("MliteHead", headProc)
	registerClass("MlitePeek", peekProc)
	createMainWindow(!*trayFlag)
	refresh()
	if dir := os.Getenv("MLITE_ICONDUMP"); dir != "" {
		dumpHeads(dir)
		dumpNotifyIcons(dir)
	}
	be.hwnd.Store(hMain)
	toastStart()
	updateStart()
	if *loginDemo > 0 {
		showLogin(false)
		if *loginDemo >= 2 {
			lg.started = true
			lg.step = meta.LoginStep{Kind: meta.LoginInput, Fields: []meta.LoginField{{ID: "username", Name: "Username"}, {ID: "password", Name: "Password", Secret: true}}}
			if *loginDemo == 3 {
				lg.err = "Logowanie się nie udało: nieprawidłowe hasło."
			}
			if *loginDemo == 4 { // the step that asks how to confirm the login
				lg.step = meta.LoginStep{Kind: meta.LoginInput, Instructions: "Choose how to finish signing in",
					Fields: []meta.LoginField{{ID: "mfatype", Name: "Login method", Options: []string{
						"Authentication app", "Text message", "Approve from another device", "Backup code"}}}}
			}
			loginFields()
		}
	}
	if *offlineFlag {
		be.armed.Store(true) // so that changes made to the test database announce themselves
		close(be.done)
	} else {
		be.start(hMain)
	}

	var m msgT
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if m.message == wmKeyDown && handleKey(&m) {
			continue
		}
		if r, _, _ := pIsDialogMessageW.Call(hMain, uintptr(unsafe.Pointer(&m))); r != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	be.stop()
}

// handleKey takes the keys that the dialog manager would otherwise swallow or misroute.
func handleKey(m *msgT) bool {
	ctrl := keyDown(vkControl)
	switch {
	case m.hwnd == hInput && m.wParam == vkReturn && !keyDown(vkShift) && !ctrl:
		if m.lParam&(1<<30) == 0 {
			doSend()
		}
		return true
	case ctrl && m.wParam == 'C' && m.hwnd == hView && selText() != "":
		copyText(selText())
		viewToast("Skopiowano zaznaczenie")
		return true
	case ctrl && m.wParam == 'V' && m.hwnd == hInput:
		if pasteAttachment() {
			return true
		}
	case ctrl && m.wParam == 'F' && selectedID != 0 && nav != navSettings && !lg.active:
		if pn.open && pn.search {
			pSetFocus.Call(pn.edit)
		} else {
			showPanel(true)
		}
		return true
	case m.hwnd == pn.edit && m.wParam == vkEscape:
		closePanel()
		return true
	case ctrl && m.wParam == 'K':
		if nav == navSettings {
			setNav(navChats)
		}
		pSetFocus.Call(hSearch)
		return true
	case dlg.field != 0 && m.hwnd == dlg.field && (m.wParam == vkReturn || m.wParam == vkEscape):
		closeDialog(m.wParam == vkReturn)
		return true
	case lg.active && m.wParam == vkReturn:
		for _, e := range lg.edits {
			if m.hwnd == e {
				loginNext()
				return true
			}
		}
	case pick.hwnd != 0 && m.wParam == vkEscape:
		closePicker()
		return true
	case m.hwnd == hInput && m.wParam == vkEscape && compose.mode != cmNone:
		cancelCompose()
		return true
	case lay.chat && m.wParam == vkEscape && (m.hwnd == hInput || m.hwnd == hView):
		goBack()
		return true
	case compact.on && m.wParam == vkEscape: // from the list of the panel: put the overlay away
		closeWindow()
		return true
	case m.hwnd == hSearch && m.wParam == vkEscape:
		if query != "" {
			setText(hSearch, "")
		} else {
			pSetFocus.Call(hList)
		}
		return true
	case m.hwnd == hSearch && (m.wParam == vkDown || m.wParam == vkReturn):
		if len(rows) > 0 {
			pSetFocus.Call(hList)
			if curSel() < 0 {
				sendMsg(hList, lbSetCurSel, 0, 0)
			}
			if nav == navChats || m.wParam == vkReturn {
				commitRow(curSel(), m.wParam == vkReturn)
			}
		}
		return true
	case m.hwnd == hList && m.wParam == vkReturn:
		commitRow(curSel(), true)
		return true
	case m.wParam == vkReturn:
		for i, h := range hNav {
			if m.hwnd == h {
				setNav(i)
				return true
			}
		}
	}
	return false
}

// onStrip reports whether a point is on the bar that leads to the settings.
func onStrip(x, y int32) bool {
	s := lay.strip
	return lay.stripClick && x >= s.left && x < s.right && y >= s.top && y < s.bottom
}

var (
	hotStrip bool   // the pointer is on that bar
	stripDo  func() // what a click on it does
)

func fatal(msg string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(msg))), uintptr(unsafe.Pointer(u16(appName))), 0x10)
	os.Exit(1)
}

func subclass(h uintptr, proc func(uintptr, uint32, uintptr, uintptr) uintptr) uintptr {
	old, _, _ := pSetWindowLongPtrW.Call(h, ^uintptr(3), syscall.NewCallback(proc)) // GWLP_WNDPROC
	return old
}

func createMainWindow(visible bool) {
	hMain = createWindow(0, "MliteMain", appName, wsOverlappedWindow|wsClipChildren, 0, 0)
	if hMain == 0 {
		fatal("Nie można utworzyć okna.")
	}
	if d, _, _ := pGetDpiForWindow.Call(hMain); d != 0 {
		dpi = int32(d)
	}
	loadFonts(hMain)
	measureFonts()
	pMoveWindow.Call(hMain, uintptr(dp(90)), uintptr(dp(60)), uintptr(dp(1020)), uintptr(dp(700)), 0)

	for i, label := range []string{"Rozmowy", "Osoby", "Ustawienia"} {
		hNav[i] = createWindow(0, "BUTTON", label, wsChild|wsVisible|wsTabStop|bsOwnerDraw, hMain, uintptr(idNavChats+i))
	}
	hSearch = createWindow(0, "EDIT", "", wsChild|wsVisible|wsTabStop|esAutoHScroll, hMain, idSearch)
	hList = createWindow(0, "MliteList", "Rozmowy", wsChild|wsVisible|wsTabStop, hMain, idList)
	hView = createWindow(0, "MliteView", "Wiadomości", wsChild|wsVisible|wsTabStop, hMain, 0)
	mv.hwnd = hView
	mv.sc = scroller{hwnd: hView, pos: &mv.scroll, total: func() int32 { return mv.total }, page: func() int32 { return mv.h }, changed: viewScrolled}
	mv.hover, mv.hotAct = -1, -1
	hInput = createWindow(0, "EDIT", "", wsChild|wsTabStop|esMultiline|esAutoVScroll|esWantReturn, hMain, idInput)
	hSend = createWindow(0, "BUTTON", "Wyślij", wsChild|wsTabStop|bsOwnerDraw, hMain, idSend)
	hAttach = createWindow(0, "BUTTON", "Dołącz plik", wsChild|wsTabStop|bsOwnerDraw, hMain, idAttach)
	// the two buttons of the conversation header are real buttons: Tab reaches them
	hFind = createWindow(0, "BUTTON", "Szukaj w rozmowie", wsChild|wsTabStop|bsOwnerDraw, hMain, idFind)
	hInfo = createWindow(0, "BUTTON", "Informacje o rozmowie", wsChild|wsTabStop|bsOwnerDraw, hMain, idInfo)
	hClose = createWindow(0, "BUTTON", "Zamknij panel", wsChild|wsTabStop|bsOwnerDraw, hMain, idClose)
	hSet = createWindow(0, "MliteSettings", "Ustawienia", wsChild|wsVScroll|wsTabStop, hMain, 0)
	initLogin()
	sv.hwnd = hSet
	sendMsg(hInput, emSetLimitText, 20000, 0)
	pEnableWindow.Call(hSend, 0)
	oldInputProc = subclass(hInput, inputProc)
	initList()
	initPanel()
	for _, h := range []uintptr{hNav[0], hNav[1], hNav[2], hSend, hAttach, hFind, hInfo, hClose} {
		oldBtnProc = subclass(h, btnProc)
	}
	applyTheme()

	setWindowIcons()
	icon := makeIcon(traySize(), 0, cfg.DND)
	if icon == 0 {
		icon, _, _ = pLoadIconW.Call(0, idiApp)
	}
	tray = notifyIconData{hWnd: hMain, uID: 1, uFlags: nifMsg | nifIcon | nifTip, uCallbackMessage: wmAppTray, hIcon: icon}
	tray.cbSize = uint32(unsafe.Sizeof(tray))
	copyU16(tray.szTip[:], appName)
	pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&tray)))

	layout()
	if v := os.Getenv("MLITE_LOGINPREVIEW"); v != "" { // for looking at the sign-in steps only
		loginPreview(v)
	}
	memMB = workingSetMB()
	pSetTimer.Call(hMain, timerMem, 5000, 0)
	if visible {
		pShowWindow.Call(hMain, swShow)
		pUpdateWindow.Call(hMain)
	}
}

// measureFonts records the real line height of the text used in the edit fields.
func measureFonts() {
	hdc, _, _ := pGetDC.Call(hMain)
	old, _, _ := pSelectObject.Call(hdc, fnt.body)
	var sz sizeT
	probe := u16s("Ąg")
	pGetTextExtentPoint32.Call(hdc, uintptr(unsafe.Pointer(&probe[0])), 2, uintptr(unsafe.Pointer(&sz)))
	pSelectObject.Call(hdc, old)
	pReleaseDC.Call(hMain, hdc)
	lay.editH = sz.cy
	if lay.editH < sp(16) {
		lay.editH = sp(20)
	}
}

// withFont runs fn on a screen canvas that has the font selected, for measuring text.
func withFont(font uintptr, fn func(c canvas)) {
	hdc, _, _ := pGetDC.Call(hMain)
	old, _, _ := pSelectObject.Call(hdc, font)
	dw.cur = font
	fn(canvas{hdc})
	dwEnd()
	pSelectObject.Call(hdc, old)
	pReleaseDC.Call(hMain, hdc)
}

func rowHeight() uintptr {
	if nav == navPeople {
		return uintptr(dp(16) + sp(40))
	}
	return uintptr(dp(24) + sp(40))
}

// applyTheme pushes the current palette and fonts into the native controls.
func applyTheme() {
	if brMica != 0 {
		pDeleteObject.Call(brMica)
		pDeleteObject.Call(brCard)
	}
	brMica, _, _ = pCreateSolidBrush.Call(cref(pal.mica))
	brCard, _, _ = pCreateSolidBrush.Call(cref(pal.card))
	dark := int32(0)
	scroll := "Explorer"
	if pal.dark {
		dark, scroll = 1, "DarkMode_Explorer"
	}
	caption, captionText := uint32(cref(pal.mica)), uint32(cref(pal.fg))
	pDwmSetWindowAttribute.Call(hMain, 20, uintptr(unsafe.Pointer(&dark)), 4)
	pDwmSetWindowAttribute.Call(hMain, 35, uintptr(unsafe.Pointer(&caption)), 4)     // caption colour
	pDwmSetWindowAttribute.Call(hMain, 36, uintptr(unsafe.Pointer(&captionText)), 4) // caption text
	for _, h := range []uintptr{hList, hView, hSet} {
		pSetWindowTheme.Call(h, uintptr(unsafe.Pointer(u16(scroll))), 0)
	}
	sendMsg(hSearch, wmSetFont, fnt.body, 1)
	sendMsg(hInput, wmSetFont, fnt.body, 1)
	sendMsg(pn.edit, wmSetFont, fnt.body, 1)
	sendMsg(hList, lbSetItemHeight, 0, rowHeight())
	setCue()
}

func setCue() {
	cue := "Szukaj rozmowy"
	if nav == navPeople {
		cue = "Szukaj osób w Messengerze"
	}
	sendMsg(hSearch, emSetCueBanner, 1, uintptr(unsafe.Pointer(u16(cue))))
}

// statusLine is the text of the strip above the list; empty while everything works.
func statusLine() string {
	if st := be.getStatus(); st != statusOK {
		return st
	}
	return ""
}

func layout() {
	var rc rect
	pGetClientRect.Call(hMain, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.right, rc.bottom
	lay.w, lay.h = w, h
	if dlg.hwnd != 0 { // a question covers everything; the window as it was is drawn dimmed behind it
		for _, c := range []uintptr{hNav[0], hNav[1], hNav[2], hSearch, hList, hView, hInput, hSend, hAttach, hFind, hInfo, hClose, hSet, pn.hwnd, lg.hwnd} {
			show(c, false)
		}
		moveTo(dlg.hwnd, rect{0, 0, w, h})
		return
	}
	if lg.active { // the sign-in screen covers everything else
		for _, c := range []uintptr{hNav[0], hNav[1], hNav[2], hSearch, hList, hView, hInput, hSend, hAttach, hFind, hInfo, hClose, hSet} {
			show(c, false)
		}
		moveTo(lg.hwnd, rect{0, 0, w, h})
		show(lg.hwnd, true)
		invalidate(hMain)
		return
	}
	show(lg.hwnd, false)
	if vw.hwnd != 0 { // a picture shown large covers everything else
		for _, c := range []uintptr{hNav[0], hNav[1], hNav[2], hSearch, hList, hView, hInput, hSend, hAttach, hFind, hInfo, hClose, hSet} {
			show(c, false)
		}
		moveTo(vw.hwnd, rect{0, 0, w, h})
		return
	}
	settings := nav == navSettings
	hasSel := selectedID != 0
	// A narrow window shows one column: the list with the navigation along the bottom,
	// or the conversation alone with a way back.
	narrow := w < dp(720)
	chat := narrow && inChat && hasSel && !settings
	lay.narrow, lay.chat = narrow, chat
	showPane := !settings && !chat
	showContent := !settings && (!narrow || chat)
	full := w
	pn.side = pn.open && hasSel && showContent && !narrow && w >= dp(1040)
	pn.cover = pn.open && hasSel && showContent && !pn.side
	if pn.side {
		w -= panelWidth()
		lay.w = w
	}

	railW, paneW := dp(72), dp(296)
	withFont(fnt.small, func(c canvas) {
		if lw := c.width("Ustawienia") + dp(14); lw > railW {
			railW = lw
		}
	})
	if w < dp(900) {
		paneW = dp(248)
	}
	editH := lay.editH
	navH := dp(34) + sp(20)
	searchH := editH + dp(12)
	if searchH < dp(32) {
		searchH = dp(32)
	}
	hdrH := dp(18) + sp(38)
	if hdrH < dp(56) {
		hdrH = dp(56)
	}
	barH, paneLeft := int32(0), railW
	if narrow {
		railW, paneLeft, paneW = 0, 0, w
		if !chat {
			barH = navH + dp(8)
		}
	}
	lay.rail = rect{0, 0, railW, h}
	lay.pane = rect{paneLeft, 0, paneLeft + paneW, h - barH}
	lay.content = rect{paneLeft + paneW, 0, w, h}
	cx := lay.content.left + 1
	if narrow {
		lay.content, cx = rect{0, 0, w, h}, 0
	}
	lay.navBar = rect{0, h - barH, w, h}
	searchTop := dp(14) + sp(34)
	lay.search = rect{lay.pane.left + dp(12), searchTop, lay.pane.right - dp(12), searchTop + searchH}

	lay.stripText, lay.stripClick, stripDo = statusLine(), false, nil
	if lay.stripText == "" && !settings {
		if n := updateNotice(); n != "" {
			lay.stripText, lay.stripClick, stripDo = n, true, goToUpdates
		}
	}
	lay.strip = rect{}
	listTop := lay.search.bottom + dp(10)
	viewTop := hdrH + 1
	if lay.stripText != "" {
		left, right := lay.pane.left+dp(12), lay.pane.right-dp(12)
		if chat {
			left, right = dp(12), w-dp(12)
		}
		var th int32
		withFont(fnt.small, func(c canvas) { _, th = c.measure(lay.stripText, right-left-dp(20)) })
		if max := 3 * sp(16); th > max {
			th = max
		}
		if chat { // in the conversation column the notice sits under the header
			lay.strip = rect{left, hdrH + dp(8), right, hdrH + dp(8) + th + dp(14)}
			viewTop = lay.strip.bottom + dp(6)
		} else {
			top := lay.search.bottom + dp(8)
			lay.strip = rect{left, top, right, top + th + dp(14)}
			listTop = lay.strip.bottom + dp(6)
		}
	}

	inH := int32(compLines) * editH
	fieldH := inH + dp(16)
	if fieldH < dp(36) {
		fieldH = dp(36)
	}
	compH := fieldH + dp(20)
	lay.bar = rect{}
	if compose.mode != cmNone && hasSel {
		barH := sp(36)
		if composeThumb() != 0 { // a picture waiting to be sent shows itself
			barH = composeThumbSide() + dp(8)
		}
		compH += barH + dp(6)
		lay.bar = rect{cx + dp(12), h - compH + dp(8), w - dp(12), h - compH + dp(8) + barH}
		compose.closeHit = rect{lay.bar.right - dp(32), lay.bar.top, lay.bar.right, lay.bar.bottom}
	}
	lay.header = rect{cx, 0, w, hdrH}
	lay.hbInfo = rect{w - dp(10) - dp(36), (hdrH - dp(36)) / 2, w - dp(10), (hdrH + dp(36)) / 2}
	lay.hbSearch = rect{lay.hbInfo.left - dp(4) - dp(36), lay.hbInfo.top, lay.hbInfo.left - dp(4), lay.hbInfo.bottom}
	lay.hx, lay.back = 0, rect{}
	if chat && !compact.on {
		lay.hx, lay.back = dp(34), rect{0, 0, dp(46), hdrH}
	}
	lay.comp = rect{cx, h - compH, w, h}
	lay.input = rect{cx + dp(12) + dp(40), h - dp(10) - fieldH, w - dp(12) - dp(36) - dp(8), h - dp(10)}

	if narrow {
		third := w / 3
		for i := int32(0); i < 3; i++ {
			moveTo(hNav[i], rect{i*third + dp(4), h - barH + dp(4), (i+1)*third - dp(4), h - dp(4)})
		}
	} else {
		for i := int32(0); i < 2; i++ {
			top := dp(6) + i*(navH+dp(4))
			moveTo(hNav[i], rect{dp(4), top, railW - dp(4), top + navH})
		}
		setBottom := h - dp(10) - sp(22)
		moveTo(hNav[2], rect{dp(4), setBottom - navH, railW - dp(4), setBottom})
	}
	st := lay.search.top + (searchH-editH)/2
	moveTo(hSearch, rect{lay.search.left + dp(32), st, lay.search.right - dp(8), st + editH})
	moveTo(hList, rect{lay.pane.left, listTop, lay.pane.right, lay.pane.bottom})
	if hasSel {
		moveTo(hView, rect{cx, viewTop, w, h - compH - 1})
	} else {
		moveTo(hView, rect{cx, 0, w, h})
	}
	it := lay.input.top + (fieldH-inH)/2
	moveTo(hInput, rect{lay.input.left + dp(10), it, lay.input.right - dp(8), it + inH})
	moveTo(hSend, rect{w - dp(12) - dp(36), h - dp(10) - dp(36), w - dp(12), h - dp(10)})
	moveTo(hAttach, rect{cx + dp(10), h - dp(10) - dp(36), cx + dp(46), h - dp(10)})
	moveTo(hFind, lay.hbSearch)
	moveTo(hInfo, lay.hbInfo)
	moveTo(hClose, lay.hbInfo)
	moveTo(hSet, rect{railW, 0, w, h - barH})
	switch {
	case pn.side:
		moveTo(pn.hwnd, rect{w, 0, full, h})
	case pn.cover:
		moveTo(pn.hwnd, rect{cx, 0, full, h})
	}

	for _, nb := range hNav {
		show(nb, !chat)
	}
	show(hSearch, showPane)
	show(hList, showPane)
	show(hView, showContent && !pn.cover)
	show(hInput, showContent && hasSel && !pn.cover)
	show(hSend, showContent && hasSel && !pn.cover)
	show(hAttach, showContent && hasSel && !pn.cover)
	show(hFind, showContent && hasSel && !pn.cover && !compact.on)
	show(hInfo, showContent && hasSel && !pn.cover && !compact.on)
	show(hClose, showContent && hasSel && compact.on)
	invalidate(hFind)
	invalidate(hInfo)
	invalidate(hClose)
	show(pn.hwnd, pn.side || pn.cover)
	show(pn.edit, (pn.side || pn.cover) && pn.search)
	show(hSet, settings)
	invalidate(hMain)
}

// goBack leaves the conversation column of the narrow layout for the list.
func goBack() {
	if !lay.chat {
		return
	}
	inChat = false
	layout()
	pSetFocus.Call(hList)
}

func setNav(n int) {
	if nav == n {
		return
	}
	nav = n
	inChat = false
	hotRow = -1
	query = ""
	setText(hSearch, "")
	setCue()
	sendMsg(hList, lbSetItemHeight, 0, rowHeight())
	listKey = 0
	layout()
	refresh()
	if n == navSettings {
		invalidate(hSet)
	}
	for _, h := range hNav {
		invalidate(h)
	}
}

func paintMain(target uintptr) {
	buf := newBuffer(target, lay.w, lay.h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, lay.w, lay.h}, pal.mica)
	settings := nav == navSettings
	showPane := !settings && !lay.chat
	showContent := !settings && (!lay.narrow || lay.chat)
	t, has := threadByID(selectedID)
	has = has && showContent
	hdrPhoto := false
	hx := lay.hx
	if showContent {
		c.fill(lay.content, pal.layer)
		if !lay.narrow {
			c.fill(rect{lay.content.left, 0, lay.content.left + 1, lay.h}, pal.line)
		}
		if has {
			c.fill(lay.header, pal.card)
			c.fill(rect{lay.header.left, lay.header.bottom, lay.w, lay.header.bottom + 1}, pal.line)
			c.fill(lay.comp, pal.card)
			c.fill(rect{lay.comp.left, lay.comp.top - 1, lay.w, lay.comp.top}, pal.line)
		}
	}
	if lay.narrow && !lay.chat {
		c.fill(rect{0, lay.navBar.top, lay.w, lay.navBar.top + 1}, pal.line)
	}
	if !settings {
		g := newGfx(buf.hdc)
		field := func(r rect, focused bool) {
			// a field of the identity: the control corner all round, a quiet edge, and the focus
			// colour two pixels wide while the keyboard is in it
			edge, inset := mix(pal.fg2, pal.line, 65), int32(1) // 3:1 against the field, so its outline can be seen
			if focused {
				edge, inset = pal.accent, 2
			}
			rr := dp(8)
			g.round(r.left, r.top, r.right-r.left, r.bottom-r.top, rr, rr, rr, rr, edge)
			g.round(r.left+inset, r.top+inset, r.right-r.left-2*inset, r.bottom-r.top-2*inset, rr-inset, rr-inset, rr-inset, rr-inset, pal.card)
		}
		if showPane {
			field(lay.search, focusEdit == hSearch)
		}
		if has {
			field(lay.input, focusEdit == hInput)
			if compose.mode != cmNone {
				g.pill(lay.bar.left, lay.bar.top+dp(2), dp(3), lay.bar.bottom-lay.bar.top-dp(4), pal.accent)
				if img := composeThumb(); img != 0 {
					s := composeThumbSide()
					g.image(img, lay.bar.left+dp(12), (lay.bar.top+lay.bar.bottom-s)/2, s, s)
				}
			}
			hdrPhoto = threadAvatar(g, t, lay.header.left+hx+dp(14), (lay.header.bottom-dp(36))/2, dp(36), pal.card)
		}
		if lay.stripText != "" {
			s := lay.strip
			fill := mix(pal.accent, pal.mica, 16)
			if lay.stripClick {
				fill = mix(pal.accent, pal.mica, 30)
				if hotStrip {
					fill = mix(pal.accent, pal.mica, 44)
				}
			}
			g.round(s.left, s.top, s.right-s.left, s.bottom-s.top, dp(8), dp(8), dp(8), dp(8), fill)
		}
		g.free()

		if showPane {
			c.font(fnt.title)
			title := "Rozmowy"
			if nav == navPeople {
				title = "Osoby"
			}
			c.text(title, rect{lay.pane.left + dp(16), dp(8), lay.pane.right - dp(12), dp(8) + sp(34)}, pal.fg, dtSingleLine|dtVCenter)
			c.font(fnt.icon)
			c.text(gSearch, rect{lay.search.left + dp(8), lay.search.top, lay.search.left + dp(30), lay.search.bottom}, pal.fg2, dtSingleLine|dtVCenter)
		}
		if lay.stripText != "" {
			s := lay.strip
			right := s.right - dp(10)
			if lay.stripClick {
				right -= dp(20)
				c.font(fnt.iconSm)
				c.text(gChevron, rect{s.right - dp(26), s.top, s.right - dp(8), s.bottom}, pal.accentText, dtCenter|dtVCenter|dtSingleLine)
			}
			c.font(fnt.small)
			c.text(lay.stripText, rect{s.left + dp(10), s.top + dp(7), right, s.bottom - dp(7)}, pal.fg, dtWordBreak|dtEditControl|dtEndEllipsis)
		}
		if has && compose.mode != cmNone {
			b := lay.bar
			title, excerpt := composeLabels()
			mid := (b.top + b.bottom) / 2
			tx := b.left + dp(12)
			if composeThumb() != 0 {
				tx += composeThumbSide() + dp(12)
			}
			c.font(fnt.smallSemi)
			c.text(title, rect{tx, mid - sp(17), b.right - dp(40), mid}, pal.accentText, dtSingleLine|dtVCenter|dtEndEllipsis)
			c.font(fnt.small)
			c.text(excerpt, rect{tx, mid, b.right - dp(40), mid + sp(17)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
			c.font(fnt.iconSm)
			c.text(gClose, compose.closeHit, pal.fg2, dtCenter|dtVCenter|dtSingleLine)
		}
		if has {
			hh := lay.header.bottom
			if lay.chat {
				c.font(fnt.icon)
				c.text(gBack, lay.back, pal.fg, dtCenter|dtVCenter|dtSingleLine)
			}
			if !hdrPhoto {
				c.font(fnt.avSm)
				c.text(initials(threadTitle(t)), rect{lay.header.left + hx + dp(14), (hh - dp(36)) / 2, lay.header.left + hx + dp(50), (hh + dp(36)) / 2}, pal.fg, dtCenter|dtVCenter|dtSingleLine)
			}
			sub := ""
			switch {
			case isGroup(t):
				sub = "Rozmowa grupowa"
			case isE2EE(t):
				sub = "Rozmowa szyfrowana"
			}
			if real := realTitle(t); real != threadTitle(t) { // shown under a nickname
				if sub == "" {
					sub = real
				} else {
					sub = real + " · " + sub
				}
			}
			x := lay.header.left + hx + dp(62)
			// the panel of the overlay has one button in its header, the window has two
			titleRight := lay.hbSearch.left - dp(8)
			if compact.on {
				titleRight = lay.hbInfo.left - dp(8)
			}
			c.font(fnt.head)
			if sub == "" {
				c.text(threadTitle(t), rect{x, 0, titleRight, hh}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
			} else {
				top := (hh - sp(38)) / 2
				c.text(threadTitle(t), rect{x, top, titleRight, top + sp(20)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
				c.font(fnt.small)
				c.text(sub, rect{x, top + sp(20), titleRight, top + sp(38)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
			}
		}
	}
	if cfg.ShowMem && !lay.narrow {
		c.font(fnt.small)
		c.text(fmt.Sprintf("%d MB", memMB), rect{0, lay.h - dp(8) - sp(22), lay.rail.right, lay.h - dp(8)}, pal.fg2, dtCenter|dtVCenter|dtSingleLine)
	}
	buf.flush(0, 0)
}

func focusCue(d *drawItem) bool {
	return d.itemState&odsFocus != 0 && d.itemState&odsNoFocusRect == 0
}

func drawNav(d *drawItem, i int) {
	w, h := d.rcItem.right, d.rcItem.bottom
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.mica)
	current := nav == i
	bg := pal.mica
	switch {
	case d.itemState&odsSelected != 0:
		bg = mix(pal.fg, pal.mica, 12)
	case current:
		bg = pal.hover
	case hotBtn == d.hwndItem:
		bg = mix(pal.fg, pal.mica, 5)
	}
	g := newGfx(buf.hdc)
	if bg != pal.mica {
		g.round(0, 0, w, h, dp(6), dp(6), dp(6), dp(6), bg)
	}
	if current && lay.narrow {
		g.pill(w/2-dp(9), h-dp(3), dp(18), dp(3), pal.accent)
	} else if current {
		g.pill(0, h/2-dp(9), dp(3), dp(18), pal.accent)
	}
	g.free()
	glyph := []string{gChat, gPeople, gSettings}[i]
	label := []string{"Rozmowy", "Osoby", "Ustawienia"}[i]
	ic, tc := pal.fg2, pal.fg2
	if current {
		ic, tc = pal.accent, pal.fg
	}
	c.font(fnt.iconLg)
	c.text(glyph, rect{0, dp(7), w, dp(31)}, ic, dtCenter|dtVCenter|dtSingleLine)
	c.font(fnt.small)
	c.text(label, rect{0, dp(32), w, h - dp(4)}, tc, dtCenter|dtVCenter|dtSingleLine)
	if i == navChats && unreadTotal > 0 {
		s := fmt.Sprint(unreadTotal)
		if unreadTotal > 99 {
			s = "99+"
		}
		c.font(fnt.smallSemi)
		bw := c.width(s) + dp(10)
		if bw < dp(18) {
			bw = dp(18)
		}
		g := newGfx(buf.hdc)
		g.pill(w/2+dp(4), dp(2), bw, dp(18), pal.accent)
		g.free()
		c.text(s, rect{w/2 + dp(4), dp(2), w/2 + dp(4) + bw, dp(20)}, pal.onAccent, dtCenter|dtVCenter|dtSingleLine)
	}
	if focusCue(d) {
		fr := rect{2, 2, w - 2, h - 2}
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(0, 0)
}

func drawSend(d *drawItem) {
	w, h := d.rcItem.right, d.rcItem.bottom
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.card)
	fill, ink := pal.accent, pal.onAccent
	switch {
	case d.itemState&odsDisabled != 0:
		fill, ink = mix(pal.fg, pal.card, 8), pal.fg2
	case d.itemState&odsSelected != 0:
		fill = pal.accentDown
	case hotBtn == d.hwndItem:
		fill = pal.accentHover
	}
	g := newGfx(buf.hdc)
	g.round(0, 0, w, h, dp(8), dp(8), dp(8), dp(8), fill)
	g.free()
	c.font(fnt.icon)
	c.text(gSend, rect{0, 0, w, h}, ink, dtCenter|dtVCenter|dtSingleLine)
	if focusCue(d) {
		fr := rect{3, 3, w - 3, h - 3}
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(0, 0)
}

func drawRow(d *drawItem) {
	w, h := d.rcItem.right-d.rcItem.left, d.rcItem.bottom-d.rcItem.top
	if int(d.itemID) >= len(rows) || w <= 0 || h <= 0 {
		canvas{d.hDC}.fill(d.rcItem, pal.mica)
		return
	}
	row := rows[d.itemID]
	people := nav == navPeople
	now := time.Now()
	muted := row.person == nil && (row.t.Muted(now) || quietThread(row.t.ID))
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.mica)
	selected := !people && row.t.ID == selectedID
	if people || lay.narrow {
		selected = d.itemState&odsSelected != 0
	}
	g := newGfx(buf.hdc)
	switch {
	case selected:
		g.round(dp(6), dp(2), w-dp(12), h-dp(4), dp(8), dp(8), dp(8), dp(8), pal.sel)
		g.pill(dp(6), h/2-dp(9), dp(3), dp(18), pal.accent)
	case int(d.itemID) == hotRow:
		g.round(dp(6), dp(2), w-dp(12), h-dp(4), dp(8), dp(8), dp(8), dp(8), mix(pal.fg, pal.mica, 5))
	}
	bg := pal.mica
	if selected {
		bg = pal.card
	} else if int(d.itemID) == hotRow {
		bg = mix(pal.fg, pal.mica, 5)
	}
	av := dp(44)
	if people {
		av = dp(40)
	}
	ax, ay := dp(16), (h-av)/2
	id := row.t.ID
	if row.person != nil {
		id = row.person.ID
	}
	var photo bool
	if row.person == nil {
		photo = threadAvatar(g, row.t, ax, ay, av, bg)
	} else {
		photo = g.avatar(id, ax, ay, av)
	}
	badgeW, badgeH := int32(0), sp(16)+dp(4)
	badgeY := h/2 - sp(20) + sp(21) + (sp(19)-badgeH)/2
	badge := ""
	if row.unread > 0 && !people {
		badge = fmt.Sprint(row.unread)
		if row.unread > 99 {
			badge = "99+"
		}
		c.font(fnt.smallSemi)
		badgeW = c.width(badge) + dp(12)
		if badgeW < badgeH {
			badgeW = badgeH
		}
		fill := pal.accent
		if muted {
			fill = pal.fg2
		}
		g.pill(w-dp(16)-badgeW, badgeY, badgeW, badgeH, fill)
	}
	seenW := int32(0) // small pictures of those who have read our last message
	if badgeW == 0 && !people {
		for k, who := range row.seen {
			if k == 2 {
				break
			}
			g.avatar(who, w-dp(16)-int32(k+1)*dp(18)+dp(2), badgeY+(badgeH-dp(16))/2, dp(16))
			seenW = int32(k+1) * dp(18)
		}
	}
	g.free()

	if !photo {
		c.font(fnt.av)
		c.text(initials(row.title), rect{ax, ay, ax + av, ay + av}, pal.fg, dtCenter|dtVCenter|dtSingleLine)
	}
	x0, x1 := ax+av+dp(12), w-dp(16)
	if people {
		c.font(fnt.name)
		if row.sub == "" {
			c.text(row.title, rect{x0, 0, x1, h}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
		} else {
			top := h/2 - sp(19)
			c.text(row.title, rect{x0, top, x1, top + sp(20)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
			c.font(fnt.small)
			c.text(row.sub, rect{x0, top + sp(20), x1, top + sp(38)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
		}
	} else {
		top := h/2 - sp(20)
		c.font(fnt.small)
		if row.unread > 0 {
			c.font(fnt.smallSemi)
		}
		tw := c.width(row.when)
		timeCol := pal.fg2
		if row.unread > 0 && !muted {
			timeCol = pal.accentText
		}
		c.text(row.when, rect{x1 - tw, top, x1, top + sp(20)}, timeCol, dtSingleLine|dtVCenter)
		nameRight := x1 - tw - dp(8)
		if muted {
			c.font(fnt.iconSm)
			c.text(gMute, rect{nameRight - dp(16), top, nameRight, top + sp(20)}, pal.fg2, dtSingleLine|dtVCenter)
			nameRight -= dp(20)
		}
		if row.unread > 0 {
			c.font(fnt.nameBold)
		} else {
			c.font(fnt.name)
		}
		c.text(row.title, rect{x0, top, nameRight, top + sp(20)}, pal.fg, dtSingleLine|dtVCenter|dtEndEllipsis)
		c.font(fnt.prev)
		right := x1
		if badgeW > 0 {
			right -= badgeW + dp(8)
		} else if seenW > 0 {
			right -= seenW + dp(6)
		}
		px := x0
		if row.icon != "" {
			c.font(fnt.iconSm)
			c.text(row.icon, rect{x0, top + sp(21), x0 + dp(16), top + sp(40)}, pal.fg2, dtSingleLine|dtVCenter)
			c.font(fnt.prev)
			px += dp(20)
		}
		c.text(row.preview, rect{px, top + sp(21), right, top + sp(40)}, pal.fg2, dtSingleLine|dtVCenter|dtEndEllipsis)
		if badge != "" {
			c.font(fnt.smallSemi)
			col := pal.onAccent
			if muted {
				col = pal.mica
			}
			c.text(badge, rect{w - dp(16) - badgeW, badgeY, w - dp(16), badgeY + badgeH}, col, dtCenter|dtVCenter|dtSingleLine)
		}
	}
	if focusCue(d) {
		fr := rect{dp(7), dp(3), w - dp(7), h - dp(3)}
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(d.rcItem.left, d.rcItem.top)
}

// ---- subclassed controls

type trackMouse struct {
	cbSize, flags uint32
	hwnd          uintptr
	hover         uint32
}

func trackLeave(h uintptr) {
	t := trackMouse{flags: 2, hwnd: h}
	t.cbSize = uint32(unsafe.Sizeof(t))
	pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&t)))
}

// btnProc adds pointer feedback to the owner-drawn buttons and keeps fast clicks from being lost.
func btnProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmMouseMove:
		if hotBtn != hwnd {
			old := hotBtn
			hotBtn = hwnd
			invalidate(old)
			invalidate(hwnd)
			trackLeave(hwnd)
		}
	case wmMouseLeave:
		if hotBtn == hwnd {
			hotBtn = 0
			invalidate(hwnd)
		}
	case wmLButtonDblClk:
		msg = wmLButtonDown
	}
	r, _, _ := pCallWindowProcW.Call(oldBtnProc, hwnd, uintptr(msg), wp, lp)
	return r
}

func setHotRow(i int) {
	if i != hotRow {
		old := hotRow
		hotRow = i
		invalidateRow(old)
		invalidateRow(i)
	}
}

func paintEmptyList() {
	msg := "Nie ma jeszcze rozmów"
	switch {
	case nav == navPeople && searchBusy:
		msg = "Szukam w Messengerze…"
	case nav == navPeople && query != "":
		msg = "Nikogo nie znaleziono"
	case nav == navPeople:
		msg = "Wpisz imię lub nazwisko, aby znaleźć osobę"
	case query != "":
		msg = "Żadna rozmowa nie pasuje"
	}
	var rc rect
	pGetClientRect.Call(hList, uintptr(unsafe.Pointer(&rc)))
	hdc, _, _ := pGetDC.Call(hList)
	old, _, _ := pSelectObject.Call(hdc, fnt.prev)
	dw.cur = fnt.prev
	canvas{hdc}.text(msg, rect{dp(16), dp(20), rc.right - dp(16), dp(20) + sp(60)}, pal.fg2, dtCenter|dtWordBreak)
	dwEnd()
	pSelectObject.Call(hdc, old)
	pReleaseDC.Call(hList, hdc)
}

// inputProc draws the placeholder that multi-line edit controls do not offer themselves.
func inputProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := pCallWindowProcW.Call(oldInputProc, hwnd, uintptr(msg), wp, lp)
	if msg == wmPaint && inputEmpty {
		var rc rect
		sendMsg(hwnd, emGetRect, 0, uintptr(unsafe.Pointer(&rc)))
		hdc, _, _ := pGetDC.Call(hwnd)
		old, _, _ := pSelectObject.Call(hdc, fnt.body)
		canvas{hdc}.gdiText("Napisz wiadomość…", rc, pal.fg2, dtSingleLine)
		pSelectObject.Call(hdc, old)
		pReleaseDC.Call(hwnd, hdc)
	}
	// On a change of focus the field redraws its line by itself, outside painting, and
	// wipes the placeholder: ask for a repaint so that it comes back.
	if (msg == wmSetFocus || msg == wmKillFocus) && inputEmpty {
		pInvalidateRect.Call(hwnd, 0, 1)
	}
	return r
}

// inputChanged keeps the composer height, the send button and the placeholder in step with the text.
func inputChanged() {
	n, _, _ := pGetWindowTextLengthW.Call(hInput)
	empty := n == 0
	if empty != inputEmpty {
		inputEmpty = empty
		on := uintptr(1)
		if empty {
			on = 0
		}
		pEnableWindow.Call(hSend, on)
		pInvalidateRect.Call(hInput, 0, 1)
	}
	sendOn := uintptr(0)
	if !empty || compose.mode == cmFile {
		sendOn = 1
	}
	pEnableWindow.Call(hSend, sendOn)
	lines := int(sendMsg(hInput, emGetLineCount, 0, 0))
	if lines < 1 {
		lines = 1
	}
	if lines > maxInputLines {
		lines = maxInputLines
	}
	if lines != compLines {
		compLines = lines
		layout()
		sendMsg(hInput, emLineScroll, 0, ^uintptr(999))
		sendMsg(hInput, emScrollCaret, 0, 0)
	}
}

func wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if msg == wmNcCalcSize && compact.on && wp != 0 {
		return 0 // the whole window is its content; the frame stays only for the rounding
	}
	if msg == wmNcHitTest && compact.on {
		// The panel belongs to its head: it is moved by the head and by nothing else, so
		// every point of it is content — no title bar to drag, no edges to pull.
		return htClient
	}
	if compact.on {
		// The panel has no frame of its own. Whatever happens to the styles, the system
		// must never draw a title bar or a border over what the panel has painted.
		switch msg {
		case wmNcPaint:
			return 0
		case wmNcActivate:
			return 1
		}
	}
	if msg == showMsg && showMsg != 0 { // a second start of the program asks this one to come forward
		showMain()
		return 0
	}
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		paintMain(hdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSize:
		if hList != 0 {
			layout()
		}
		overlayRound() // the panel of the overlay keeps its rounded corners
		return 0
	case wmGetMinMaxInfo:
		mm := (*minMaxInfo)(unsafe.Pointer(lp))
		mm.minTrack = point{dp(340), dp(440)}
		return 0
	case wmMeasureItem:
		mi := (*measureItem)(unsafe.Pointer(lp))
		mi.itemHeight = uint32(rowHeight())
		return 1
	case wmDrawItem:
		d := (*drawItem)(unsafe.Pointer(lp))
		switch {
		case d.ctlID >= idNavChats && d.ctlID <= idNavSettings:
			drawNav(d, int(d.ctlID-idNavChats))
		case d.ctlID == idSend:
			drawSend(d)
		case d.ctlID == idAttach:
			drawAttach(d)
		case d.ctlID == idFind || d.ctlID == idInfo || d.ctlID == idClose:
			drawHeaderBtn(d)
		case d.ctlID == idList:
			drawRow(d)
		}
		return 1
	case wmCtlColorEdit:
		pSetTextColor.Call(wp, cref(pal.fg))
		pSetBkColor.Call(wp, cref(pal.card))
		return brCard
	case wmCtlColorLB:
		return brMica
	case 0x0135: // WM_CTLCOLORBTN: what a button is erased with before it is drawn
		if lp == hSend || lp == hAttach || lp == hFind || lp == hInfo || lp == hClose {
			return brCard
		}
		return brMica
	case wmCommand:
		id, code := wp&0xFFFF, (wp>>16)&0xFFFF
		switch {
		case id >= idNavChats && id <= idNavSettings && code == 0:
			setNav(int(id - idNavChats))
		case id == idList && code == lbnSelChange:
			if nav == navChats && !lay.narrow { // in one column a conversation opens on release or Enter
				commitRow(curSel(), false)
			}
		case id >= idMsgReply && id <= idMsgUnreact && code == 0:
			messageAction(int(id))
		case (id == idRowPin || id == idRowUnpin) && code == 0:
			if t, ok := threadByID(selectedID); ok {
				rowAction(int(id), t)
			}
		case (id == idFind || id == idInfo) && code == 0:
			showPanel(id == idFind)
		case id == idClose && code == 0:
			hideOverlay()
		case id == idAttach && code == 0:
			pickFile()
		case id == idSend && code == 0:
			doSend()
			pSetFocus.Call(hInput)
		case id == idSearch && code == enChange:
			if q := getText(hSearch); q != query {
				query = q
				listKey = 0
				refresh()
				if nav == navPeople {
					pSetTimer.Call(hMain, timerSearch, 400, 0)
				}
			}
		case id == idInput && code == enChange:
			inputChanged()
		case (id == idSearch || id == idInput) && (code == enSetFocus || code == enKillFocus):
			focusEdit = 0
			if code == enSetFocus {
				focusEdit = lp
			}
			invalidate(hMain)
		}
		return 0
	case wmTimer:
		switch wp {
		case timerOpen:
			pKillTimer.Call(hMain, timerOpen)
			if t, ok := threadByID(selectedID); ok {
				be.openThread(t, isForeground())
			}
		case timerNote:
			pKillTimer.Call(hMain, timerNote)
			if statusLine() != lay.stripText {
				layout()
			}
		case timerSearch:
			pKillTimer.Call(hMain, timerSearch)
			if q := strings.TrimSpace(query); nav == navPeople && len([]rune(q)) >= 2 {
				be.searchPeople(q)
			}
		case timerMem:
			if mb := workingSetMB(); mb != memMB {
				memMB = mb
				if cfg.ShowMem {
					r := rect{0, lay.h - dp(10) - sp(22), lay.rail.right, lay.h}
					pInvalidateRect.Call(hMain, uintptr(unsafe.Pointer(&r)), 0)
				}
			}
		}
		return 0
	case wmSettingChange, wmDpiChanged:
		oldPal, oldScale := pal, textScale
		if msg == wmDpiChanged {
			dpi = int32(wp >> 16 & 0xFFFF)
			if r := (*rect)(unsafe.Pointer(lp)); r != nil {
				pMoveWindow.Call(hwnd, uintptr(r.left), uintptr(r.top), uintptr(r.right-r.left), uintptr(r.bottom-r.top), 1)
			}
		}
		loadPalette()
		if msg == wmSettingChange && pal == oldPal && textScale == oldScale {
			return 0
		}
		loadFonts(hwnd)
		avatarsDrop()
		mediaDropThumbs()
		measureFonts()
		applyTheme()
		setWindowIcons()
		trayRefresh()
		headsRedraw()
		layout()
		layoutSettings()
		listKey = 0
		refresh()
		loadView(true)
		for _, h := range []uintptr{hNav[0], hNav[1], hNav[2], hSend, hList, hSet, hSearch, hInput} {
			invalidate(h)
		}
		return 0
	case wmAppRefresh:
		be.refreshPending.Store(false)
		refresh()
		return 0
	case wmAppStatus:
		if statusLine() != lay.stripText {
			layout()
		}
		pSetTimer.Call(hMain, timerNote, 8300, 0) // a notice leaves by itself
		return 0
	case wmLButtonUp:
		x, y := int32(int16(lp)), int32(int16(lp>>16))
		if h := compose.closeHit; compose.mode != cmNone && x >= h.left && x < h.right && y >= h.top && y < h.bottom {
			cancelCompose()
		} else if b := lay.back; lay.chat && x >= b.left && x < b.right && y >= b.top && y < b.bottom {
			goBack()
		} else if onStrip(x, y) && stripDo != nil {
			stripDo()
		}
		return 0
	case wmMouseMove:
		if hot := onStrip(int32(int16(lp)), int32(int16(lp>>16))); hot != hotStrip {
			hotStrip = hot
			pInvalidateRect.Call(hMain, uintptr(unsafe.Pointer(&lay.strip)), 0)
			if hot {
				trackLeave(hMain)
			}
		}
		return 0
	case wmMouseLeave:
		if hotStrip {
			hotStrip = false
			pInvalidateRect.Call(hMain, uintptr(unsafe.Pointer(&lay.strip)), 0)
		}
		return 0
	case wmAppLogin:
		showLogin(wp == loginExpired)
		return 0
	case wmAppLoginStep:
		loginStepArrived()
		return 0
	case wmAppHistory:
		historyChecked()
		return 0
	case wmAppAvatar:
		avatarArrived(int64(lp), wp != 0)
		return 0
	case wmAppMedia:
		if ogArrived() {
			relayoutView()
		}
		mediaArrived()
		viewerRefresh()
		if pn.open {
			invalidate(pn.hwnd)
		}
		return 0
	case wmSetCursor:
		if lp&0xFFFF == 1 && hotStrip {
			cur, _, _ := pLoadCursorW.Call(0, idcHand)
			pSetCursor.Call(cur)
			return 1
		}
	case wmAppUpdate:
		layout()         // the bar about a new version comes and goes
		layoutSettings() // the row grows and shrinks with its text
		clampSettingsScroll()
		invalidate(hSet)
		return 0
	case wmAppToast:
		toastArrived()
		return 0
	case wmAppOlder:
		olderArrived(int64(lp), wp)
		return 0
	case wmAppQuit: // the installer or uninstaller asks a running copy to end
		quitting = true
		pDestroyWindow.Call(hMain)
		return 0
	case wmAppOpen:
		refresh()
		if t, ok := threadByID(int64(lp)); ok {
			selectThread(t, true)
		}
		return 0
	case wmAppTray:
		switch lp & 0xFFFF {
		case wmLButtonUp:
			showMain()
		case ninBalloonCk: // a click on the notification opens the conversation it was about
			showMain()
			if t, ok := threadByID(notifyID); ok {
				selectThread(t, true)
			}
		case wmRButtonUp:
			trayMenu()
		}
		return 0
	case wmActivate:
		if wp&0xFFFF != 0 && hInput != 0 && nav != navSettings {
			if selectedID != 0 && (!lay.narrow || lay.chat) {
				pSetFocus.Call(hInput)
			}
			markSelectedRead()
		}
		return 0
	case wmClose:
		if quitting {
			break
		}
		closeWindow()
		return 0
	case wmDestroy:
		toastQuit()
		headsClose()
		pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&tray)))
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func closeWindow() {
	if !cfg.Background {
		quitting = true
		pDestroyWindow.Call(hMain)
		return
	}
	closeViewer()
	pShowWindow.Call(hMain, swHide)
	dwRelease() // nothing is drawn while hidden; give the text engine's memory back
	debug.FreeOSMemory()
	avatarsDrop()
	mediaDropThumbs()
	if !hintShown {
		hintShown = true
		balloon(appName+" działa w tle", "Kliknij ikonę w zasobniku, aby wrócić. Aby zamknąć: prawy przycisk, Zakończ.")
	}
}

func isForeground() bool {
	fg, _, _ := pGetForegroundWindow.Call()
	vis, _, _ := pIsWindowVisible.Call(hMain)
	return fg == hMain && vis != 0
}

func showMain() {
	if compact.on { // back from the panel of the overlay to the window as it was
		compact.on = false
		setOverlay(false)
		r := compact.restore
		pSetWindowPos.Call(hMain, 0, uintptr(r.left), uintptr(r.top), uintptr(r.right-r.left), uintptr(r.bottom-r.top), 0x0004)
	}
	if r, _, _ := pIsIconic.Call(hMain); r != 0 {
		pShowWindow.Call(hMain, swRestore)
	} else {
		pShowWindow.Call(hMain, swShow)
	}
	pSetForegroundWindow.Call(hMain)
	refresh()
}

func trayMenu() {
	menu, _, _ := pCreatePopupMenu.Call()
	pAppendMenuW.Call(menu, mfString, idTrayShow, uintptr(unsafe.Pointer(u16("Pokaż "+appName))))
	dnd := uintptr(mfString)
	if cfg.DND {
		dnd |= mfChecked
	}
	pAppendMenuW.Call(menu, dnd, idTrayDND, uintptr(unsafe.Pointer(u16("Nie przeszkadzać"))))
	pAppendMenuW.Call(menu, mfSeparator, 0, 0)
	pAppendMenuW.Call(menu, mfString, idTrayQuit, uintptr(unsafe.Pointer(u16("Zakończ"))))
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(hMain)
	cmd, _, _ := pTrackPopupMenu.Call(menu, tpmRightBtn|tpmReturnCmd|tpmNoNotify, uintptr(pt.x), uintptr(pt.y), 0, hMain, 0)
	pDestroyMenu.Call(menu)
	switch cmd {
	case idTrayShow:
		showMain()
	case idTrayDND:
		cfg.DND = !cfg.DND
		saveConfig()
		trayRefresh()
		invalidate(hSet)
	case idTrayQuit:
		quitting = true
		pDestroyWindow.Call(hMain)
	}
}

func balloon(title, text string) {
	n := tray
	n.uFlags = nifInfo
	n.dwInfoFlags = niifInfo
	copyU16(n.szInfoTitle[:], title)
	copyU16(n.szInfo[:], text)
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&n)))
}

func threadByID(id int64) (store.Thread, bool) {
	if id == 0 {
		return store.Thread{}, false
	}
	for _, t := range threads {
		if t.ID == id {
			return t, true
		}
	}
	return store.Thread{}, false
}

// threadTitle is the name shown for a conversation: the nickname given in Messenger when
// there is one, the real name otherwise.
func threadTitle(t store.Thread) string {
	if n := titleNick[t.ID]; n != "" && !isGroup(t) {
		return n
	}
	return realTitle(t)
}

func realTitle(t store.Thread) string {
	if t.Title != "" {
		return t.Title
	}
	return fmt.Sprintf("Rozmowa %d", t.ID)
}

var weekdays = [7]string{"niedz.", "pon.", "wt.", "śr.", "czw.", "pt.", "sob."}

func whenLabel(t time.Time) string {
	if t.IsZero() || t.Unix() <= 0 {
		return ""
	}
	now := time.Now()
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format("15:04")
	case now.Sub(t) < 6*24*time.Hour:
		return weekdays[t.Weekday()]
	}
	return t.Format("2.01")
}

// contextLabel turns the search context from the core into text for the row.
func contextLabel(s string) string {
	switch s {
	case "person":
		return ""
	case "group":
		return "Grupa"
	}
	return oneLine(s, 80)
}

// commitRow opens the row: a conversation directly, a search result through Messenger.
func commitRow(idx int, focusInput bool) {
	if idx < 0 || idx >= len(rows) {
		return
	}
	r := rows[idx]
	if r.person != nil {
		be.openPerson(*r.person)
		return
	}
	if r.t.ID == selectedID && nav == navChats {
		if lay.narrow && !lay.chat {
			inChat = true
			layout()
		}
		if focusInput {
			pSetFocus.Call(hInput)
		}
		return
	}
	selectThread(r.t, focusInput)
}

func selectThread(t store.Thread, focusInput bool) {
	toastGone(t.ID)
	closeViewer()
	selClear()
	first := selectedID == 0
	if t.ID != selectedID && compose.mode != cmNone {
		if compose.mode == cmEdit {
			setText(hInput, "")
		}
		compose.mode = cmNone
		inputChanged()
		first = true // forces a new layout below
	}
	if t.ID != selectedID {
		mediaDropThumbs()
	}
	selectedID = t.ID
	inChat = true
	mv.thread, mv.has, mv.key = t, true, 0
	mv.hasNew = t.Unread()
	mv.newAfter = t.LastRead
	switch {
	case nav != navChats:
		setNav(navChats)
	case first || lay.narrow:
		layout()
	}
	for i, r := range rows { // bring the chosen conversation into view in the list
		if r.person == nil && r.t.ID == t.ID {
			sendMsg(hList, lbSetCurSel, uintptr(i), 0)
			break
		}
	}
	loadView(true)
	invalidate(hList)
	invalidate(hMain)
	if focusInput {
		pSetFocus.Call(hInput)
	}
	pSetTimer.Call(hMain, timerOpen, 500, 0)
}

func doSend() {
	text := strings.TrimSpace(strings.ReplaceAll(getText(hInput), "\r\n", "\n"))
	t, ok := threadByID(selectedID)
	if !ok || (text == "" && compose.mode != cmFile) {
		return
	}
	switch compose.mode {
	case cmFile:
		if !be.sendFile(t, compose.file) {
			return
		}
		if text != "" && !be.send(t, text, store.Message{}) {
			return
		}
	case cmEdit:
		if text != strings.TrimSpace(compose.msg.Text) && !be.edit(t, compose.msg, text) {
			return
		}
	case cmReply:
		if !be.send(t, text, compose.msg) {
			return
		}
	default:
		if !be.send(t, text, store.Message{}) {
			return
		}
	}
	wasComposing := compose.mode != cmNone
	compose.mode = cmNone
	setText(hInput, "")
	inputChanged()
	if wasComposing {
		layout()
	}
	mv.hasNew = false
}

func markSelectedRead() {
	t, ok := threadByID(selectedID)
	if !ok || !t.Unread() || !isForeground() || nav == navSettings || (lay.narrow && !lay.chat) {
		return
	}
	stamp := t.UpdatedAt.UnixMilli()
	if lastMarked[t.ID] == stamp {
		return
	}
	lastMarked[t.ID] = stamp
	// Seen here counts at once, whether or not Messenger can be told right now: the counter
	// must not grow for messages that are on the screen.
	seenLocal[t.ID] = stamp
	be.openThread(t, true)
}

// seenLocal holds, per conversation, the time of the newest message the user has had on
// screen in this run; unread counts never include messages up to it.
var seenLocal = map[int64]int64{}

// unreadNow reports whether a conversation has messages the user has not seen.
func unreadNow(t *store.Thread) bool {
	return t.Unread() && t.UpdatedAt.UnixMilli() > seenLocal[t.ID]
}

func refresh() {
	retryRead := false
	if be.mediaRetry.CompareAndSwap(true, false) { // the connection came back: failed pictures get another try
		for id := range md.failed {
			delete(md.failed, id)
		}
		defer relayoutView()
		retryRead = true
	}
	ts, err := be.db.ListThreads(be.ctx)
	if err != nil {
		return
	}
	threads = ts
	if nicks, err := be.db.TitleNicknames(be.ctx); err == nil {
		titleNick = nicks
	}
	if urls, err := be.db.Pictures(be.ctx); err == nil {
		avatarsSync(urls)
	}
	if all, err := be.db.AllReceipts(be.ctx); err == nil {
		receiptsBy = all
	}
	if last, err := be.db.LastSenders(be.ctx); err == nil {
		lastSender = last
	}
	lastMedia, _ := be.db.LastAttachments(be.ctx)
	now := time.Now()
	fg := isForeground()
	self := be.accountID()
	mode := nav
	if mode == navSettings {
		mode = navChats
	}

	var notify *store.Thread
	unreadBy = map[int64]int{}
	total := 0
	for i := range threads {
		t := &threads[i]
		stamp := t.UpdatedAt.UnixMilli()
		prev, seen := prevUpdated[t.ID]
		if retryRead && t.Unread() && seenLocal[t.ID] >= stamp {
			be.openThread(*t, true) // seen while Messenger could not be told: tell it now
		}
		if be.armed.Load() && (!seen || stamp > prev) && unreadNow(t) && !t.Muted(now) && !quietThread(t.ID) && !(fg && t.ID == selectedID) && notify == nil {
			notify = t
		}
		prevUpdated[t.ID] = stamp
		if unreadNow(t) {
			n := 0
			if since := seenLocal[t.ID]; since > t.LastRead.UnixMilli() { // seen here, not yet recorded as read
				n, _ = be.db.UnreadSince(be.ctx, t.ID, self, since)
			} else if ids, err := be.db.UnreadMessageIDs(be.ctx, t.ID, self); err == nil {
				for _, v := range ids {
					n += len(v)
				}
			}
			if n == 0 {
				n = 1
			}
			unreadBy[t.ID] = n
			if !t.Muted(now) {
				total += n
			}
		}
	}

	q := strings.ToLower(strings.TrimSpace(query))
	list := make([]rowData, 0, len(threads))
	known := map[int64]bool{}
	for _, t := range threads {
		title, real := threadTitle(t), realTitle(t)
		if q != "" && !strings.Contains(strings.ToLower(title), q) && !strings.Contains(strings.ToLower(real), q) {
			continue
		}
		sub := ""
		if real != title {
			sub = real
		}
		if mode == navPeople && isGroup(t) {
			continue
		}
		known[t.ID] = true
		row := rowData{t: t, title: title, sub: sub, preview: oneLine(emojify(previewText(t.LastPreview)), 90), when: whenLabel(t.UpdatedAt), unread: unreadBy[t.ID]}
		if icon, label := previewIcon(t.LastPreview, lastMedia[t.ID]); icon != "" {
			row.icon, row.preview = icon, label
		}
		if row.unread == 0 {
			row.seen = seenBy(t, self)
		}
		list = append(list, row)
	}
	searchBusy = false
	if mode == navPeople {
		sort.SliceStable(list, func(a, b int) bool { return strings.ToLower(list[a].title) < strings.ToLower(list[b].title) })
		if q != "" {
			found, busy := be.peopleResults(strings.TrimSpace(query))
			searchBusy = busy
			for i := range found {
				p := found[i]
				if known[p.ID] {
					continue
				}
				known[p.ID] = true
				list = append(list, rowData{person: &p, title: p.Name, sub: contextLabel(p.Context)})
			}
		}
	}
	hash := fnv.New64a()
	fmt.Fprintf(hash, "%d|%s|%v;", mode, q, searchBusy)
	for _, r := range list {
		fmt.Fprintf(hash, "%d|%v|%s|%s|%s|%s|%d|%v|%v;", r.t.ID, r.person != nil, r.title, r.preview, r.when, r.sub, r.unread, r.t.Muted(now), r.seen)
	}
	rows = list
	if key := hash.Sum64(); key != listKey {
		listKey = key
		keep := curSel()
		top := sendMsg(hList, lbGetTopIndex, 0, 0)
		sendMsg(hList, wmSetRedraw, 0, 0)
		sendMsg(hList, lbResetContent, 0, 0)
		sel := -1
		for i, r := range rows {
			label := r.title
			if r.unread > 0 && mode == navChats {
				label += fmt.Sprintf(", nieprzeczytane: %d", r.unread)
			}
			if r.preview != "" && mode == navChats {
				label += ", " + r.preview
			}
			if r.sub != "" {
				label += ", " + r.sub
			}
			sendMsg(hList, lbAddString, 0, uintptr(unsafe.Pointer(u16(sanitize(label)))))
			if r.person == nil && r.t.ID == selectedID && mode == navChats {
				sel = i
			}
		}
		if mode == navPeople && keep >= 0 && keep < len(rows) {
			sel = keep
		}
		if sel >= 0 {
			sendMsg(hList, lbSetCurSel, uintptr(sel), 0)
		}
		sendMsg(hList, lbSetTopIndex, top, 0)
		sendMsg(hList, wmSetRedraw, 1, 0)
		pInvalidateRect.Call(hList, 0, 1)
	}
	if total != unreadTotal {
		unreadTotal = total
		invalidate(hNav[0])
	}
	title := appName
	if unreadTotal > 0 {
		title = fmt.Sprintf("(%d) %s", unreadTotal, appName)
	}
	if getText(hMain) != title {
		setText(hMain, title)
	}
	trayRefresh()
	headsSync(be.armed.Load())

	if notify != nil && cfg.Toasts && !cfg.DND && !lastFromSelf(notify.ID, self) {
		notifyID = notify.ID
		body := "Nowa wiadomość"
		if cfg.Body {
			body = oneLine(previewText(notify.LastPreview), 200)
		}
		if !fg { // the window in front already shows the message: no sound over it
			playMessageSound()
		}
		notifyAbout(*notify, oneLine(threadTitle(*notify), 60), body)
		if !fg {
			pFlashWindow.Call(hMain, 1)
		}
	}
	if selectedID != 0 {
		if t, ok := threadByID(selectedID); ok {
			mv.thread = t
		}
		if vis, _, _ := pIsWindowVisible.Call(hMain); vis != 0 { // a hidden window lays nothing out
			loadView(false)
			markSelectedRead()
		}
	}
}

func lastFromSelf(threadID, self int64) bool {
	if self == 0 {
		return false
	}
	msgs, err := be.db.ListMessages(be.ctx, threadID)
	if err != nil || len(msgs) == 0 {
		return false
	}
	return msgs[len(msgs)-1].SenderID == self
}
