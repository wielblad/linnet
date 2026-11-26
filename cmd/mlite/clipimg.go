//go:build windows

package main

// A picture copied to the clipboard, the way Windows' own programs do it: as the picture
// itself, for pasting into a drawing or a message, and as its file, for pasting into a
// folder or a program that takes files.

import (
	"syscall"
	"time"
	"unsafe"

	"github.com/rs/zerolog/log"
)

const (
	cfDIB   = 8
	cfHDrop = 15
)

var (
	pGdipCreateHBITMAPFromBitmap = gdiplus.NewProc("GdipCreateHBITMAPFromBitmap")
	pGetObjectW                  = gdi32.NewProc("GetObjectW")
)

// dibSection is what Windows tells of a bitmap made as a DIB section.
type dibSection struct {
	bmType, bmWidth, bmHeight, bmWidthBytes int32
	bmPlanes, bmBitsPixel                   uint16
	_                                       uint32
	bmBits                                  uintptr
	// BITMAPINFOHEADER
	biSize                     uint32
	biWidth, biHeight          int32
	biPlanes, biBitCount       uint16
	biCompression, biSizeImage uint32
	biXPels, biYPels           int32
	biClrUsed, biClrImportant  uint32
	dsBitfields                [3]uint32
	dshSection                 uintptr
	dsOffset                   uint32
}

// globalBytes puts bytes into memory the clipboard can take over.
func globalBytes(data []byte) uintptr {
	h, _, _ := pGlobalAlloc.Call(0x0002, uintptr(len(data))) // movable
	if h == 0 {
		return 0
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return 0
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
	pGlobalUnlock.Call(h)
	return h
}

// pictureDIB reads a picture file into the form the clipboard keeps pictures in: a header
// and its rows from the bottom up, 32 bits a point. Transparent parts become white.
func pictureDIB(path string) []byte {
	shown, _, _ := drawable(path) // a WebP picture becomes a PNG copy that can be read
	var img uintptr
	if st, _, _ := pGdipLoadImageFromFile.Call(uintptr(unsafe.Pointer(u16(shown))), uintptr(unsafe.Pointer(&img))); st != 0 || img == 0 {
		return nil
	}
	defer pGdipDisposeImage.Call(img)
	var hbm uintptr
	if st, _, _ := pGdipCreateHBITMAPFromBitmap.Call(img, uintptr(unsafe.Pointer(&hbm)), 0xFFFFFFFF); st != 0 || hbm == 0 {
		return nil
	}
	defer pDeleteObject.Call(hbm)
	var ds dibSection
	if n, _, _ := pGetObjectW.Call(hbm, unsafe.Sizeof(ds), uintptr(unsafe.Pointer(&ds))); n == 0 || ds.bmBits == 0 || ds.bmBitsPixel != 32 {
		return nil
	}
	w, h := int(ds.bmWidth), int(ds.bmHeight)
	if w <= 0 || h <= 0 || w*h > 64<<20 {
		return nil
	}
	stride := w * 4
	out := make([]byte, 40+stride*h)
	hdr := (*[10]uint32)(unsafe.Pointer(&out[0]))
	hdr[0], hdr[1], hdr[2] = 40, uint32(w), uint32(h) // size of the header, width, height (bottom-up)
	hdr[3] = 1 | 32<<16                               // one plane, 32 bits a point
	hdr[5] = uint32(stride * h)
	src := unsafe.Slice((*byte)(unsafe.Pointer(ds.bmBits)), stride*h)
	topDown := ds.biHeight < 0
	for y := 0; y < h; y++ {
		from := y
		if topDown { // the clipboard wants the last row first
			from = h - 1 - y
		}
		copy(out[40+y*stride:40+(y+1)*stride], src[from*stride:(from+1)*stride])
	}
	return out
}

// copyPicture puts a picture on the clipboard, as a picture and as its file. It reports
// whether anything was put there.
func copyPicture(path string) bool {
	dib := pictureDIB(path)
	// DROPFILES: where the names begin, a point, two flags (the names are wide), then the
	// name and two zeros.
	name := syscall.StringToUTF16(path)
	drop := make([]byte, 20+len(name)*2+2)
	*(*uint32)(unsafe.Pointer(&drop[0])) = 20
	*(*uint32)(unsafe.Pointer(&drop[16])) = 1
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&drop[20])), len(name)), name)

	opened := false
	for try := 0; try < 10 && !opened; try++ { // another program may hold the clipboard for a moment
		if r, _, _ := pOpenClipboard.Call(hMain); r != 0 {
			opened = true
		} else {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !opened {
		log.Warn().Msg("copy picture: the clipboard is held by another program")
		return false
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	put := false
	if dib != nil {
		if h := globalBytes(dib); h != 0 {
			if r, _, _ := pSetClipboardData.Call(cfDIB, h); r != 0 { // the clipboard owns the memory from here
				put = true
			}
		}
	}
	if h := globalBytes(drop); h != 0 {
		if r, _, _ := pSetClipboardData.Call(cfHDrop, h); r != 0 {
			put = true
		}
	}
	return put
}
