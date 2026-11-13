//go:build windows

package main

// The conversation opened from a head is not the ordinary program window: it is a panel of
// the overlay. It has no frame and no button on the taskbar, it stays above other windows,
// and it is dragged by its own header. Everything inside works as usual — reading,
// writing, pictures, the panel of the conversation.

import (
	"fmt"
	"runtime/debug"
	"unsafe"

	"github.com/rs/zerolog/log"
)

const (
	gwlStyle      = ^uintptr(15) // GWL_STYLE (-16)
	gwlExStyle    = ^uintptr(19) // GWL_EXSTYLE (-20)
	wmNcHitTest   = 0x0084
	htClient      = 1
	hwndTopmost   = ^uintptr(0) // HWND_TOPMOST (-1)
	hwndNoTop     = ^uintptr(1) // HWND_NOTOPMOST (-2)
	swpFrame      = 0x0020
	swpNoMove     = 0x0002
	swpNoSize     = 0x0001
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
)

var (
	pGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	pRedrawWindow        = user32.NewProc("RedrawWindow")
	pBeginDeferWindowPos = user32.NewProc("BeginDeferWindowPos")
	pDeferWindowPos      = user32.NewProc("DeferWindowPos")
	pEndDeferWindowPos   = user32.NewProc("EndDeferWindowPos")
)

const (
	dwmaCornerPreference = 33
	dwmaBorderColour     = 34
	dwmRoundSmall        = 3          // DWMWCP_ROUNDSMALL
	dwmRoundNone         = 1          // DWMWCP_DONOTROUND
	dwmDefaultColour     = 0xFFFFFFFF // DWMWA_COLOR_DEFAULT
	wsThickFrame         = 0x00040000
	wmNcCalcSize         = 0x0083
	wmNcPaint            = 0x0085
	wmNcActivate         = 0x0086
)

func windowLong(h, which uintptr) uintptr {
	v, _, _ := pGetWindowLongPtrW.Call(h, which)
	return v
}

// setOverlay turns the window into a panel of the overlay, or back into the ordinary
// program window. The window is hidden for the change, because Windows looks at the styles
// only then when deciding about the taskbar.
func setOverlay(on bool) {
	style, ex := windowLong(hMain, gwlStyle), windowLong(hMain, gwlExStyle)
	wantStyle, wantEx := style, ex
	if on {
		wantStyle = style&^uintptr(wsOverlappedWindow) | wsPopup | wsThickFrame
		wantEx = ex | wsExToolWindow
	} else {
		wantStyle = style&^uintptr(wsPopup|wsThickFrame) | wsOverlappedWindow
		wantEx = ex &^ uintptr(wsExToolWindow)
	}
	if wantStyle == style && wantEx == ex {
		return
	}
	visible, _, _ := pIsWindowVisible.Call(hMain)
	if visible != 0 {
		pShowWindow.Call(hMain, swHide)
	}
	pSetWindowLongPtrW.Call(hMain, gwlStyle, wantStyle)
	pSetWindowLongPtrW.Call(hMain, gwlExStyle, wantEx)
	after := hwndNoTop
	if on {
		after = hwndTopmost
	}
	pSetWindowPos.Call(hMain, after, 0, 0, 0, 0, swpNoMove|swpNoSize|swpFrame)
	if visible != 0 {
		pShowWindow.Call(hMain, swShow)
	}
	overlayRound()
	// Rare and worth a line: a frame that came back when it should not have is hard to
	// catch afterwards, and this says what the window really was at the time.
	var r rect
	pGetWindowRect.Call(hMain, uintptr(unsafe.Pointer(&r)))
	log.Warn().Bool("panel", on).Str("style", fmt.Sprintf("0x%08X", windowLong(hMain, gwlStyle))).
		Str("exstyle", fmt.Sprintf("0x%08X", windowLong(hMain, gwlExStyle))).
		Int("w", int(r.right-r.left)).Int("h", int(r.bottom-r.top)).Msg("overlay styles")
}

// overlayRound asks Windows for the rounded corners, the shadow and the thin edge of a
// panel, so that it looks the way the ordinary window does. The frame itself is taken away
// in wndProc: the window keeps it only for what the system draws around it.
func overlayRound() {
	round, border := uintptr(dwmRoundSmall), uintptr(cref(pal.line))
	if !compact.on {
		round, border = dwmRoundNone, dwmDefaultColour
	} else if cfg.HeadMode == headModeCols { // columns stand shoulder to shoulder
		round = dwmRoundNone
	}
	pDwmSetWindowAttribute.Call(hMain, dwmaCornerPreference, uintptr(unsafe.Pointer(&round)), 4)
	pDwmSetWindowAttribute.Call(hMain, dwmaBorderColour, uintptr(unsafe.Pointer(&border)), 4)
}

// hideOverlay puts the panel away and leaves the head on the desktop; the program itself
// keeps running whatever the setting about closing the window says.
func hideOverlay() {
	if !compact.on {
		closeWindow()
		return
	}
	closeViewer()
	closePanel()
	pShowWindow.Call(hMain, swHide)
	panesPlace() // the panes close the gap the panel left
	headsSync(false)
	dwRelease() // nothing is drawn while hidden; give the text engine's memory back
	debug.FreeOSMemory()
	mediaDropThumbs()
}
