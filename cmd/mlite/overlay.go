//go:build windows

package main

// The conversation opened from a head is not the ordinary program window: it is a panel of
// the overlay. It has no frame and no button on the taskbar, it stays above other windows,
// and it is dragged by its own header. Everything inside works as usual — reading,
// writing, pictures, the panel of the conversation.

import "unsafe"

const (
	gwlStyle    = ^uintptr(15) // GWL_STYLE (-16)
	gwlExStyle  = ^uintptr(19) // GWL_EXSTYLE (-20)
	wmNcHitTest = 0x0084
	htCaption   = 2
	hwndTopmost = ^uintptr(0) // HWND_TOPMOST (-1)
	hwndNoTop   = ^uintptr(1) // HWND_NOTOPMOST (-2)
	swpFrame    = 0x0020
	swpNoMove   = 0x0002
	swpNoSize   = 0x0001
	swpNoZOrder = 0x0004
)

var pGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")

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
		wantStyle = style&^uintptr(wsOverlappedWindow) | wsPopup
		wantEx = ex | wsExToolWindow
	} else {
		wantStyle = style&^uintptr(wsPopup) | wsOverlappedWindow
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
}

// overlayRound rounds the corners of the panel, and gives them back to the window when it
// is an ordinary window again.
func overlayRound() {
	if !compact.on {
		pSetWindowRgn.Call(hMain, 0, 1)
		return
	}
	var rc rect
	pGetClientRect.Call(hMain, uintptr(unsafe.Pointer(&rc)))
	if rc.right < 2 || rc.bottom < 2 {
		return
	}
	r := dp(12)
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(rc.right+1), uintptr(rc.bottom+1), uintptr(r), uintptr(r))
	if rgn != 0 {
		pSetWindowRgn.Call(hMain, rgn, 1)
	}
}

// overlayHit lets the panel be dragged by the empty part of its header, the way a title bar
// would, without swallowing the buttons drawn there.
func overlayHit(lp uintptr) (uintptr, bool) {
	if !compact.on {
		return 0, false
	}
	pt := point{int32(int16(lp)), int32(int16(lp >> 16))}
	pScreenToClient.Call(hMain, uintptr(unsafe.Pointer(&pt)))
	h := lay.header
	if pt.y < h.top || pt.y >= h.bottom || pt.x < h.left {
		return 0, false
	}
	for _, b := range []rect{lay.back, lay.hbSearch, lay.hbInfo} {
		if b.right > b.left && pt.x >= b.left && pt.x < b.right && pt.y >= b.top && pt.y < b.bottom {
			return 0, false
		}
	}
	return htCaption, true
}
