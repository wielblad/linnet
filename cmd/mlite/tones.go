//go:build windows

package main

// The program's own message sounds: short, soft chimes made of sine waves when first played,
// so that no sound files travel with the program. Each is kept once made, because Windows
// plays it from memory after the call has returned.

import (
	"encoding/binary"
	"math"
	"sync"
)

const toneRate = 44100

// A note of a tone: its pitch in Hz, when it starts and how fast it fades (seconds to 1/e).
type toneNote struct {
	freq, start, decay float64
	gain               float64
	glide              float64 // pitch at the end, as a share of freq; 0 for a steady pitch
	overtone           float64 // loudness of a soft partial an octave and a fifth above
}

var tones = []struct {
	id, label string
	length    float64
	notes     []toneNote
}{
	{"", "Systemowy", 0, nil},
	{"drop", "Kropla", 0.35, []toneNote{{freq: 1250, decay: 0.07, gain: 1, glide: 0.62}}},
	{"bell", "Dzwoneczek", 1.1, []toneNote{{freq: 1568, decay: 0.32, gain: 0.8, overtone: 0.18}}},
	{"duo", "Dwa tony", 0.9, []toneNote{
		{freq: 1046.5, decay: 0.12, gain: 0.8, overtone: 0.1},
		{freq: 1318.5, start: 0.13, decay: 0.22, gain: 0.8, overtone: 0.1}}},
	{"wood", "Marimba", 0.6, []toneNote{
		{freq: 880, decay: 0.09, gain: 1, overtone: 0.3},
		{freq: 1174.7, start: 0.09, decay: 0.12, gain: 0.7, overtone: 0.3}}},
	{"chord", "Akord", 1.3, []toneNote{
		{freq: 523.3, decay: 0.35, gain: 0.55},
		{freq: 659.3, start: 0.06, decay: 0.35, gain: 0.5},
		{freq: 784.0, start: 0.12, decay: 0.4, gain: 0.5, overtone: 0.08}}},
}

var toneCache struct {
	sync.Mutex
	wav map[string][]byte
}

// toneWAV returns the WAV file of a sound of the program, nil for the Windows sound.
func toneWAV(id string) []byte {
	if id == "" {
		return nil
	}
	toneCache.Lock()
	defer toneCache.Unlock()
	if w, ok := toneCache.wav[id]; ok {
		return w
	}
	for _, t := range tones {
		if t.id == id && len(t.notes) > 0 {
			if toneCache.wav == nil {
				toneCache.wav = map[string][]byte{}
			}
			w := synthTone(t.length, t.notes)
			toneCache.wav[id] = w
			return w
		}
	}
	return nil
}

// synthTone mixes the notes into a mono 16-bit WAV at a gentle loudness.
func synthTone(length float64, notes []toneNote) []byte {
	n := int(length * toneRate)
	mix := make([]float64, n)
	for _, nt := range notes {
		phase, phase3 := 0.0, 0.0
		first := int(nt.start * toneRate)
		for i := first; i < n; i++ {
			t := float64(i-first) / toneRate
			f := nt.freq
			if nt.glide > 0 {
				f *= 1 + (nt.glide-1)*math.Min(t/0.12, 1)
			}
			phase += 2 * math.Pi * f / toneRate
			phase3 += 2 * math.Pi * f * 3 / toneRate
			env := math.Exp(-t / nt.decay)
			if a := 0.004; t < a { // a short rise: no click at the start
				env *= t / a
			}
			mix[i] += nt.gain * env * (math.Sin(phase) + nt.overtone*math.Sin(phase3))
		}
	}
	peak := 0.0
	for _, v := range mix {
		peak = math.Max(peak, math.Abs(v))
	}
	scale := 0.0
	if peak > 0 {
		scale = 0.32 * 32767 / peak // well below full scale: a message, not an alarm
	}
	fade := toneRate / 50 // the last 20 ms go to silence: no click at the end either
	out := make([]byte, 44+2*n)
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+2*n))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1) // PCM
	binary.LittleEndian.PutUint16(out[22:], 1) // mono
	binary.LittleEndian.PutUint32(out[24:], toneRate)
	binary.LittleEndian.PutUint32(out[28:], toneRate*2)
	binary.LittleEndian.PutUint16(out[32:], 2)
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(2*n))
	for i, v := range mix {
		v *= scale
		if left := n - i; left < fade {
			v *= float64(left) / float64(fade)
		}
		binary.LittleEndian.PutUint16(out[44+2*i:], uint16(int16(math.Round(v))))
	}
	return out
}
