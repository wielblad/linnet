//go:build windows

package main

// Pictures inside conversations: fetched once into a folder next to the database and shown
// in the bubble at a modest size. Only pictures of the open conversation are kept decoded.

import (
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"lazyfmess/internal/store"
)

const wmAppMedia = wmApp + 6

var pShellExecuteW = shell32.NewProc("ShellExecuteW")

type thumb struct {
	img  uintptr
	w, h int32
}

type mediaDone struct {
	id string
	ok bool
}

var md struct {
	files   map[string]string        // local file by message id
	busy    map[string]bool          // download running
	failed  map[string]time.Time     // last failed attempt
	dims    map[string][2]int32      // pixel size of the file; zero when it cannot be decoded here
	thumbs  map[string]thumb         // decoded, scaled, rounded
	pending map[string]store.Message // files to open as soon as they arrive
	shown   map[string]string        // the file that is drawn, when it differs from the fetched one
	done    chan mediaDone
}

func init() {
	md.files, md.busy, md.failed = map[string]string{}, map[string]bool{}, map[string]time.Time{}
	md.dims, md.thumbs = map[string][2]int32{}, map[string]thumb{}
	md.pending = map[string]store.Message{}
	md.shown = map[string]string{}
	md.done = make(chan mediaDone, 256)
}

// showsInline reports whether the message carries a picture this window can fetch.
func showsInline(m store.Message) bool {
	if m.MediaKind != "image" || m.Unsent {
		return false
	}
	if m.MediaURL != "" || m.MediaRef != "" {
		return true
	}
	// a picture sent from here has no address yet: it is shown from the copy kept at sending
	if _, ok := md.files[m.ID]; ok {
		return true
	}
	_, err := os.Stat(mediaPath(m))
	return err == nil
}

// placeholderLabel turns the words the core stores for attachments it could not keep into a label.
func placeholderLabel(text string) string {
	switch text {
	case "[image]":
		return "Zdjęcie"
	case "[video]":
		return "Wideo"
	case "[sticker]":
		return "Naklejka"
	case "[audio]", "[voice]":
		return "Nagranie głosowe"
	case "[file]":
		return "Plik"
	case "[gif]":
		return "GIF"
	}
	return ""
}

// previewIcon is the small sign shown before the preview of a conversation whose last
// message is an attachment, and the words that go with it.
func previewIcon(preview string, last [2]string) (string, string) {
	kind := ""
	if strings.HasPrefix(preview, "[") && strings.HasSuffix(preview, "]") && placeholderLabel(preview) != "" {
		kind = strings.Trim(preview, "[]")
	}
	if kind == "" {
		return "", ""
	}
	switch kind {
	case "image", "sticker", "gif":
		return gPhoto, placeholderLabel(preview)
	case "video":
		return gVideo, "Wideo"
	case "audio", "voice":
		return gMic, "Nagranie głosowe"
	}
	if name := last[1]; last[0] == "file" && name != "" && name != "file" { // a file is best told by its name
		return gDoc, name
	}
	return gDoc, "Plik"
}

// previewText is the last message as shown in lists and notifications.
func previewText(s string) string {
	if l := placeholderLabel(s); l != "" {
		return l
	}
	return s
}

func mediaExt(mime string) string {
	switch mime {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	case "audio/aac":
		return ".aac"
	case "audio/ogg":
		return ".ogg"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	}
	return ".img"
}

func mediaPath(m store.Message) string {
	h := fnv.New64a()
	h.Write([]byte(m.ID))
	ext := mediaExt(m.MediaMime)
	if e := strings.ToLower(filepath.Ext(m.MediaName)); ext == ".img" && len(e) > 1 && len(e) <= 6 && !strings.ContainsAny(e, " /\\") {
		ext = e // keep the sender's file type, so the right program opens it
	}
	return filepath.Join(dataDir, "media", fmt.Sprintf("%016x%s", h.Sum64(), ext))
}

