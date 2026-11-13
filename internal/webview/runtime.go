//go:build windows

package webview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

// The Evergreen WebView2 runtime registers itself with Edge Update under this client id.
// The machine-wide installer writes to the 32-bit view of HKLM, the per-user one to HKCU.
const (
	runtimeClientKey = `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	runtimeStateKey  = `SOFTWARE\Microsoft\EdgeUpdate\ClientState\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	runtimeDLL       = "EmbeddedBrowserWebView.dll"
	runtimeEntry     = "CreateWebViewEnvironmentWithOptionsInternal"

	regSZ       = 1
	regExpandSZ = 2
)

var errRuntimeNotFound = errors.New("webview: the Microsoft Edge WebView2 runtime was not found on this computer")

var (
	loaderMu    sync.Mutex
	loaderEntry *syscall.Proc // stays loaded for the life of the process
)

// Available reports whether the WebView2 runtime can be found on this computer.
func Available() bool {
	_, err := findRuntimeDLL()
	return err == nil
}

// findRuntimeDLL returns the full path of the runtime's embedding DLL for this
// architecture: machine-wide installation first, then the per-user one.
func findRuntimeDLL() (string, error) {
	for _, root := range []syscall.Handle{syscall.HKEY_LOCAL_MACHINE, syscall.HKEY_CURRENT_USER} {
		for _, dir := range runtimeVersionDirs(root) {
			dll := filepath.Join(dir, "EBWebView", archDir, runtimeDLL)
			if st, err := os.Stat(dll); err == nil && !st.IsDir() {
				return dll, nil
			}
		}
	}
	return "", errRuntimeNotFound
}

// runtimeVersionDirs lists the folders that may hold the installed version under root:
// "location\pv" from the Clients key, then the folder recorded in ClientState.
func runtimeVersionDirs(root syscall.Handle) []string {
	var dirs []string
	pv := regString(root, runtimeClientKey, "pv")
	if pv != "" && pv != "0.0.0.0" {
		if loc := regString(root, runtimeClientKey, "location"); loc != "" {
			dirs = append(dirs, filepath.Join(loc, pv))
		}
	}
	if dir := regString(root, runtimeStateKey, "EBWebView"); dir != "" {
		dirs = append(dirs, dir)
	}
	return dirs
}

// regString reads a string value from the 32-bit registry view; "" when it is missing.
func regString(root syscall.Handle, path, name string) string {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return ""
	}
	var key syscall.Handle
	if syscall.RegOpenKeyEx(root, p, 0, syscall.KEY_QUERY_VALUE|syscall.KEY_WOW64_32KEY, &key) != nil {
		return ""
	}
	defer syscall.RegCloseKey(key)
	var typ, size uint32
	if syscall.RegQueryValueEx(key, n, nil, &typ, nil, &size) != nil || size < 2 {
		return ""
	}
	if typ != regSZ && typ != regExpandSZ {
		return ""
	}
	buf := make([]uint16, size/2+1)
	size = uint32(len(buf)-1) * 2
	if syscall.RegQueryValueEx(key, n, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// loadRuntime loads the runtime's DLL once and returns its environment-creating entry
// point, the undocumented function that Microsoft's own WebView2Loader.dll calls.
func loadRuntime() (*syscall.Proc, error) {
	loaderMu.Lock()
	defer loaderMu.Unlock()
	if loaderEntry != nil {
		return loaderEntry, nil
	}
	path, err := findRuntimeDLL()
	if err != nil {
		return nil, err
	}
	dll, err := syscall.LoadDLL(path)
	if err != nil {
		return nil, fmt.Errorf("webview: loading the WebView2 runtime %s failed: %w", path, err)
	}
	proc, err := dll.FindProc(runtimeEntry)
	if err != nil {
		dll.Release()
		return nil, fmt.Errorf("webview: the WebView2 runtime %s has no %s: %w", path, runtimeEntry, err)
	}
	loaderEntry = proc
	return proc, nil
}
