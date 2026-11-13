//go:build windows

package main

import (
	"os"
	"testing"
	"time"
)

// TestSoundChoices checks that every sound offered is really on this computer, and that the
// name shown follows the choice.
func TestSoundChoices(t *testing.T) {
	for _, s := range messageSounds {
		if s.file == "" {
			continue
		}
		path := soundPath(s.file)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s: %v", s.name, err)
		}
	}
	defer func(was string) { cfg.SoundFile = was }(cfg.SoundFile)
	cfg.SoundFile = ""
	if got := soundName(); got != "Systemowy dźwięk wiadomości" {
		t.Errorf("soundName() with nothing chosen = %q", got)
	}
	cfg.SoundFile = "chimes.wav"
	if got := soundName(); got != "Kuranty" {
		t.Errorf("soundName() for chimes.wav = %q", got)
	}
	cfg.SoundFile = `C:\Dzwieki\moj dzwiek.wav`
	if got := soundName(); got != "moj dzwiek.wav" {
		t.Errorf("soundName() for a file of one's own = %q", got)
	}
	if got := soundPath(cfg.SoundFile); got != cfg.SoundFile {
		t.Errorf("soundPath() for a full path = %q", got)
	}
}

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
	defer func(was string) { cfg.SoundFile = was }(cfg.SoundFile)
	cfg.SoundFile = "chimes.wav" // and the chosen one, from a file
	if !playMessageSound() {
		t.Fatal("the chosen sound could not be started")
	}
	if soundPlayed != soundPath(cfg.SoundFile) {
		t.Fatalf("played %q instead of the chosen file", soundPlayed)
	}
	t.Logf("played the chosen file %q", soundPlayed)
	time.Sleep(2500 * time.Millisecond)
	cfg.SoundFile = ""
	cfg.Sound = false
	if playMessageSound() {
		t.Fatal("the sound played although it is switched off")
	}
}
