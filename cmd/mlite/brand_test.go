//go:build windows

package main

import (
	"os"
	"testing"
)

// TestBrandContrast checks the colour pairs of the Linnet identity as the program really
// combines them, in both themes: text needs 4.5:1, the edges of controls and focus marks 3:1.
func TestBrandContrast(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		os.Setenv("MLITE_THEME", theme)
		os.Setenv("MLITE_HC", "")
		loadPalette()
		p := pal
		type pair struct {
			name   string
			fg, bg uint32
			min    float64
		}
		edge := mix(p.fg2, p.line, 65)
		toast := mix(p.fg, p.layer, 92)
		pairs := []pair{
			{"text on list", p.fg, p.mica, 4.5},
			{"text on conversation", p.fg, p.layer, 4.5},
			{"text on incoming bubble", p.fg, p.bubble, 4.5},
			{"text on selected row", p.fg, p.sel, 4.5},
			{"secondary on list", p.fg2, p.mica, 4.5},
			{"secondary on conversation", p.fg2, p.layer, 4.5},
			{"secondary on incoming bubble", p.fg2, p.bubble, 4.5},
			{"secondary on selected row", p.fg2, p.sel, 4.5},
			{"text on own bubble", p.ownFg, p.own, 4.5},
			{"time on own bubble", p.ownMeta, p.own, 4.5},
			{"accent text on list", p.accentText, p.mica, 4.5},
			{"accent text on conversation", p.accentText, p.layer, 4.5},
			{"accent text on incoming bubble", p.accentText, p.bubble, 4.5},
			{"glyph on accent", p.onAccent, p.accent, 4.5},
			{"glyph on accent, hover", p.onAccent, p.accentHover, 4.5},
			{"glyph on accent, pressed", p.onAccent, p.accentDown, 4.5},
			{"error text on conversation", p.signal, p.layer, 4.5},
			{"toast text", p.layer, toast, 4.5},
			{"viewer text", viewerInk, viewerGround, 4.5},
			{"viewer secondary", viewerInk2, viewerGround, 4.5},
			{"text on the band about a new version", 0xFFFFFF, bandGreen, 4.5},
			{"dots on a head, someone writing", 0xFFFFFF, headTyping, 3},
			{"tick on a head, message read", 0xFFFFFF, headSeen, 3},
			{"focus edge on field", p.accent, p.card, 3},
			{"focus edge on list", p.accent, p.mica, 3},
			{"unread badge on list", p.accent, p.mica, 3},
			{"switch outline, off", p.fg2, p.card, 3},
			{"field edge at rest", edge, p.card, 3},
			{"incoming bubble against conversation (informative)", p.bubble, p.layer, 0},
		}
		for _, c := range pairs {
			got := contrast(c.fg, c.bg)
			t.Logf("%-5s %-50s %06X on %06X  %5.2f:1  (needs %.1f)", theme, c.name, c.fg, c.bg, got, c.min)
			if got < c.min {
				t.Errorf("%s: %s is %.2f:1, below %.1f:1", theme, c.name, got, c.min)
			}
		}
	}
	os.Unsetenv("MLITE_THEME")
}
