//go:build windows

package main

// YouTube plays only inside a frame on a page of someone else's: its player opened as a page
// of its own answers "error 153", and a page kept in a file has no address it accepts. So the
// program hands the player a page of its own, from this computer only: a listener on the
// loopback address, one page behind a name nobody can guess, and nothing else served. It
// lives only as long as the video is on the screen.

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
)

var player struct {
	srv *http.Server
}

// playerPage starts that little server for one video and gives back the page's address. The
// id is written into the page, so it comes from youtubeID, which lets nothing else through.
func playerPage(id string) (string, error) {
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
	path := "/" + hex.EncodeToString(name)
	// Some videos may not be played outside YouTube's own page. The player says so, and then
	// the whole page opens after all, the way it did before there was a player here.
	page := []byte(`<!doctype html><meta charset="utf-8"><title>Odtwarzacz</title>` +
		`<style>html,body{margin:0;height:100%;background:#000;overflow:hidden}` +
		`#p,iframe{position:fixed;inset:0;width:100%;height:100%;border:0}</style>` +
		`<div id="p"></div><script src="https://www.youtube.com/iframe_api"></script><script>
var id = "` + id + `", left = false, loaded = false;
function whole() { if (!left) { left = true; location.replace("https://www.youtube.com/watch?v=" + id); } }
function onYouTubeIframeAPIReady() {
  loaded = true;
  new YT.Player("p", {videoId: id, playerVars: {autoplay: 1, rel: 0, playsinline: 1}, events: {onError: whole}});
}
setTimeout(function () { if (!loaded) { whole(); } }, 6000);
</script>`)
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(page)
	})
	srv := &http.Server{Handler: mux}
	player.srv = srv
	go srv.Serve(ln)
	return "http://" + ln.Addr().String() + path, nil
}

// playerStop takes the page away again; the video is gone from the screen by then.
func playerStop() {
	if player.srv != nil {
		player.srv.Close()
		player.srv = nil
	}
}
