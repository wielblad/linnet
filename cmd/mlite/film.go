//go:build windows

package main

// A film sent in a conversation: shown as a picture of itself with a play sign, and played
// in the program's own window. The picture is the one Windows makes of the film's file, so
// the file has to be here first: a short film is fetched as soon as it is on the screen, a
// long one when it is first played. The film plays in the embedded browser from a page on
// this computer only, served the way the YouTube player's page is.

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/rs/zerolog/log"

	"lazyfmess/internal/store"
	"lazyfmess/internal/webview"
)

const filmAutoFetchMs = 90_000 // films up to this long are fetched for their picture unasked

var (
	pSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")

	film struct {
		state map[string]int // 0 not asked, 1 being made, 2 there, 3 cannot be made
	}
)

func isFilm(m store.Message) bool {
	return m.MediaKind == "video" && !m.Unsent && m.MediaMime != "image/gif"
}

// filmPicture stands in for the picture of a film in the picture cache.
func filmPicture(m store.Message) store.Message {
	return store.Message{ID: "vth-" + m.ID, MediaKind: "image", MediaMime: "image/jpeg"}
}

// filmHave reports whether the picture of a film has been made.
func filmHave(m store.Message) bool {
	if film.state == nil {
		film.state = map[string]int{}
	}
	if film.state[m.ID] == 0 {
		if _, err := os.Stat(mediaPath(filmPicture(m))); err == nil {
			film.state[m.ID] = 2
		}
	}
	return film.state[m.ID] == 2
}

// filmKick makes the picture of a film once its file is here, fetching a short film for it.
// The conversation is laid out again when the picture is there.
func filmKick(m store.Message) {
	if filmHave(m) || film.state[m.ID] != 0 {
		return
	}
	path := mediaPath(m)
	if _, err := os.Stat(path); err != nil {
		if m.MediaDurationMs > 0 && m.MediaDurationMs <= filmAutoFetchMs {
			mediaFile(m) // asked for; the view is drawn again when it has come
		}
		return
	}
	film.state[m.ID] = 1
	dst := mediaPath(filmPicture(m))
	id := m.ID
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		pCoInitializeEx.Call(0, 2)
		defer pCoUninitialize.Call()
		ok := shellPicture(path, dst)
		postFilm(id, ok)
	}()
}

// filmDone is the picture of a film made, or found impossible; it runs in the window's thread.
var filmResults = make(chan [2]string, 32)

func postFilm(id string, ok bool) {
	state := "3"
	if ok {
		state = "2"
	}
	select {
	case filmResults <- [2]string{id, state}:
	default:
	}
	be.requestRefresh()
}

// filmCollect takes in the pictures made since the last look; called before a layout.
func filmCollect() {
	for {
		select {
		case r := <-filmResults:
			if film.state == nil {
				film.state = map[string]int{}
			}
			film.state[r[0]] = int(r[1][0] - '0')
		default:
			return
		}
	}
}

// shellPicture saves the picture Windows makes of a file, a frame of a film, as a JPEG.
func shellPicture(src, dst string) bool {
	iid := syscall.GUID{Data1: 0xbcc18b79, Data2: 0xba16, Data3: 0x442f, Data4: [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b}} // IShellItemImageFactory
	var factory uintptr
	if hr, _, _ := pSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(u16(src))), 0, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&factory))); int32(hr) < 0 || factory == 0 {
		return false
	}
	defer com(factory, 2)
	var hbm uintptr
	size := uintptr(640) | uintptr(640)<<32 // SIZE by value
	// a real picture of the file only, never the icon of its kind; a larger one is fine
	if int32(com(factory, 3, size, 0x8|0x1, uintptr(unsafe.Pointer(&hbm)))) < 0 || hbm == 0 {
		return false
	}
	defer pDeleteObject.Call(hbm)
	var bmp uintptr
	if st, _, _ := pGdipFromHBITMAP.Call(hbm, 0, uintptr(unsafe.Pointer(&bmp))); st != 0 || bmp == 0 {
		return false
	}
	defer pGdipDisposeImage.Call(bmp)
	jpeg := syscall.GUID{Data1: 0x557CF401, Data2: 0x1A04, Data3: 0x11D3, Data4: [8]byte{0x9A, 0x73, 0x00, 0x00, 0xF8, 0x1E, 0xF3, 0x2E}}
	tmp := dst + ".tmp"
	if st, _, _ := pGdipSaveImageToFile.Call(bmp, uintptr(unsafe.Pointer(u16(tmp))), uintptr(unsafe.Pointer(&jpeg)), 0); st != 0 {
		os.Remove(tmp)
		return false
	}
	return os.Rename(tmp, dst) == nil
}

