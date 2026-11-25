//go:build windows

package main

import "testing"

// The edge of a window has to show against the window itself and against a white or a
// black page under it, in both themes.
func TestEdgeShows(t *testing.T) {
	old := cfg.Theme
	defer func() { cfg.Theme = old; loadPalette() }()
	for _, theme := range []string{"light", "dark"} {
		cfg.Theme = theme
		t.Setenv("MLITE_THEME", theme)
		loadPalette()
		under := uint32(0xFFFFFF)
		if pal.dark {
			under = 0x000000
		}
		for _, bg := range []uint32{pal.layer, under} {
			if c := contrast(pal.edge, bg); c < 1.9 {
				t.Errorf("%s: edge %06X on %06X is %.2f:1, want at least 1.9:1", theme, pal.edge, bg, c)
			}
		}
	}
}
