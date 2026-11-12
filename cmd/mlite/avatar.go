//go:build windows

package main

// Profile pictures: fetched once into a folder next to the database, shown as small circles.
// Only the sizes that are actually drawn are kept in memory.

import (
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

var (
	pGdipLoadImageFromFile    = gdiplus.NewProc("GdipLoadImageFromFile")
	pGdipDisposeImage         = gdiplus.NewProc("GdipDisposeImage")
	pGdipGetImageWidth        = gdiplus.NewProc("GdipGetImageWidth")
	pGdipGetImageHeight       = gdiplus.NewProc("GdipGetImageHeight")
	pGdipCreateBitmapScan0    = gdiplus.NewProc("GdipCreateBitmapFromScan0")
	pGdipGetImageGraphicsCtx  = gdiplus.NewProc("GdipGetImageGraphicsContext")
	pGdipSetInterpolationMode = gdiplus.NewProc("GdipSetInterpolationMode")
	pGdipDrawImageRectRectI   = gdiplus.NewProc("GdipDrawImageRectRectI")
	pGdipDrawImageRectI       = gdiplus.NewProc("GdipDrawImageRectI")
	pGdipCreateTexture        = gdiplus.NewProc("GdipCreateTexture")
)

// avatarClient fetches pictures; tests replace it.
var avatarClient = &http.Client{Timeout: 20 * time.Second}

type avKey struct {
	id int64
	d  int32
}

var av struct {
	offline bool
	urls    map[int64]string    // picture address by contact or thread id, from the store
	files   map[int64]string    // downloaded file by id
	busy    map[int64]bool      // download running
	failed  map[int64]time.Time // last failed attempt
	imgs    map[avKey]uintptr   // ready circles by id and diameter; 0 = file could not be decoded
	sem     chan struct{}
}

func init() {
	av.urls, av.files, av.busy = map[int64]string{}, map[int64]string{}, map[int64]bool{}
	av.failed, av.imgs = map[int64]time.Time{}, map[avKey]uintptr{}
	av.sem = make(chan struct{}, 3)
}

// avatarPath names the cache file. Only the path of the address counts: the signature in the
// query changes over time while the picture stays the same.
func avatarPath(id int64, raw string) string {
	key := raw
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		key = u.Path
	}
	h := fnv.New32a()
	h.Write([]byte(key))
	return filepath.Join(dataDir, "avatars", fmt.Sprintf("%d-%08x.img", id, h.Sum32()))
}

// avatarsSync takes the current picture addresses from the store and forgets pictures that changed.
func avatarsSync(urls map[int64]string) {
	for id, u := range urls {
		if old := av.urls[id]; old != u && (old == "" || avatarPath(id, old) != avatarPath(id, u)) {
			delete(av.files, id)
			delete(av.failed, id)
			avatarForget(id)
		}
	}
	av.urls = urls
}

func avatarForget(id int64) {
	for k, img := range av.imgs {
		if k.id == id {
			if img != 0 {
				pGdipDisposeImage.Call(img)
			}
			delete(av.imgs, k)
		}
	}
}

// avatarsDrop frees every decoded picture; they are rebuilt from the files when drawn again.
func avatarsDrop() {
	for k, img := range av.imgs {
		if img != 0 {
			pGdipDisposeImage.Call(img)
		}
		delete(av.imgs, k)
	}
}

// avatarImage returns the circle of the given diameter, or 0 while there is no picture (yet).
func avatarImage(id int64, d int32) uintptr {
	key := avKey{id, d}
	if img, ok := av.imgs[key]; ok {
		return img
	}
	raw := av.urls[id]
	if raw == "" || d <= 0 {
		return 0
	}
	path := avatarPath(id, raw)
	if av.files[id] != path {
		if av.busy[id] || time.Since(av.failed[id]) < 10*time.Minute {
			return 0
		}
		if _, err := os.Stat(path); err != nil {
			if av.offline {
				av.failed[id] = time.Now()
				return 0
			}
			av.busy[id] = true
			go avatarFetch(id, raw, path)
			return 0
		}
		av.files[id] = path
	}
	img := makeCircle(path, d)
	av.imgs[key] = img
	return img
}

