//go:build windows

package main

import (
	"bytes"
	"testing"
)

// oggPage writes one Ogg page holding whole packets, the way a recording is made of them.
func oggPage(packets ...[]byte) []byte {
	var table, body []byte
	for _, p := range packets {
		for n := len(p); ; n -= 255 {
			if n < 255 {
				table = append(table, byte(n))
				break
			}
			table = append(table, 255)
		}
		body = append(body, p...)
	}
	page := append([]byte("OggS"), make([]byte, 22)...)
	page = append(page, byte(len(table)))
	return append(append(page, table...), body...)
}

func TestOggOpusToMatroska(t *testing.T) {
	head := append([]byte("OpusHead"), 1, 1, 0x38, 0x01, 0x80, 0xBB, 0, 0, 0, 0, 0) // one channel, 312 samples thrown away
	tags := []byte("OpusTags\x00\x00\x00\x00\x00\x00\x00\x00")
	long := append([]byte{0x78}, bytes.Repeat([]byte{7}, 300)...) // 20 ms of CELT, longer than one piece of a page
	short := []byte{0x08, 1, 2, 3}                                // 20 ms of SILK
	data := append(append(oggPage(head), oggPage(tags)...), oggPage(long, short, short)...)

	packets, err := oggPackets(data)
	if err != nil || len(packets) != 5 || !bytes.Equal(packets[2], long) || !bytes.Equal(packets[4], short) {
		t.Fatalf("packets: %d, %v", len(packets), err)
	}
	if s := opusSamples(long); s != 960 {
		t.Errorf("CELT 20 ms: %d samples, want 960", s)
	}
	if s := opusSamples([]byte{0x09, 0}); s != 1920 {
		t.Errorf("two SILK frames of 20 ms: %d samples, want 1920", s)
	}
	out, err := oggOpusToMatroska(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte{0x1A, 0x45, 0xDF, 0xA3}) || !bytes.Contains(out, []byte("A_OPUS")) || !bytes.Contains(out, head) || !bytes.Contains(out, long) {
		t.Error("the Matroska file lacks its head, its codec, the Opus head or the sound")
	}
	if _, err := oggOpusToMatroska(oggPage([]byte("vorbis-something-else"), tags, short)); err == nil {
		t.Error("sound that is not Opus was taken for Opus")
	}
}
