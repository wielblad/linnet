//go:build windows

package main

// Links and non-picture attachments in a conversation: a line under the text that opens the
// page, plays a video in a small window of its own, or fetches and opens a file.

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/rs/zerolog/log"

	"lazyfmess/internal/store"
)

const (
	chipNone = iota
	chipFile
	chipLink
	chipPlay

	gLink  = ""
	gPlay  = ""
	gVideo = ""
	gMic   = ""
	gDoc   = ""
)

// firstURL returns the first web address written in the text.
func firstURL(s string) string {
	for _, w := range strings.Fields(s) {
		if !strings.HasPrefix(w, "https://") && !strings.HasPrefix(w, "http://") {
			continue
		}
		w = strings.TrimRight(w, ".,;:!?)\"'")
		if u, err := url.Parse(w); err == nil && u.Host != "" && !strings.ContainsAny(w, "\"<>") {
			return w
		}
	}
	return ""
}

func linkHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// youtubeID returns the video id of a YouTube address, or "".
func youtubeID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := linkHost(raw)
	id := ""
	switch {
	case host == "youtu.be":
		id = strings.Trim(u.Path, "/")
	case host == "youtube.com" || strings.HasSuffix(host, ".youtube.com"):
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		switch {
		case u.Path == "/watch":
			id = u.Query().Get("v")
		case len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live"):
			id = parts[1]
		}
	}
	if len(id) < 6 || len(id) > 15 {
		return ""
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return ""
		}
	}
	return id
}

// embedURL is the address of the video alone, without the page around it: YouTube's player,
// or Facebook's own player plugin. Empty when no such player is known for the address.
func embedURL(raw string) string {
	if id := youtubeID(raw); id != "" {
		return "https://www.youtube.com/embed/" + id + "?autoplay=1&rel=0&playsinline=1"
	}
	host := linkHost(raw)
	if host != "fb.watch" && host != "facebook.com" && !strings.HasSuffix(host, ".facebook.com") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || strings.HasPrefix(u.Path, "/plugins/") {
		return ""
	}
	return "https://www.facebook.com/plugins/video.php?show_text=false&autoplay=true&allowfullscreen=true&href=" + url.QueryEscape(raw)
}

// playableLink reports whether the address is a video page worth a window of its own.
func playableLink(raw string) bool {
	host := linkHost(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	p := u.Path
	switch {
	case host == "fb.watch", host == "vimeo.com", strings.HasSuffix(host, "tiktok.com"):
		return true
	case strings.HasSuffix(host, "facebook.com"):
		return strings.Contains(p, "/reel") || strings.Contains(p, "/watch") || strings.Contains(p, "/videos/") ||
			strings.HasPrefix(p, "/share/v/") || strings.HasPrefix(p, "/share/r/")
	case strings.HasSuffix(host, "instagram.com"):
		return strings.Contains(p, "/reel")
	}
	return false
}

// linkOf is the address a message links to: the first one written in it, else the one
// Messenger attached (a share without text).
func linkOf(m store.Message) string {
	if u := firstURL(m.Text); u != "" {
		return u
	}
	return firstURL(m.LinkURL)
}

// linkMessage stands in for the preview picture of a link in the picture cache.
func linkMessage(m store.Message) store.Message {
	return store.Message{ID: "lnk-" + m.ID, MediaKind: "image", MediaMime: "image/jpeg", MediaURL: m.LinkImage, MediaRef: m.LinkRef}
}

// ytMessage stands in for the preview picture of a YouTube video in the picture cache.
func ytMessage(id string) store.Message {
	return store.Message{ID: "yt-" + id, MediaKind: "image", MediaMime: "image/jpeg",
		MediaURL: "https://i.ytimg.com/vi/" + id + "/mqdefault.jpg"}
}

var dryOpen = os.Getenv("MLITE_DRYOPEN") != ""

func shellOpen(file, args string) bool {
	if dryOpen { // checks: note what would be opened instead of opening it
		log.Warn().Str("file", file).Str("args", args).Msg("would open")
		return true
	}
	var a uintptr
	if args != "" {
		a = uintptr(unsafe.Pointer(u16(args)))
	}
	r, _, _ := pShellExecuteW.Call(hMain, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16(file))), a, 0, 1)
	return r > 32
}

// openURL opens a web address in the default browser.
func openURL(raw string) {
	if u, err := url.Parse(raw); err == nil && (u.Scheme == "https" || u.Scheme == "http") && !strings.ContainsAny(raw, "\" ") {
		shellOpen(raw, "")
	}
}

// playInWindow plays a video page inside the program's own window.
func playInWindow(raw string) { openVideo(raw) }

// playExternal opens a video page in the default browser: the way out when no browser
// can be embedded in the window.
func playExternal(raw string) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || strings.ContainsAny(raw, "\" ") {
		return
	}
	openURL(raw)
}

// safeOpen opens a fetched file only when its type is a plain document, picture, sound or
// video; anything else is shown in its folder, never run.
func safeOpen(path string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".mp4", ".mov", ".webm", ".mkv",
		".mp3", ".m4a", ".aac", ".ogg", ".opus", ".wav", ".pdf", ".txt":
		shellOpen(path, "")
	default:
		shellOpen("explorer.exe", `/select,"`+path+`"`)
	}
}

func hasAttachment(m store.Message) bool {
	return m.MediaKind != "" && m.MediaKind != "image" && (m.MediaURL != "" || m.MediaRef != "") && !m.Unsent
}

func attachmentIcon(m store.Message) string {
	switch m.MediaKind {
	case "video":
		return gVideo
	case "audio", "voice":
		return gMic
	}
	return gDoc
}

// attachmentLabel is the line that stands for a video, a recording or a file.
func attachmentLabel(m store.Message) string {
	if md.busy[m.ID] {
		return "Pobieranie…"
	}
	label := mediaLabel(m)
	if m.MediaDurationMs > 0 {
		s := m.MediaDurationMs / 1000
		label += fmt.Sprintf(" · %d:%02d", s/60, s%60)
	}
	return label
}

// openAttachment opens a file of the conversation, fetching it first when needed.
func openAttachment(m store.Message) {
	if p, ok := mediaFile(m); ok {
		safeOpen(p)
		return
	}
	if md.busy[m.ID] {
		md.pending[m.ID] = m
		relayoutView()
	}
}
