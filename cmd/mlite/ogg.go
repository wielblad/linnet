//go:build windows

package main

// Messenger's voice recordings are Opus sound in an Ogg file. Windows has the decoder for
// Opus but does not read Ogg files; it does read Matroska ones. So the sound is moved, as
// it is, from the one kind of file into the other: nothing is decoded or lost on the way.
// The copy is kept beside the fetched file and made once.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
)

// playableSound gives a file Windows can play for a fetched recording: the file itself, or
// the Matroska copy of an Ogg Opus one.
func playableSound(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return path
	}
	head := make([]byte, 4)
	_, err = f.Read(head)
	f.Close()
	if err != nil || string(head) != "OggS" {
		return path
	}
	copyPath := path + ".mka"
	if _, err := os.Stat(copyPath); err == nil {
		return copyPath
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 64<<20 {
		return path
	}
	out, err := oggOpusToMatroska(data)
	if err != nil {
		return path
	}
	tmp := copyPath + ".tmp"
	if os.WriteFile(tmp, out, 0o600) != nil || os.Rename(tmp, copyPath) != nil {
		os.Remove(tmp)
		return path
	}
	return copyPath
}

// oggPackets takes the packets out of an Ogg file with one stream in it.
func oggPackets(data []byte) ([][]byte, error) {
	var packets [][]byte
	var cur []byte
	for len(data) >= 27 {
		if string(data[:4]) != "OggS" {
			return nil, errors.New("ogg: lost the page")
		}
		n := int(data[26])
		if len(data) < 27+n {
			break
		}
		table := data[27 : 27+n]
		body := data[27+n:]
		for _, seg := range table {
			if len(body) < int(seg) {
				return nil, errors.New("ogg: page cut short")
			}
			cur = append(cur, body[:seg]...)
			body = body[seg:]
			if seg < 255 { // a packet ends with a piece shorter than the longest
				packets = append(packets, cur)
				cur = nil
			}
		}
		data = body
	}
	return packets, nil
}

// opusSamples tells how many samples (at 48 kHz) an Opus packet holds, from its first byte.
func opusSamples(p []byte) int64 {
	if len(p) == 0 {
		return 0
	}
	cfg := p[0] >> 3
	var frame int64 // in samples at 48 kHz
	switch {
	case cfg < 12: // SILK: 10, 20, 40, 60 ms
		frame = []int64{480, 960, 1920, 2880}[cfg%4]
	case cfg < 16: // hybrid: 10, 20 ms
		frame = []int64{480, 960}[cfg%2]
	default: // CELT: 2.5, 5, 10, 20 ms
		frame = []int64{120, 240, 480, 960}[cfg%4]
	}
	switch p[0] & 3 {
	case 0:
		return frame
	case 1, 2:
		return 2 * frame
	}
	if len(p) < 2 {
		return 0
	}
	return int64(p[1]&0x3F) * frame
}

// ebml writes one element: its name, its length, its content.
func ebml(id uint32, content []byte) []byte {
	var out []byte
	for shift := 24; shift >= 0; shift -= 8 {
		if b := byte(id >> shift); b != 0 || len(out) > 0 {
			out = append(out, b)
		}
	}
	size := make([]byte, 8)
	binary.BigEndian.PutUint64(size, uint64(len(content)))
	size[0] = 0x01 // a length written in eight bytes
	return append(append(out, size...), content...)
}

// oggOpusToMatroska moves Opus sound from an Ogg file into a Matroska one.
func oggOpusToMatroska(data []byte) ([]byte, error) {
	packets, err := oggPackets(data)
	if err != nil {
		return nil, err
	}
	if len(packets) < 3 || len(packets[0]) < 19 || string(packets[0][:8]) != "OpusHead" {
		return nil, errors.New("ogg: not Opus sound")
	}
	head := packets[0]
	channels := uint64(head[9])
	preSkip := uint64(binary.LittleEndian.Uint16(head[10:12]))
	sound := packets[2:] // after the head and the tags

	num := func(id uint32, v uint64) []byte {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, v)
		i := 0
		for i < 7 && b[i] == 0 {
			i++
		}
		return ebml(id, b[i:])
	}
	float := func(id uint32, v float64) []byte {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, math.Float64bits(v))
		return ebml(id, b)
	}
	text := func(id uint32, s string) []byte { return ebml(id, []byte(s)) }
	join := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

	var total int64
	for _, p := range sound {
		total += opusSamples(p)
	}
	header := ebml(0x1A45DFA3, join(
		num(0x4286, 1), num(0x42F7, 1), num(0x42F2, 4), num(0x42F3, 8),
		text(0x4282, "matroska"), num(0x4287, 4), num(0x4285, 2),
	))
	info := ebml(0x1549A966, join(
		num(0x2AD7B1, 1000000), // times are in milliseconds
		text(0x4D80, "Linnet"), text(0x5741, "Linnet"),
		float(0x4489, float64(total)/48),
	))
	track := ebml(0x1654AE6B, ebml(0xAE, join(
		num(0xD7, 1), num(0x73C5, 1), num(0x83, 2), // the first track, sound
		text(0x86, "A_OPUS"),
		num(0x56AA, preSkip*1000000000/48000), // what the decoder throws away at the start
		num(0x56BB, 80000000),
		ebml(0x63A2, head),
		ebml(0xE1, join(float(0xB5, 48000), num(0x9F, channels))),
	)))

	var clusters, blocks []byte
	var at, clusterAt int64 // samples
	flush := func() {
		if len(blocks) > 0 {
			clusters = append(clusters, ebml(0x1F43B675, join(num(0xE7, uint64(clusterAt/48)), blocks))...)
			blocks = nil
		}
	}
	for _, p := range sound {
		if at-clusterAt >= 5*48000 { // a new group every five seconds
			flush()
			clusterAt = at
		}
		rel := uint16((at - clusterAt) / 48)
		block := append([]byte{0x81, byte(rel >> 8), byte(rel), 0x80}, p...) // track 1, its time, a key frame
		blocks = append(blocks, ebml(0xA3, block)...)
		at += opusSamples(p)
	}
	flush()
	return join(header, ebml(0x18538067, join(info, track, clusters))), nil
}
