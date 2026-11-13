//go:build windows

package main

// The sound of a new message. The program plays it itself rather than leaving it to the
// notification, so that one switch in the settings rules it whichever kind of
// notification is shown.
//
// Which sound it is can be chosen. The choices are the sounds Windows itself keeps in
// C:\Windows\Media: nothing is downloaded, nothing travels with the program, and they are
// already on every Windows computer. A file of your own may be named instead.

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	winmm       = syscall.NewLazyDLL("winmm.dll")
	pPlaySoundW = winmm.NewProc("PlaySoundW")
	soundPlayed string // the Windows sound that played last, for checks
)

const (
	idSoundFirst  = 700 // plus the index into messageSounds
	idSoundCustom = 799

	soundRowTitle = "Dźwięk wiadomości"
)

// messageSounds are the sounds offered for a new message, named the way they sound rather
// than the way the file is called.
var messageSounds = []struct{ name, file string }{
	{"Systemowy dźwięk wiadomości", ""},
	{"Wiadomość", "Windows Notify Messaging.wav"},
	{"Powiadomienie", "Windows Notify.wav"},
	{"Poczta", "Windows Notify Email.wav"},
	{"Kalendarz", "Windows Notify Calendar.wav"},
	{"Ogólny sygnał", "Windows Notify System Generic.wav"},
	{"Szturchnięcie", "Windows Message Nudge.wav"},
	{"Dzwonek Windows", "Windows Ding.wav"},
	{"Kuranty", "chimes.wav"},
	{"Krótkie ding", "ding.wav"},
	{"Stare „notify”", "notify.wav"},
	{"Dymek", "Windows Balloon.wav"},
	{"Cichy sygnał", "Windows Information Bar.wav"},
	{"Akord", "chord.wav"},
	{"Zbliżenie", "Windows Proximity Notification.wav"},
	{"Fanfara", "tada.wav"},
}

// soundPath is the file to play, or "" when the system sound for a message is to be used.
func soundPath(choice string) string {
	switch {
	case choice == "":
		return ""
	case filepath.IsAbs(choice):
		return choice
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "Media", choice)
}

// soundName is what the settings show as the chosen sound.
func soundName() string {
	for _, s := range messageSounds {
		if s.file == cfg.SoundFile {
			return s.name
		}
	}
	if cfg.SoundFile != "" {
		return filepath.Base(cfg.SoundFile)
	}
	return "Systemowy dźwięk wiadomości"
}

// playMessageSound plays the chosen sound, or the Windows sound for an incoming message,
// falling back to the general notification sound of the user's sound scheme.
func playMessageSound() bool {
	if !cfg.Sound {
		return false
	}
	if cfg.Tone != "" {
		return playTone(cfg.Tone)
	}
	const async, noDefault, alias, filename = 0x0001, 0x0002, 0x00010000, 0x00020000
	if path := soundPath(cfg.SoundFile); path != "" {
		if r, _, _ := pPlaySoundW.Call(uintptr(unsafe.Pointer(u16(path))), 0, filename|async|noDefault); r != 0 {
			soundPlayed = path
			return true
		}
	}
	for _, name := range []string{"Notification.IM", "Notification.Default", "SystemNotification"} {
		if r, _, _ := pPlaySoundW.Call(uintptr(unsafe.Pointer(u16(name))), 0, alias|async|noDefault); r != 0 {
			soundPlayed = name
			return true
		}
	}
	return false
}

// playTone plays one of the program's own sounds, made as it is first needed and kept in
// memory afterwards, because Windows plays it from there after the call has returned.
func playTone(id string) bool {
	const async, noDefault, memory = 0x0001, 0x0002, 0x0004
	wav := toneWAV(id)
	if wav == nil {
		return false
	}
	r, _, _ := pPlaySoundW.Call(uintptr(unsafe.Pointer(&wav[0])), 0, memory|async|noDefault)
	soundPlayed = id
	return r != 0
}

// soundLine tells which sound is chosen, and says when its file is not on this computer.
func soundLine() string {
	if cfg.Tone == "" {
		if path := soundPath(cfg.SoundFile); path != "" {
			if _, err := os.Stat(path); err != nil {
				return soundName() + " — nie ma takiego pliku, zagra dźwięk systemowy."
			}
		}
	}
	return "Wybrany dźwięk od razu zagra, żeby go posłuchać."
}

// soundLabel names the chosen sound on the button of the row.
func soundLabel() string {
	if cfg.Tone != "" {
		for _, t := range tones {
			if t.id == cfg.Tone {
				return t.label + " ▾"
			}
		}
	}
	if cfg.SoundFile == "" {
		return "Systemowy ▾"
	}
	return soundName() + " ▾"
}

// pickSound offers everything the message sound can be, in three groups: the system sound
// and the program's own short ones, the sounds Windows keeps in its Media folder, and a
// file of one's own. The chosen one plays at once.
func pickSound() {
	type choice struct {
		tone, file string
		own        bool // the entry that asks for a file of one's own
	}
	labels, choices, breaks := []string{}, []choice{}, []int{}
	for _, t := range tones {
		labels, choices = append(labels, t.label), append(choices, choice{tone: t.id})
	}
	breaks = append(breaks, len(labels))
	for _, w := range messageSounds[1:] {
		labels, choices = append(labels, w.name), append(choices, choice{file: w.file})
	}
	breaks = append(breaks, len(labels))
	labels, choices = append(labels, "Własny plik .wav…"), append(choices, choice{own: true})
	current := 0
	for i, c := range choices {
		if !c.own && c.tone == cfg.Tone && c.file == cfg.SoundFile {
			current = i
			break
		}
	}
	i := settingsPick(labels, current, breaks...)
	if i < 0 {
		return
	}
	c := choices[i]
	if c.own {
		path := pickSoundFile()
		if path == "" {
			return
		}
		c = choice{file: path}
	}
	cfg.Tone, cfg.SoundFile = c.tone, c.file
	saveConfig()
	layoutSettings()
	invalidate(sv.hwnd)
	was := cfg.Sound
	cfg.Sound = true // the choice is heard even when the sound is switched off
	playMessageSound()
	cfg.Sound = was
}

// pickSoundFile asks for a sound of one's own.
func pickSoundFile() string {
	buf := make([]uint16, 1024)
	filter := utf16.Encode([]rune("Dźwięki\x00*.wav\x00Wszystkie pliki\x00*.*\x00\x00"))
	ofn := openFileName{owner: hMain, filter: &filter[0], file: &buf[0], maxFile: uint32(len(buf)),
		title: u16("Wybierz dźwięk wiadomości"), flags: 0x00080000 | 0x00001000 | 0x00000800 | 0x00000008}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))
	if r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return ""
	}
	path := syscall.UTF16ToString(buf)
	if !strings.EqualFold(filepath.Ext(path), ".wav") {
		be.notice("Program gra tylko pliki .wav.")
		return ""
	}
	return path
}
