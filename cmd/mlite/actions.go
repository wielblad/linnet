//go:build windows

package main

// What can be done with a single message: reply, react, copy, edit, remove for everyone,
// and for a message that failed to send: try again or discard.

import (
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/rs/zerolog/log"

	"lazyfmess/internal/store"
)

const (
	cmNone = iota
	cmReply
	cmEdit
	cmFile

	idMsgReply   = 400
	idMsgCopy    = 401
	idMsgEdit    = 402
	idMsgUnsend  = 403
	idMsgRetry   = 404
	idMsgDrop    = 405
	idMsgReact   = 410 // plus the index into reactions
	idMsgUnreact = 419

	timerNote     = 4
	mfChecked     = 0x0008
	mfPopup       = 0x0010
	wmContextMenu = 0x007B
	emSetSel      = 0x00B1
	gClose        = ""
	gBack         = ""
)

// The same quick reactions the core offers.
var reactions = []struct{ emoji, name string }{
	{"👍", "Lubię to"}, {"❤", "Super"}, {"😆", "Ha ha"}, {"😮", "Wow"}, {"😢", "Przykro mi"}, {"😠", "Wrr"},
}

var (
	pClientToScreen   = user32.NewProc("ClientToScreen")
	pOpenClipboard    = user32.NewProc("OpenClipboard")
	pEmptyClipboard   = user32.NewProc("EmptyClipboard")
	pSetClipboardData = user32.NewProc("SetClipboardData")
	pCloseClipboard   = user32.NewProc("CloseClipboard")
	pGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	pGlobalLock       = kernel32.NewProc("GlobalLock")
	pGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	pGetProcAddress   = kernel32.NewProc("GetProcAddress")

	// compose is what the text field is currently for.
	compose struct {
		mode     int
		msg      store.Message
		file     string // the file waiting to be sent
		closeHit rect
	}
	menuMsg store.Message // the message the last menu was opened for
	menuHas bool
)

// allowDarkMenus lets pop-up menus follow the dark app mode. Windows offers this only through
// unnamed entries of uxtheme.dll; when they are missing the menus simply stay light.
func allowDarkMenus() {
	h, _, _ := pLoadLibraryW.Call(uintptr(unsafe.Pointer(u16("uxtheme.dll"))))
	if h == 0 {
		return
	}
	if set, _, _ := pGetProcAddress.Call(h, 135); set != 0 { // SetPreferredAppMode(AllowDark)
		syscall.SyscallN(set, 1)
	}
	if flush, _, _ := pGetProcAddress.Call(h, 136); flush != 0 { // FlushMenuThemes
		syscall.SyscallN(flush)
	}
}

func ownReaction(m store.Message) string {
	for _, r := range m.Reactions {
		if r.ActorID == mv.self && mv.self != 0 {
			return r.Emoji
		}
	}
	return ""
}

// messageAt returns the message whose bubble is under the point of the conversation view.
func messageAt(x, y int32) (store.Message, bool) {
	y += mv.scroll
	for i := range mv.items {
		it := &mv.items[i]
		if it.kind == itMsg && y >= it.y+it.top && y < it.y+it.top+it.bh && x >= it.bx && x < it.bx+it.bw {
			return it.m, true
		}
	}
	return store.Message{}, false
}

// viewMenu opens the menu of a message: under the pointer, or for the newest visible message
// when it was asked for from the keyboard.
func viewMenu(x, y int32, keyboard bool) {
	var m store.Message
	ok := false
	if keyboard {
		for i := len(mv.items) - 1; i >= 0 && !ok; i-- {
			it := &mv.items[i]
			if it.kind == itMsg && it.y+it.top-mv.scroll < mv.h && it.y+it.top+it.bh-mv.scroll > 0 {
				m, ok = it.m, true
				x, y = it.bx+dp(12), it.y+it.top+it.bh-mv.scroll
			}
		}
	} else {
		m, ok = messageAt(x, y)
	}
	if !ok {
		return
	}
	messageMenu(m, x, y)
}