// mediaFile returns the local file of a picture and starts the download when it is missing.
func mediaFile(m store.Message) (string, bool) {
	if p, ok := md.files[m.ID]; ok {
		return p, true
	}
	if md.busy[m.ID] || time.Since(md.failed[m.ID]) < 10*time.Minute {
		return "", false
	}
	p := mediaPath(m)
	if _, err := os.Stat(p); err == nil {
		md.files[m.ID] = p
		return p, true
	}
	if av.offline {
		md.failed[m.ID] = time.Now()
		return "", false
	}
	md.busy[m.ID] = true
	go func(id, raw, ref string) {
		av.sem <- struct{}{}
		var err error
		if ref != "" { // encrypted chat: fetched and decrypted through the connection
			err = be.fetchEncrypted(ref, p)
		} else {
			err = fetchTo(raw, p, 16<<20)
		}
		<-av.sem
		md.done <- mediaDone{id, err == nil}
		postMsg(hMain, wmAppMedia, 0, 0)
	}(m.ID, m.MediaURL, m.MediaRef)
	return "", false
}

// fetchTo downloads an https address into a file, through a temporary file.
func fetchTo(raw, path string, limit int64) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return fmt.Errorf("address not usable")
	}
	resp, err := avatarClient.Get(raw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, limit))
	f.Close()
	if err != nil || n == 0 {
		os.Remove(tmp)
		return fmt.Errorf("download incomplete")
	}
	return os.Rename(tmp, path)
}

// mediaArrived runs on the window thread after downloads ended.
func mediaArrived() {
	changed := false
	for {
		select {
		case d := <-md.done:
			delete(md.busy, d.id)
			if !d.ok {
				md.failed[d.id] = time.Now()
			}
			if m, wanted := md.pending[d.id]; wanted {
				delete(md.pending, d.id)
				if d.ok {
					openAttachment(m)
				}
			}
			changed = true
			continue
		default:
		}
		break
	}
	if changed {
		relayoutView()
	}
}

// mediaBox returns the size a picture takes inside a bubble with the given inner width,
// and whether the picture itself can be drawn already.
func mediaBox(m store.Message, inner int32) (w, h int32, ready bool) {
	maxW, maxH := dp(280), dp(320)
	if inner < maxW {
		maxW = inner
	}
	d, known := md.dims[m.ID]
	if !known {
		p, ok := mediaFile(m)
		if !ok {
			return placeholderBox(maxW)
		}
		shown, pw, ph := drawable(p) // a WebP sticker becomes a PNG copy the window can draw
		md.shown[m.ID] = shown
		d = [2]int32{pw, ph}
		md.dims[m.ID] = d
	}
	if d[0] <= 0 || d[1] <= 0 {
		return placeholderBox(maxW)
	}
	w, h = d[0], d[1]
	if w > maxW {
		h, w = h*maxW/w, maxW
	}
	if h > maxH {
		w, h = w*maxH/h, maxH
	}
	if w < dp(48) {
		w = dp(48)
	}
	if h < dp(48) {
		h = dp(48)
	}
	return w, h, true
}

func placeholderBox(maxW int32) (int32, int32, bool) {
	w := dp(220)
	if w > maxW {
		w = maxW
	}
	return w, dp(120), false
}

// mediaNote is the text shown in place of a picture that is not drawn.
func mediaNote(m store.Message) string {
	switch {
	case md.busy[m.ID]:
		return "Pobieranie zdjęcia…"
	case md.files[m.ID] != "":
		return "Kliknij, aby otworzyć zdjęcie"
	case av.offline:
		return "Zdjęcie"
	}
	return "Nie udało się pobrać zdjęcia"
}

// mediaThumb returns the scaled picture with rounded corners; 0 when it cannot be made.
func mediaThumb(m store.Message, w, h int32) uintptr {
	if t, ok := md.thumbs[m.ID]; ok && t.w == w && t.h == h {
		return t.img
	}
	if t, ok := md.thumbs[m.ID]; ok && t.img != 0 {
		pGdipDisposeImage.Call(t.img)
	}
	if len(md.thumbs) > 30 {
		mediaDropThumbs()
	}
	src := md.shown[m.ID]
	if src == "" {
		src = md.files[m.ID]
	}
	img := makeThumb(src, w, h, dp(10))
	md.thumbs[m.ID] = thumb{img, w, h}
	return img
}

