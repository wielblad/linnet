//go:build windows

package main

// Installing for the current user without any outside tool and without administrator
// rights: the program copies itself to the user's Programs folder, adds a Start Menu
// shortcut (which also gives its notifications their name and icon) and an entry in
// "Installed apps". Uninstalling takes all of it back and asks before touching the data.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	appVersion = "0.8.3"
	logName    = "linnet.log"
	wmAppQuit  = wmApp + 9
	uninstRoot = `Software\Microsoft\Windows\CurrentVersion\Uninstall\`
)

// appName is the name people see. A test install passes another one with -name.
var appName = "Linnet"

var (
	pSetAppID          = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
	pRegDeleteTreeW    = advapi32.NewProc("RegDeleteTreeW")
	pOpenProcess       = kernel32.NewProc("OpenProcess")
	pQueryImageNameW   = kernel32.NewProc("QueryFullProcessImageNameW")
	pCloseHandle       = kernel32.NewProc("CloseHandle")
	pFindWindowExW     = user32.NewProc("FindWindowExW")
	pGetWindowThreadID = user32.NewProc("GetWindowThreadProcessId")
)

func appSlug() string  { return strings.ToLower(strings.ReplaceAll(appName, " ", "-")) }
func appID() string    { return strings.ReplaceAll(appName, " ", "") }
func exeName() string  { return appSlug() + ".exe" }
func iconPath() string { return filepath.Join(installDir(), appSlug()+".ico") }

func installDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	return filepath.Join(base, "Programs", appName)
}

func shortcutPath() string {
	base, _ := os.UserConfigDir()
	return filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs", appName+".lnk")
}

func installedExe() string { return filepath.Join(installDir(), exeName()) }

// setAppID ties the running program to its Start Menu shortcut, so that Windows shows the
// shortcut's name and icon on notifications and groups the windows under it.
func setAppID() { pSetAppID.Call(uintptr(unsafe.Pointer(u16(appID())))) }

func ask(text string, flags uintptr) uintptr {
	r, _, _ := pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16(appName))), flags)
	return r
}

// runInstall is the whole of "setup": a question, the work, and an offer to start.
// launchAfter starts the installed program when the installation ends; an update uses it.
var launchAfter bool

func runInstall(quiet bool) {
	const yes = 6
	if !quiet && ask("Zainstalować "+appName+" dla bieżącego użytkownika?\n\nProgram zostanie skopiowany do:\n"+installDir()+
		"\n\ni pojawi się w menu Start. Uprawnienia administratora nie są potrzebne.", 0x24) != yes {
		return
	}
	if err := install(); err != nil {
		if !quiet {
			ask("Instalacja się nie udała.\n\n"+err.Error(), 0x10)
		}
		os.Exit(1)
	}
	if launchAfter || (!quiet && ask(appName+" jest zainstalowany.\n\nUruchomić teraz?", 0x24) == yes) {
		args := ""
		if appName != "Linnet" { // a test installation keeps its own name
			args = `-name "` + appName + `"`
		}
		shellOpen(installedExe(), args)
	}
}

func install() error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	dir, dst := installDir(), installedExe()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(src), filepath.Clean(dst)) {
		quitRunning(dst)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("nie można zapisać %s (jeśli %s działa, zamknij go z zasobnika i spróbuj ponownie): %w", dst, appName, err)
		}
	}
	os.Remove(iconPath()) // older versions kept a drawn icon beside the program; the file carries its own now
	if err := createShortcut(shortcutPath(), dst, dir, dst, appID(), "Klient wiadomości Messenger na Windows"); err != nil {
		return fmt.Errorf("skrót w menu Start: %w", err)
	}
	key := uninstRoot + appID()
	uninstall := fmt.Sprintf(`"%s" -uninstall`, dst)
	if appName != "Linnet" {
		uninstall += fmt.Sprintf(` -name "%s"`, appName)
	}
	regSetStr(key, "DisplayName", appName)
	regSetStr(key, "DisplayVersion", appVersion)
	regSetStr(key, "DisplayIcon", dst)
	regSetStr(key, "InstallLocation", dir)
	regSetStr(key, "UninstallString", uninstall)
	regSetStr(key, "QuietUninstallString", uninstall+" -quiet")
	regSetDword(key, "NoModify", 1)
	regSetDword(key, "NoRepair", 1)
	if info, err := os.Stat(dst); err == nil {
		regSetDword(key, "EstimatedSize", uint32(info.Size()/1024))
	}
	return nil
}

// runUninstall removes what install made. The messages and the session are the user's
// data: they stay unless the user says otherwise.
func runUninstall(quiet bool, dataDir string) {
	const yes = 6
	if !quiet && ask("Odinstalować "+appName+"?", 0x24) != yes {
		return
	}
	dropData := false
	if !quiet {
		if _, err := os.Stat(dataDir); err == nil {
			dropData = ask("Usunąć także dane programu: sesję logowania i zapisane wiadomości?\n\n"+dataDir+
				"\n\nTej operacji nie da się cofnąć. Wybierz Nie, aby je zostawić.", 0x124) == yes
		}
	}
	quitRunning(installedExe())
	os.Remove(shortcutPath())
	pRegDeleteKeyValueW.Call(hkcu, uintptr(unsafe.Pointer(u16(runKey))), uintptr(unsafe.Pointer(u16(appName))))
	pRegDeleteTreeW.Call(hkcu, uintptr(unsafe.Pointer(u16(uninstRoot+appID()))))
	os.Remove(iconPath())
	if dropData {
		os.RemoveAll(dataDir)
	}
	// A running program cannot delete its own file: a hidden command does it a moment later.
	dir := installDir()
	if self, err := os.Executable(); err == nil && strings.EqualFold(filepath.Dir(self), dir) {
		// The command line is given whole: the usual quoting of arguments is not what cmd reads.
		cmd := exec.Command("cmd.exe")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000,
			CmdLine: `cmd.exe /s /c "ping -n 3 127.0.0.1 >nul & rmdir /s /q "` + dir + `""`}
		cmd.Dir = os.TempDir()
		_ = cmd.Start()
	} else {
		os.RemoveAll(dir)
	}
	if !quiet {
		ask(appName+" został odinstalowany.", 0x40)
	}
}

// quitRunning asks a running copy started from the given file to end, and waits a moment.
func quitRunning(exe string) {
	found := false
	var w uintptr
	for i := 0; i < 200; i++ {
		w, _, _ = pFindWindowExW.Call(0, w, uintptr(unsafe.Pointer(u16("MliteMain"))), 0)
		if w == 0 {
			break
		}
		var pid uint32
		pGetWindowThreadID.Call(w, uintptr(unsafe.Pointer(&pid)))
		if pid == uint32(os.Getpid()) || !strings.EqualFold(processPath(pid), exe) {
			continue
		}
		pPostMessageW.Call(w, wmAppQuit, 0, 0)
		found = true
	}
	if found {
		for i := 0; i < 40; i++ { // up to four seconds for the file to be released
			f, err := os.OpenFile(exe, os.O_RDWR, 0)
			if err == nil {
				f.Close()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func processPath(pid uint32) string {
	h, _, _ := pOpenProcess.Call(0x1000, 0, uintptr(pid)) // query limited information
	if h == 0 {
		return ""
	}
	defer pCloseHandle.Call(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if r, _, _ := pQueryImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func regSetStr(key, name, value string) {
	v := syscall.StringToUTF16(value)
	pRegSetKeyValueW.Call(hkcu, uintptr(unsafe.Pointer(u16(key))), uintptr(unsafe.Pointer(u16(name))), regSz, uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2))
}

func regSetDword(key, name string, value uint32) {
	pRegSetKeyValueW.Call(hkcu, uintptr(unsafe.Pointer(u16(key))), uintptr(unsafe.Pointer(u16(name))), 4, uintptr(unsafe.Pointer(&value)), 4)
}

// writeIcon saves the program's icon as an .ico file with several sizes, for the shortcut
// and the list of installed apps.
func writeIcon(path string) error {
	png := syscall.GUID{Data1: 0x557CF406, Data2: 0x1A04, Data3: 0x11D3, Data4: [8]byte{0x9A, 0x73, 0x00, 0x00, 0xF8, 0x1E, 0xF3, 0x2E}}
	sizes := []int32{16, 24, 32, 48, 64, 256}
	var images [][]byte
	tmp := path + ".png"
	defer os.Remove(tmp)
	for _, s := range sizes {
		bmp := iconBitmap(s, 0, false)
		if bmp == 0 {
			return errors.New("nie można narysować ikony")
		}
		st, _, _ := pGdipSaveImageToFile.Call(bmp, uintptr(unsafe.Pointer(u16(tmp))), uintptr(unsafe.Pointer(&png)), 0)
		pGdipDisposeImage.Call(bmp)
		data, err := os.ReadFile(tmp)
		if st != 0 || err != nil {
			return errors.New("nie można zapisać ikony")
		}
		images = append(images, data)
	}
	out := make([]byte, 6+16*len(sizes))
	binary.LittleEndian.PutUint16(out[2:], 1)
	binary.LittleEndian.PutUint16(out[4:], uint16(len(sizes)))
	offset := len(out)
	for i, s := range sizes {
		e := out[6+16*i:]
		e[0], e[1] = byte(s), byte(s) // 256 is written as 0
		binary.LittleEndian.PutUint16(e[4:], 1)
		binary.LittleEndian.PutUint16(e[6:], 32)
		binary.LittleEndian.PutUint32(e[8:], uint32(len(images[i])))
		binary.LittleEndian.PutUint32(e[12:], uint32(offset))
		offset += len(images[i])
	}
	for _, img := range images {
		out = append(out, img...)
	}
	return os.WriteFile(path, out, 0o644)
}

// createShortcut writes a .lnk file carrying the application identity.
func createShortcut(link, target, workDir, icon, id, description string) error {
	clsid := syscall.GUID{Data1: 0x00021401, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidLink := syscall.GUID{Data1: 0x000214F9, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidFile := syscall.GUID{Data1: 0x0000010B, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidStore := syscall.GUID{Data1: 0x886D8EEB, Data2: 0x8CF2, Data3: 0x4446, Data4: [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99}}
	pCoInitializeEx.Call(0, 2)
	var sl, store, file uintptr
	if hr, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&iidLink)), uintptr(unsafe.Pointer(&sl))); hr != 0 || sl == 0 {
		return fmt.Errorf("krok 1 (%08X)", uint32(hr))
	}
	defer com(sl, 2)
	p := func(s string) uintptr { return uintptr(unsafe.Pointer(u16(s))) }
	com(sl, 20, p(target))     // SetPath
	com(sl, 9, p(workDir))     // SetWorkingDirectory
	com(sl, 7, p(description)) // SetDescription
	com(sl, 17, p(icon), 0)    // SetIconLocation
	if hr := com(sl, 0, uintptr(unsafe.Pointer(&iidStore)), uintptr(unsafe.Pointer(&store))); hr != 0 || store == 0 {
		return fmt.Errorf("krok 2 (%08X)", uint32(hr))
	}
	key := struct {
		fmtid syscall.GUID
		pid   uint32
	}{syscall.GUID{Data1: 0x9F4C2855, Data2: 0x9F79, Data3: 0x4B39, Data4: [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3}}, 5}
	idText := syscall.StringToUTF16(id)
	value := struct {
		vt         uint16
		r1, r2, r3 uint16
		p          unsafe.Pointer
		pad        uintptr
	}{vt: 31, p: unsafe.Pointer(&idText[0])}
	hr := com(store, 6, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&value))) // SetValue
	if hr == 0 {
		hr = com(store, 7) // Commit
	}
	com(store, 2)
	if hr != 0 {
		return fmt.Errorf("krok 3 (%08X)", uint32(hr))
	}
	if hr := com(sl, 0, uintptr(unsafe.Pointer(&iidFile)), uintptr(unsafe.Pointer(&file))); hr != 0 || file == 0 {
		return fmt.Errorf("krok 4 (%08X)", uint32(hr))
	}
	defer com(file, 2)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	if hr := com(file, 6, p(link), 1); hr != 0 { // Save
		return fmt.Errorf("krok 5 (%08X)", uint32(hr))
	}
	return nil
}