// messageMenu opens the menu of one message at a point of the conversation view.
func messageMenu(m store.Message, x, y int32) {
	if m.Unsent {
		return
	}
	menuMsg, menuHas = m, true
	me := m.SenderID == mv.self && mv.self != 0
	menu, _, _ := pCreatePopupMenu.Call()
	add := func(to uintptr, flags, id uintptr, label string) {
		pAppendMenuW.Call(to, flags, id, uintptr(unsafe.Pointer(u16(label))))
	}
	switch {
	case m.Error != "":
		add(menu, mfString, idMsgRetry, "Ponów wysyłanie")
		add(menu, mfString, idMsgCopy, "Kopiuj tekst")
		add(menu, mfSeparator, 0, "")
		add(menu, mfString, idMsgDrop, "Usuń")
	case m.Pending:
		add(menu, mfString, idMsgCopy, "Kopiuj tekst")
	default:
		if selIn(m.ID) {
			add(menu, mfString, idMsgCopySel, "Kopiuj zaznaczenie")
			add(menu, mfSeparator, 0, "")
		}
		add(menu, mfString, idMsgReply, "Odpowiedz")
		sub, _, _ := pCreatePopupMenu.Call()
		own := ownReaction(m)
		for i, r := range reactions {
			flags := uintptr(mfString)
			if r.emoji == own {
				flags |= mfChecked
			}
			add(sub, flags, uintptr(idMsgReact+i), r.emoji+"   "+r.name)
		}
		if own != "" {
			add(sub, mfSeparator, 0, "")
			add(sub, mfString, idMsgUnreact, "Usuń moją reakcję")
		}
		add(menu, mfPopup, sub, "Reakcja")
		if m.Text != "" {
			add(menu, mfString, idMsgCopy, "Kopiuj tekst")
		}
		if me {
			add(menu, mfSeparator, 0, "")
			if m.Text != "" {
				add(menu, mfString, idMsgEdit, "Edytuj")
			}
			add(menu, mfString, idMsgUnsend, "Usuń u wszystkich")
		}
	}
	pt := point{x, y}
	pClientToScreen.Call(mv.hwnd, uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(hMain)
	cmd, _, _ := pTrackPopupMenu.Call(menu, tpmRightBtn|tpmReturnCmd|tpmNoNotify, uintptr(pt.x), uintptr(pt.y), 0, hMain, 0)
	pDestroyMenu.Call(menu)
	if cmd != 0 {
		messageAction(int(cmd))
	}
}

// messageAction carries out a menu choice for the message the menu was opened for.
func messageAction(id int) {
	t, ok := threadByID(selectedID)
	if !menuHas || !ok || menuMsg.ThreadID != t.ID {
		return
	}
	m := menuMsg
	switch {
	case id == idMsgReply:
		setCompose(cmReply, m)
	case id == idMsgEdit:
		setCompose(cmEdit, m)
	case id == idMsgCopy:
		copyText(m.Text)
		viewToast("Skopiowano wiadomość")
	case id == idMsgCopySel:
		copyText(selText())
		viewToast("Skopiowano zaznaczenie")
	case id == idMsgUnsend:
		confirmBox("Usunąć tę wiadomość u wszystkich?", "Zniknie także u rozmówców. Tego nie da się cofnąć.", "Usuń u wszystkich", true, func() {
			be.unsend(t, m)
		})
	case id == idMsgRetry:
		be.retry(t, m)
	case id == idMsgDrop:
		be.dropLocal(m)
	case id == idMsgUnreact:
		be.react(t, m, "")
	case id >= idMsgReact && id < idMsgReact+len(reactions):
		emoji := reactions[id-idMsgReact].emoji
		if emoji == ownReaction(m) { // choosing the same reaction again takes it back
			emoji = ""
		}
		be.react(t, m, emoji)
	}
}

func setCompose(mode int, m store.Message) {
	if compose.mode == cmEdit && mode != cmEdit {
		setText(hInput, "")
	}
	if compose.mode == cmFile {
		dropPasted(compose.file)
	}
	compose.mode, compose.msg = mode, m
	if mode == cmEdit {
		text := strings.ReplaceAll(strings.ReplaceAll(m.Text, "\r\n", "\n"), "\n", "\r\n")
		setText(hInput, text)
		n := uintptr(len(syscall.StringToUTF16(text)) - 1)
		sendMsg(hInput, emSetSel, n, n)
	}
	inputChanged()
	layout()
	pSetFocus.Call(hInput)
}

// cancelCompose returns the text field to writing a new message.
func cancelCompose() {
	if compose.mode == cmNone {
		return
	}
	if compose.mode == cmEdit {
		setText(hInput, "")
	}
	if compose.mode == cmFile {
		dropPasted(compose.file)
	}
	compose.mode = cmNone
	inputChanged()
	layout()
}

// composeLabels returns the two lines of the bar above the text field.
func composeLabels() (title, excerpt string) {
	if compose.mode == cmFile {
		name := filepath.Base(compose.file)
		if size := fileSizeLabel(compose.file); size != "" {
			name += " · " + size
		}
		if fileKind(compose.file) == "image" {
			return "Wyślesz zdjęcie", name
		}
		return "Wyślesz plik", name
	}
	m := compose.msg
	excerpt = oneLine(m.Text, 160)
	if excerpt == "" {
		excerpt = mediaLabel(m)
	}
	if compose.mode == cmEdit {
		return "Edytujesz wiadomość", excerpt
	}
	if m.SenderID == mv.self && mv.self != 0 {
		return "Odpowiadasz na swoją wiadomość", excerpt
	}
	return "Odpowiadasz: " + senderName(m), excerpt
}

func copyText(s string) {
	if s == "" {
		return
	}
	t := syscall.StringToUTF16(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n"))
	opened := false
	for try := 0; try < 10 && !opened; try++ { // another program may hold the clipboard for a moment
		if r, _, _ := pOpenClipboard.Call(hMain); r != 0 {
			opened = true
		} else {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !opened {
		log.Warn().Msg("copy: the clipboard is held by another program")
		be.notice("Nie udało się skopiować: schowek jest zajęty przez inny program.")
		return
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	h, _, _ := pGlobalAlloc.Call(0x0002, uintptr(len(t)*2)) // movable
	if h == 0 {
		return
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(t)), t)
	pGlobalUnlock.Call(h)
	pSetClipboardData.Call(13, h) // CF_UNICODETEXT; the clipboard owns the memory from here
}
