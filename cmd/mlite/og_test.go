//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMetaHost(t *testing.T) {
	for addr, want := range map[string]bool{
		"https://www.facebook.com/reel/1":      true,
		"https://m.facebook.com/watch?v=1":     true,
		"https://fb.watch/abc":                 true,
		"https://www.instagram.com/reel/x/":    true,
		"https://facebook.com.example.org/x":   false,
		"https://notfacebook.com/reel/1":       false,
		"https://example.org/?u=facebook.com":  false,
		"https://www.youtube.com/watch?v=abc1": false,
	} {
		if got := metaHost(addr); got != want {
			t.Errorf("metaHost(%q) = %v, want %v", addr, got, want)
		}
	}
}

// TestOGFetch reads the preview of a real page; it runs only when MLITE_TEST_OG names one.
func TestOGFetch(t *testing.T) {
	addr := os.Getenv("MLITE_TEST_OG")
	if addr == "" {
		t.Skip("MLITE_TEST_OG not set")
	}
	path := filepath.Join(t.TempDir(), "preview.jpg")
	title, err := ogFetch(addr, path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() < 1000 {
		t.Fatalf("picture not saved: %v", err)
	}
	w, h := imageSize(path)
	t.Logf("title %d characters, picture %d bytes, %dx%d", len([]rune(title)), info.Size(), w, h)
}
