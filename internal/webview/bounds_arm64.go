//go:build windows

package webview

// archDir is the runtime's folder name for this processor architecture.
const archDir = "arm64"

// putBounds calls ICoreWebView2Controller::put_Bounds(RECT). On ARM64 a 16-byte struct
// passed by value travels in two integer registers. Not tested on real hardware.
func putBounds(controller uintptr, r rect) int32 {
	return comCall(controller, controllerPutBounds,
		uintptr(uint32(r.left))|uintptr(uint32(r.top))<<32,
		uintptr(uint32(r.right))|uintptr(uint32(r.bottom))<<32)
}