// mediaDropThumbs frees the decoded pictures; the files stay.
func mediaDropThumbs() {
	for id, t := range md.thumbs {
		if t.img != 0 {
			pGdipDisposeImage.Call(t.img)
		}
		delete(md.thumbs, id)
	}
}

func makeThumb(path string, w, h, radius int32) uintptr { return thumbOf(path, w, h, radius, false) }

// thumbOf scales a picture file into a rounded box. With cover the middle of the picture
// is cut to the box's shape instead of being stretched to it.
func thumbOf(path string, w, h, radius int32, cover bool) uintptr {
	if path == "" {
		return 0
	}
	var src uintptr
	if st, _, _ := pGdipLoadImageFromFile.Call(uintptr(unsafe.Pointer(u16(path))), uintptr(unsafe.Pointer(&src))); st != 0 || src == 0 {
		return 0
	}
	defer pGdipDisposeImage.Call(src)
	var sw, sh uint32
	pGdipGetImageWidth.Call(src, uintptr(unsafe.Pointer(&sw)))
	pGdipGetImageHeight.Call(src, uintptr(unsafe.Pointer(&sh)))
	if sw == 0 || sh == 0 {
		return 0
	}
	scaled, g1 := newBitmap(w, h)
	if scaled == 0 {
		return 0
	}
	pGdipSetInterpolationMode.Call(uintptr(g1), 7)
	sx, sy, cw, ch := int32(0), int32(0), int32(sw), int32(sh)
	if cover {
		if cw*h > ch*w { // wider than the box: cut the sides
			nw := ch * w / h
			sx, cw = (cw-nw)/2, nw
		} else {
			nh := cw * h / w
			sy, ch = (ch-nh)/2, nh
		}
		if cw < 1 || ch < 1 {
			sx, sy, cw, ch = 0, 0, int32(sw), int32(sh)
		}
	}
	pGdipDrawImageRectRectI.Call(uintptr(g1), src, iarg(-1), iarg(-1), uintptr(w+2), uintptr(h+2), uintptr(sx), uintptr(sy), uintptr(cw), uintptr(ch), 2, 0, 0, 0)
	g1.free()
	out, g2 := newBitmap(w, h)
	if out != 0 {
		var brush uintptr
		pGdipCreateTexture.Call(scaled, 0, uintptr(unsafe.Pointer(&brush)))
		if brush != 0 {
			g2.roundBrush(0, 0, w, h, radius, brush)
			pGdipDeleteBrush.Call(brush)
		}
		g2.free()
	}
	pGdipDisposeImage.Call(scaled)
	return out
}

// roundBrush fills a rounded rectangle with the given brush.
func (g gfx) roundBrush(x, y, w, h, r int32, brush uintptr) {
	if r*2 > w {
		r = w / 2
	}
	if r*2 > h {
		r = h / 2
	}
	if r < 1 {
		r = 1
	}
	var path uintptr
	pGdipCreatePath.Call(0, uintptr(unsafe.Pointer(&path)))
	pGdipAddPathArcI.Call(path, iarg(x), iarg(y), iarg(r*2), iarg(r*2), farg(180), farg(90))
	pGdipAddPathArcI.Call(path, iarg(x+w-r*2), iarg(y), iarg(r*2), iarg(r*2), farg(270), farg(90))
	pGdipAddPathArcI.Call(path, iarg(x+w-r*2), iarg(y+h-r*2), iarg(r*2), iarg(r*2), farg(0), farg(90))
	pGdipAddPathArcI.Call(path, iarg(x), iarg(y+h-r*2), iarg(r*2), iarg(r*2), farg(90), farg(90))
	pGdipClosePathFigure.Call(path)
	pGdipFillPath.Call(uintptr(g), brush, path)
	pGdipDeletePath.Call(path)
}

func (g gfx) image(img uintptr, x, y, w, h int32) {
	pGdipDrawImageRectI.Call(uintptr(g), img, iarg(x), iarg(y), iarg(w), iarg(h))
}

// openMedia shows the full picture in the program Windows uses for that file type.
func openMedia(m store.Message) {
	if p, ok := mediaFile(m); ok {
		safeOpen(p)
	}
}
