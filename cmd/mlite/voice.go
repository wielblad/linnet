//go:build windows

package main

// Sending a voice message. The microphone button beside the paper clip starts the
// recording; the bar above the field then shows that it is being made and for how long.
// The send button ends it and sends it; the cross of the bar throws it away.

import (
	"os"
	"path/filepath"
	"time"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/store"
)

const (
	cmVoice = 10  // the field is given over to a recording being made
	idMic   = 337 // 333 to 336 are the buttons of the conversation's header
)

var hMic uintptr

// voiceStart begins a recording for the conversation being used.
func voiceStart() {
	if compose.mode == cmVoice || rec.on {
		return
	}
	if compose.mode != cmNone {
		cancelCompose()
	}
	if err := recStart(); err != nil {
		viewToast("Nie udało się włączyć mikrofonu.")
		return
	}
	audioStop() // what was playing would be recorded with the voice
	compose.mode = cmVoice
	rec.host = hostWnd()
	hostInputChanged()
	hostLayout()
}

// voiceSend ends the recording and sends it; it reports false when nothing was sent.
func voiceSend(t store.Thread) bool {
	path, ms, wave, err := recFinish()
	if err != nil {
		viewToast("Nagranie jest za krótkie.")
		return false
	}
	return be.sendVoice(t, path, ms, wave)
}

// voiceLabels are the two lines of the bar while a recording is made.
func voiceLabels() (title, excerpt string) {
	title = "● Nagrywanie · " + clock(recSeconds()*1000)
	if !rec.on || rec.dev == 0 {
		title = "Nagranie · " + clock(recSeconds()*1000)
	}
	return title, "Wyślij strzałką albo odrzuć krzyżykiem"
}

// voiceWatch throws a recording away when the field it was made for is gone: the
// conversation was closed or another one chosen.
func voiceWatch() {
	if !rec.on {
		return
	}
	kept := false
	typingBanks(func() {
		if compose.mode == cmVoice {
			kept = true
		}
	})
	if !kept {
		recCancel()
	}
}

// sendVoice sends a recording as a voice message and notes it locally, with its file, so
// that it can be played here at once.
func (b *backend) sendVoice(thread store.Thread, path string, ms int64, wave []float64) bool {
	return b.act("Nie udało się wysłać nagrania", func(c *meta.Client) error {
		result, err := c.SendVoice(b.ctx, thread, path, time.Duration(ms)*time.Millisecond, wave)
		if err != nil {
			return err // the file stays in the outbox
		}
		defer os.Remove(path)
		at := result.Timestamp
		if at.IsZero() {
			at = time.Now()
		}
		if result.MessageID != "" {
			sent := store.Message{ID: result.MessageID, ThreadID: thread.ID, SenderID: c.AccountID(), Timestamp: at,
				MediaKind: "audio", MediaMime: meta.VoiceMime, MediaName: "audio", MediaDurationMs: ms, MediaURL: "local"}
			if data, err := os.ReadFile(path); err == nil {
				dst := mediaPath(sent)
				if os.MkdirAll(filepath.Dir(dst), 0o700) == nil {
					_ = os.WriteFile(dst, data, 0o600)
				}
			}
			_ = b.db.UpsertMessage(b.ctx, sent)
			_ = b.db.SetLastRead(b.ctx, thread.ID, at)
		}
		return nil
	})
}
