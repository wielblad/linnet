//go:build windows

package webview

import "unsafe"

// archDir is the runtime's folder name for this processor architecture.
const archDir = "x64"

// putBounds calls ICoreWebView2Controller::put_Bounds(RECT). The x64 calling convention
// passes a 16-byte struct "by value" as a pointer to a copy owned by the caller.
func putBounds(controller uintptr, r rect) int32 {
	return comCall(controller, controllerPutBounds, uintptr(unsafe.Pointer(&r)))
}
