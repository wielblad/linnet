//go:build windows

package main

import (
	"encoding/binary"
	"os"
	"testing"
	"time"
)

func TestToneWAV(t *testing.T) {
	if toneWAV("") != nil {
		t.Fatal("the Windows sound has no WAV of its own")
	}
	for _, tn := range tones[1:] {
		w := toneWAV(tn.id)
		if len(w) < 44 || string(w[:4]) != "RIFF" || string(w[8:12]) != "WAVE" {
			t.Fatalf("%s: not a WAV file", tn.id)
		}
		if got := int(binary.LittleEndian.Uint32(w[40:])); got != len(w)-44 {
			t.Fatalf("%s: data size %d, file holds %d", tn.id, got, len(w)-44)
		}
		peak, last := 0, int16(binary.LittleEndian.Uint16(w[len(w)-2:]))
		for i := 44; i+1 < len(w); i += 2 {
			v := int(int16(binary.LittleEndian.Uint16(w[i:])))
			if v < 0 {
				v = -v
			}
			if v > peak {
				peak = v
			}
		}
		if peak == 0 || peak > 32767/2 {
			t.Fatalf("%s: peak %d, want a quiet but audible sound", tn.id, peak)
		}
		if last > 50 || last < -50 {
			t.Fatalf("%s: ends at %d, want silence", tn.id, last)
		}
		if &toneWAV(tn.id)[0] != &w[0] {
			t.Fatalf("%s: made again instead of kept", tn.id)
		}
	}
}

// TestTonesHeard plays every sound of the program; only with MLITE_TEST_SOUND, because it is heard.
func TestTonesHeard(t *testing.T) {
	if os.Getenv("MLITE_TEST_SOUND") == "" {
		t.Skip("MLITE_TEST_SOUND not set")
	}
	for _, tn := range tones[1:] {
		if !playTone(tn.id) {
			t.Fatalf("%s did not start", tn.id)
		}
		time.Sleep(time.Duration(tn.length*1000+300) * time.Millisecond)
	}
}
