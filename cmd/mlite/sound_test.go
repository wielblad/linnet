//go:build windows

package main

import (
	"os"
	"testing"
	"time"
)

// TestMessageSound plays the message sound once; it runs only when MLITE_TEST_SOUND is set,
// because it is heard.
func TestMessageSound(t *testing.T) {
	if os.Getenv("MLITE_TEST_SOUND") == "" {
		t.Skip("MLITE_TEST_SOUND not set")
	}
	cfg.Sound = true
	if !playMessageSound() {
		t.Fatal("no Windows sound could be started")
	}
	t.Logf("played the Windows sound %q", soundPlayed)
	time.Sleep(1500 * time.Millisecond) // let it finish before the test process ends
	cfg.Sound = false
	if playMessageSound() {
		t.Fatal("the sound played although it is switched off")
	}
}
