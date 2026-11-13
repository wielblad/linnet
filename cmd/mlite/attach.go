//go:build windows

package main

// Sending a picture or a file: the button beside the text field opens the Windows file
// dialog; the chosen file waits above the field until Enter or the send button.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/store"
)

const (
	idAttach = 332
	gAttach  = ""
)

var (
	comdlg32          = syscall.NewLazyDLL("comdlg32.dll")
	pGetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
	hAttach           uintptr
)

type openFileName struct {
	structSize    uint32
	owner         uintptr
	instance      uintptr
	filter        *uint16
	customFilter  *uint16
	maxCustFilter uint32
	filterIndex   uint32
	file          *uint16
	maxFile       uint32
	fileTitle     *uint16
	maxFileTitle  uint32
	initialDir    *uint16
	title         *uint16
	flags         uint32
	fileOffset    uint16
	fileExtension uint16
	defExt        *uint16
	custData      uintptr
	hook          uintptr
	templateName  *uint16
	reserved      uintptr
	reserved2     uint32
	flagsEx       uint32
}

// pickFile asks for a file and puts it above the text field, ready to send.
func pickFile() {
	if _, ok := threadByID(hostThreadID()); !ok {
		return
	}
	buf := make([]uint16, 1024)
	// The filter list is separated by zero characters, which the usual conversion refuses.
	filter := utf16.Encode([]rune("Wszystkie pliki\x00*.*\x00Zdjęcia\x00*.jpg;*.jpeg;*.png;*.gif;*.webp\x00\x00"))
	ofn := openFileName{owner: hostWnd(), filter: &filter[0], file: &buf[0], maxFile: uint32(len(buf)),
		title: u16("Wybierz plik do wysłania"), flags: 0x00080000 | 0x00001000 | 0x00000800 | 0x00000008}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))
	if r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return
	}
	attachFile(syscall.UTF16ToString(buf))
}

func fileSizeLabel(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	switch n := info.Size(); {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func fileKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return "image"
	case ".mp4", ".mov", ".webm", ".mkv":
		return "video"
	case ".mp3", ".m4a", ".aac", ".ogg", ".opus", ".wav":
		return "audio"
	}
	return "file"
}

// sendFile sends a file to the conversation and notes it locally until the server's copy arrives.
func (b *backend) sendFile(thread store.Thread, path string) bool {
	return b.act("Nie udało się wysłać pliku", func(c *meta.Client) error {
		result, err := c.SendFile(b.ctx, thread, path)
		if err != nil {
			return err
		}
		defer dropPasted(path)
		at := result.Timestamp
		if at.IsZero() {
			at = time.Now()
		}
		if result.MessageID != "" {
			sent := store.Message{ID: result.MessageID, ThreadID: thread.ID, SenderID: c.AccountID(),
				Timestamp: at, MediaKind: fileKind(path), MediaName: filepath.Base(path)}
			if sent.MediaKind == "image" { // keep a copy, so that the conversation shows the picture at once
				if data, err := os.ReadFile(path); err == nil {
					dst := mediaPath(sent)
					if os.MkdirAll(filepath.Dir(dst), 0o700) == nil {
						_ = os.WriteFile(dst, data, 0o600)
					}
				}
			}
			_ = b.db.UpsertMessage(b.ctx, sent)
			_ = b.db.SetLastRead(b.ctx, thread.ID, at)
		}
		return nil
	})
}

func drawAttach(d *drawItem) {
	w, h := d.rcItem.right, d.rcItem.bottom
	buf := newBuffer(d.hDC, w, h)
	c := canvas{buf.hdc}
	c.fill(rect{0, 0, w, h}, pal.card)
	ink := pal.fg2
	if d.itemState&odsSelected != 0 || hotBtn == d.hwndItem {
		g := newGfx(buf.hdc)
		g.round(0, 0, w, h, dp(4), dp(4), dp(4), dp(4), mix(pal.fg, pal.card, 9))
		g.free()
		ink = pal.fg
	}
	c.font(fnt.icon)
	c.text(gAttach, rect{0, 0, w, h}, ink, dtCenter|dtVCenter|dtSingleLine)
	if focusCue(d) {
		fr := rect{3, 3, w - 3, h - 3}
		drawFocus(buf.hdc, &fr)
	}
	buf.flush(0, 0)
}
