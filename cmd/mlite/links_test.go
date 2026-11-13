//go:build windows

package main

import "testing"

func TestEmbedURL(t *testing.T) {
	for addr, want := range map[string]string{
		"https://www.youtube.com/watch?v=aqz-KE-bpKQ":  "https://www.youtube.com/embed/aqz-KE-bpKQ?autoplay=1&rel=0&playsinline=1",
		"https://youtu.be/aqz-KE-bpKQ?t=30":            "https://www.youtube.com/embed/aqz-KE-bpKQ?autoplay=1&rel=0&playsinline=1",
		"https://www.youtube.com/shorts/aqz-KE-bpKQ":   "https://www.youtube.com/embed/aqz-KE-bpKQ?autoplay=1&rel=0&playsinline=1",
		"https://www.facebook.com/reel/1234567890":     "https://www.facebook.com/plugins/video.php?show_text=false&autoplay=true&allowfullscreen=true&href=https%3A%2F%2Fwww.facebook.com%2Freel%2F1234567890",
		"https://fb.watch/abcdef/":                     "https://www.facebook.com/plugins/video.php?show_text=false&autoplay=true&allowfullscreen=true&href=https%3A%2F%2Ffb.watch%2Fabcdef%2F",
		"https://www.facebook.com/plugins/video.php?x": "", // already the player: left alone
		"https://vimeo.com/123456":                     "", // no player of our own for it
		"https://example.org/film":                     "",
	} {
		if got := embedURL(addr); got != want {
			t.Errorf("embedURL(%q)\n got %q\nwant %q", addr, got, want)
		}
	}
}
