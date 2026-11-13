//go:build windows

package main

// The Linnet mark comes from the icons linked into the program (tools/mkres.py), never
// drawn by hand: the application icon, and the two one-colour marks for the notification
// area.

import "unsafe"

const (
	resApp       = 1 // blue disc with the white letter
	resTrayLight = 2 // light mark, for a dark taskbar
	resTrayDark  = 3 // dark mark, for a light taskbar
)

var (
	pLoadImageW = user32.NewProc("LoadImageW")
	pDrawIconEx = user32.NewProc("DrawIconEx")
)

// brandIcon loads one of the linked icons at the given size; the caller destroys it.
func brandIcon(id int, size int32) uintptr {
	hinst, _, _ := pGetModuleHandleW.Call(0)
	icon, _, _ := pLoadImageW.Call(hinst, uintptr(id), 1, uintptr(size), uintptr(size), 0)
	if icon == 0 && id == resApp { // a build without resources still gets an icon
		icon = makeIcon(size, 0, false)
	}
	return icon
}

// shellLight reports whether the taskbar is light, which Windows sets apart from the
// light or dark mode of applications.
func shellLight() bool {
	return regDword(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "SystemUsesLightTheme", 0) != 0
}

// drawBrand draws the application mark with its top left corner at x, y.
func drawBrand(hdc uintptr, x, y, size int32) {
	if icon := brandIcon(resApp, size); icon != 0 {
		pDrawIconEx.Call(hdc, uintptr(x), uintptr(y), icon, uintptr(size), uintptr(size), 0, 0, 3)
		pDestroyIcon.Call(icon)
	}
}

var _ = unsafe.Pointer(nil)
