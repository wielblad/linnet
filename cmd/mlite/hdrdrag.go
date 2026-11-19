//go:build windows

package main

// The header of a conversation beside a head is a handle of the head. The conversation
// belongs to its head and is moved by the head alone; that does not change. But the header
// looks like the title bar of a window, and people take hold of it. So a drag that begins
// on the empty part of the header moves the head, with its group and everything that
// stands beside them, exactly as dragging the head does. Nothing comes loose.
//
// A drag by the header does not tie heads together: that happens only when a head itself
// is dropped beside another.

import "unsafe"

const idcSizeAll = 32646 // the four-way arrow: this can be moved

var hdr struct {
	on, moved      bool
	hwnd           uintptr // the window whose header was taken hold of
	id             int64   // the head it belongs to
	sx, sy, ox, oy int32   // the pointer and the head when the button went down
}

func setCursor(id uintptr) {
	cur, _, _ := pLoadCursorW.Call(0, id)
	pSetCursor.Call(cur)
}

// hdrDragStart notes that the button went down on the header of a conversation.
func hdrDragStart(hwnd uintptr, id int64) bool {
	hc := headCfgOf(id)
	if hc == nil { // a conversation whose head was unpinned stays where it is
		return false
	}
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	hdr.on, hdr.moved, hdr.hwnd, hdr.id = true, false, hwnd, id
	hdr.sx, hdr.sy, hdr.ox, hdr.oy = pt.x, pt.y, hc.X, hc.Y
	pSetCapture.Call(hwnd)
	return true
}

// hdrDragMove follows the pointer; it reports whether a drag of this window is going on.
func hdrDragMove(hwnd uintptr) bool {
	if !hdr.on || hdr.hwnd != hwnd {
		return false
	}
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	dx, dy := pt.x-hdr.sx, pt.y-hdr.sy
	if !hdr.moved {
		if dx <= dp(4) && dx >= -dp(4) && dy <= dp(4) && dy >= -dp(4) {
			return true // not yet a drag: it may still be a click
		}
		hdr.moved = true
		for _, h := range heads {
			hidePeek(h)
		}
		setCursor(idcSizeAll)
	}
	entityMove(hdr.id, hdr.ox+dx, hdr.oy+dy)
	headsApply()
	return true
}

// hdrDragEnd is the button coming up. was says a press on the header ended here; moved,
// that it was a drag and not a click.
func hdrDragEnd(hwnd uintptr) (was, moved bool) {
	if !hdr.on || hdr.hwnd != hwnd {
		return false, false
	}
	id := hdr.id
	was, moved = true, hdr.moved
	hdr.on = false
	pReleaseCapture.Call()
	if moved {
		entitySettle(id) // on the screen, and to its edge when left close to one
		headsApply()
		saveConfig()
		headsSync(false)
		headsZ()
	}
	return was, moved
}

// hdrDragLost is told that the window lost the mouse some other way.
func hdrDragLost(hwnd uintptr) {
	if hdr.on && hdr.hwnd == hwnd {
		hdr.on = false
	}
}
