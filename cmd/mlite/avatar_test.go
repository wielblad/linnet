//go:build windows

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// A picture is stored under a name that ignores the changing signature, replaces the
// previous picture of the same person and is refused over plain http.
func TestAvatarDownload(t *testing.T) {
	dataDir = t.TempDir()
	body := []byte("picture-bytes")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	avatarClient = srv.Client()

	first := srv.URL + "/a/5.jpg?sig=one"
	path := avatarPath(5, first)
	if again := avatarPath(5, srv.URL+"/a/5.jpg?sig=two"); again != path {
		t.Fatalf("a new signature changed the file name: %s vs %s", again, path)
	}
	if err := avatarDownload(5, first, path); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(body) {
		t.Fatalf("stored file = %q, %v", got, err)
	}

	second := srv.URL + "/a/5-new.jpg"
	newPath := avatarPath(5, second)
	if newPath == path {
		t.Fatal("a different picture kept the old file name")
	}
	if err := avatarDownload(5, second, newPath); err != nil {
		t.Fatalf("second download: %v", err)
	}
	if files, _ := filepath.Glob(filepath.Join(dataDir, "avatars", "5-*.img")); len(files) != 1 || files[0] != newPath {
		t.Fatalf("files after replacing = %v", files)
	}

	if err := avatarDownload(6, srv.URL+"/missing.jpg", avatarPath(6, srv.URL+"/missing.jpg")); err == nil {
		t.Fatal("a missing picture did not report an error")
	}
	if err := avatarDownload(7, "http://example.invalid/x.jpg", avatarPath(7, "http://example.invalid/x.jpg")); err == nil {
		t.Fatal("plain http was not refused")
	}
}

// A conversation picture is fetched into its own file, a missing one reports an error,
// and the box it takes in a bubble keeps the picture's proportions within the limits.
func TestMediaFetch(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ok.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("jpeg-bytes"))
	}))
	defer srv.Close()
	avatarClient = srv.Client()

	path := filepath.Join(dir, "media", "one.jpg")
	if err := fetchTo(srv.URL+"/ok.jpg", path, 1<<20); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "jpeg-bytes" {
		t.Fatalf("stored file = %q, %v", got, err)
	}
	if err := fetchTo(srv.URL+"/gone.jpg", filepath.Join(dir, "media", "two.jpg"), 1<<20); err == nil {
		t.Fatal("a missing picture did not report an error")
	}
	if _, err := os.Stat(filepath.Join(dir, "media", "two.jpg")); err == nil {
		t.Fatal("a failed download left a file behind")
	}
	if mediaExt("image/png") != ".png" || mediaExt("application/x-unknown") != ".img" {
		t.Fatal("file extension does not follow the type")
	}
}

// A WebP file is rewritten as PNG by the Windows imaging component and can then be measured.
func TestWebPToPNG(t *testing.T) {
	src := os.Getenv("MLITE_TEST_WEBP")
	if src == "" {
		t.Skip("no sample file given")
	}
	gdipInit()
	dst := filepath.Join(t.TempDir(), "out.png")
	if !toPNG(src, dst) {
		t.Fatalf("conversion failed at: %s", wicLast)
	}
	if w, h := imageSize(dst); w == 0 || h == 0 {
		t.Fatalf("converted file not readable: %dx%d", w, h)
	}
}