// filmPage serves one film file to the embedded browser: a page with the film on it and
// the file itself, behind names nobody can guess, on the loopback address only.
func filmPage(path string) (string, error) {
	playerStop()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	name := make([]byte, 16)
	if _, err := rand.Read(name); err != nil {
		ln.Close()
		return "", err
	}
	page := "/" + hex.EncodeToString(name)
	file := page + "/film"
	html := []byte(`<!doctype html><meta charset="utf-8"><title>Odtwarzacz</title>` +
		`<style>html,body{margin:0;height:100%;background:#000;overflow:hidden}` +
		`video{position:fixed;inset:0;width:100%;height:100%;object-fit:contain;outline:0}</style>` +
		`<video src="` + file + `" controls autoplay playsinline></video>`)
	mux := http.NewServeMux()
	mux.HandleFunc(page, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(html)
	})
	mux.HandleFunc(file, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeFile(w, r, path) // answers the browser's requests for parts of the film
	})
	srv := &http.Server{Handler: mux}
	player.srv = srv
	go srv.Serve(ln)
	return "http://" + ln.Addr().String() + page, nil
}

// openFilm plays a film file of the conversation in the program's own window. Where no
// browser can be embedded, or the person would rather not, the file opens in the program
// Windows has for films.
func openFilm(m store.Message, path string) {
	if cfg.VideoExternal || dryOpen || !webview.Available() {
		safeOpen(path)
		return
	}
	closeViewer() // it also takes away the page of the film before
	page, err := filmPage(path)
	if err != nil {
		log.Warn().Err(err).Msg("film page")
		safeOpen(path)
		return
	}
	vw.video, vw.film, vw.webMsg = page, path, "Ładowanie odtwarzacza…"
	vw.filmTitle = "Wideo"
	if m.MediaDurationMs > 0 {
		vw.filmTitle += " · " + clock(m.MediaDurationMs)
	}
	vw.list, vw.hot, vw.note = nil, -1, ""
	vw.host, vw.bank = hostWnd(), curBank
	vw.hwnd = createWindow(0, "MliteViewer", "Odtwarzacz", wsChild|wsVisible|wsTabStop|wsClipChildren, vw.host, 0)
	hostLayout()
	pSetFocus.Call(vw.hwnd)
	failed := func(err error) {
		log.Warn().Err(err).Msg("film player")
		if vw.film == path {
			vw.webMsg = "Nie udało się uruchomić odtwarzacza. Użyj przycisku „Otwórz plik”."
			invalidate(vw.hwnd)
		}
	}
	// A browser does not start a film with sound unasked; here the person has just asked for
	// it. The browser is told so when it is made, and it gets a place of its own for what it
	// keeps, so that it never meets one made without that word.
	const args = "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"
	old, had := os.LookupEnv(args)
	os.Setenv(args, "--autoplay-policy=no-user-gesture-required")
	defer func() {
		if had {
			os.Setenv(args, old)
		} else {
			os.Unsetenv(args)
		}
	}()
	view, err := webview.OpenWith(vw.hwnd, [4]int32{0, dp(56), vw.w, vw.h}, filepath.Join(dataDir, "webview-film"), page, nil, "", func(err error) {
		if err != nil {
			failed(err)
		}
	})
	if err != nil {
		failed(err)
		return
	}
	vw.web = view
}
