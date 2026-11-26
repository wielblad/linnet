//go:build windows

package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The picture handed to the clipboard has to be the picture: its size, its rows from the
// bottom up, and its colours in the order Windows keeps them (blue, green, red).
func TestPictureDIB(t *testing.T) {
	gdipInit()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			img.Set(x, y, color.NRGBA{0, 0, 255, 255}) // blue
		}
	}
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255}) // the upper left corner is red
	path := filepath.Join(t.TempDir(), "p.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	dib := pictureDIB(path)
	if len(dib) != 40+3*4*2 {
		t.Fatalf("got %d bytes, want a header and two rows of three points", len(dib))
	}
	if w, h := dib[4], dib[8]; w != 3 || h != 2 || dib[0] != 40 || dib[14] != 32 {
		t.Fatalf("header says %d x %d, size %d, %d bits", w, h, dib[0], dib[14])
	}
	lower, upper := dib[40:44], dib[40+12:40+16] // the first point of the lower row, then of the upper
	if lower[0] != 255 || lower[2] != 0 {
		t.Errorf("the lower left point is %v, want blue", lower[:3])
	}
	if upper[0] != 0 || upper[2] != 255 {
		t.Errorf("the upper left point is %v, want red", upper[:3])
	}
}
