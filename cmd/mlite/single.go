//go:build windows

package main

// One running program per account data. Two of them on the same session throw each other
// off Messenger's servers; a second start therefore only brings the first one forward.

import (
	"fmt"
	"hash/fnv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	pCreateMutexW           = kernel32.NewProc("CreateMutexW")
	pRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")

	showMsg    uint32  // the message that asks the running program to show its window
	instanceMu uintptr // held for as long as the program runs
)

// onlyInstance reports whether this is the only program running on the data folder. When
// another one is, it is asked to show its window and false is returned.
func onlyInstance(dir string) bool {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(dir)))
	key := fmt.Sprintf("Linnet-%08x", h.Sum32())
	id, _, _ := pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(u16(key + "-show"))))
	showMsg = uint32(id)
	mu, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16("Local\\"+key))))
	if mu != 0 && err == syscall.Errno(183) { // ERROR_ALREADY_EXISTS
		if showMsg != 0 {
			pPostMessageW.Call(0xFFFF, uintptr(showMsg), 0, 0) // to every top-level window; only the other Linnet knows it
		}
		return false
	}
	instanceMu = mu
	return true
}