// avatarFetch downloads one picture and tells the window when it is there.
func avatarFetch(id int64, raw, path string) {
	av.sem <- struct{}{}
	ok := avatarDownload(id, raw, path) == nil
	<-av.sem
	done := uintptr(0)
	if ok {
		done = 1
	}
	postMsg(hMain, wmAppAvatar, done, uintptr(id))
}

func avatarDownload(id int64, raw, path string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return fmt.Errorf("picture address not usable")
	}
	resp, err := avatarClient.Get(raw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("picture: status %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 4<<20))
	f.Close()
	if err != nil || n == 0 {
		os.Remove(tmp)
		return fmt.Errorf("picture: download incomplete")
	}
	if old, _ := filepath.Glob(filepath.Join(filepath.Dir(path), fmt.Sprintf("%d-*.img", id))); len(old) > 0 {
		for _, p := range old {
			os.Remove(p)
		}
	}
	return os.Rename(tmp, path)
}

// avatarArrived runs on the window thread when a download ended.
func avatarArrived(id int64, ok bool) {
	delete(av.busy, id)
	if !ok {
		av.failed[id] = time.Now()
		return
	}
	avatarForget(id)
	invalidate(hList)
	invalidate(hMain)
	invalidate(hView)
	headsRedraw()
}

// makeCircle decodes the file and returns a d-by-d bitmap holding the picture cut to a circle.
func makeCircle(path string, d int32) uintptr {
	var src uintptr
	if st, _, _ := pGdipLoadImageFromFile.Call(uintptr(unsafe.Pointer(u16(path))), uintptr(unsafe.Pointer(&src))); st != 0 || src == 0 {
		return 0
	}
	defer pGdipDisposeImage.Call(src)
	var w, h uint32
	pGdipGetImageWidth.Call(src, uintptr(unsafe.Pointer(&w)))
	pGdipGetImageHeight.Call(src, uintptr(unsafe.Pointer(&h)))
	if w == 0 || h == 0 {
		return 0
	}
	side := w
	if h < side {
		side = h
	}
	sx, sy := (w-side)/2, (h-side)/2
	const pargb = 0x000E200B // 32 bits per pixel with premultiplied alpha

	var square, g1 uintptr
	pGdipCreateBitmapScan0.Call(uintptr(d), uintptr(d), 0, pargb, 0, uintptr(unsafe.Pointer(&square)))
	if square == 0 {
		return 0
	}
	pGdipGetImageGraphicsCtx.Call(square, uintptr(unsafe.Pointer(&g1)))
	pGdipSetInterpolationMode.Call(g1, 7) // high quality bicubic
	pGdipSetPixelOffset.Call(g1, 4)
	// drawn one pixel larger on each side so the resampled edge does not fade
	pGdipDrawImageRectRectI.Call(g1, src, iarg(-1), iarg(-1), uintptr(d+2), uintptr(d+2),
		uintptr(sx), uintptr(sy), uintptr(side), uintptr(side), 2, 0, 0, 0)
	pGdipDeleteGraphics.Call(g1)

	var out, g2, brush uintptr
	pGdipCreateBitmapScan0.Call(uintptr(d), uintptr(d), 0, pargb, 0, uintptr(unsafe.Pointer(&out)))
	if out != 0 {
		pGdipGetImageGraphicsCtx.Call(out, uintptr(unsafe.Pointer(&g2)))
		pGdipSetSmoothing.Call(g2, 4)
		pGdipSetPixelOffset.Call(g2, 4)
		pGdipCreateTexture.Call(square, 0, uintptr(unsafe.Pointer(&brush)))
		if brush != 0 {
			pGdipFillEllipseI.Call(g2, brush, 0, 0, uintptr(d), uintptr(d))
			pGdipDeleteBrush.Call(brush)
		}
		pGdipDeleteGraphics.Call(g2)
	}
	pGdipDisposeImage.Call(square)
	return out
}

// avatar draws the picture of a person or group; without one it draws the coloured circle
// and reports false, so the caller adds the initials.
func (g gfx) avatar(id int64, x, y, d int32) bool {
	if img := avatarImage(id, d); img != 0 {
		pGdipDrawImageRectI.Call(uintptr(g), img, iarg(x), iarg(y), iarg(d), iarg(d))
		return true
	}
	g.ellipse(x, y, d, d, avatarColor(id))
	return false
}
