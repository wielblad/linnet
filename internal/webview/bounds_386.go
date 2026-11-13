//go:build windows

package webview

// archDir is the runtime's folder name for this processor architecture.
const archDir = "x86"

// putBounds calls ICoreWebView2Controller::put_Bounds(RECT). With stdcall on x86 the
// struct is copied onto the stack, which is the same as four 32-bit arguments.
// Not tested on real hardware.
func putBounds(controller uintptr, r rect) int32 {
	return comCall(controller, controllerPutBounds,
		uintptr(uint32(r.left)), uintptr(uint32(r.top)), uintptr(uint32(r.right)), uintptr(uint32(r.bottom)))
}
