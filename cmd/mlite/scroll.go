//go:build windows

package main

// scroller gives a custom-drawn window smooth scrolling and a thin scroll bar that is
// drawn over the content instead of taking a strip of its own.

import "unsafe"

const (
	timerScroll  = 21
	timerBarHide = 22
)

type scroller struct {
	hwnd      uintptr
	pos       *int32       // scroll position in pixels, owned by the window
	total     func() int32 // height of the content
	page      func() int32 // height of the window
	changed   func()       // called when the position moved
	anim      int32        // pixels still to travel
	hot, zone bool         // pointer inside the window / over the bar
	drag      bool
	dragOff   int32
	shown     bool // scrolled a moment ago
	tracking  bool
}

func (s *scroller) max() int32 {
	if s.total == nil || s.page == nil || s.pos == nil { // messages can arrive before the window is set up
		return 0
	}
	m := s.total() - s.page()
	if m < 0 {
		m = 0
	}
	return m
}

// set moves to a position at once; it reports whether anything changed.
func (s *scroller) set(v int32) bool {
	if s.pos == nil {
		return false
	}
	if m := s.max(); v > m {
		v = m
	}
	if v < 0 {
		v = 0
	}
	if v == *s.pos {
		return false
	}
	*s.pos = v
	if s.changed != nil {
		s.changed()
	}
	invalidate(s.hwnd)
	return true
}

func (s *scroller) reveal() {
	s.shown = true
	pSetTimer.Call(s.hwnd, timerBarHide, 1100, 0)
}

// by scrolls smoothly by d pixels.
func (s *scroller) by(d int32) {
	if d == 0 || s.max() == 0 {
		return
	}
	if (d > 0) != (s.anim > 0) {
		s.anim = 0 // changing direction drops what was left of the old movement
	}
	s.anim += d
	s.reveal()
	pSetTimer.Call(s.hwnd, timerScroll, 10, 0)
}

func (s *scroller) wheel(delta int32) { s.by(-delta * dp(100) / 120) }

func (s *scroller) stop() {
	s.anim = 0
	pKillTimer.Call(s.hwnd, timerScroll)
}

// timer handles the scroller's timers; it reports whether the id was one of them.
func (s *scroller) timer(id uintptr) bool {
	switch id {
	case timerScroll:
		step := s.anim * 30 / 100
		if step == 0 {
			step = s.anim
		}
		s.anim -= step
		if !s.set(*s.pos+step) || s.anim == 0 {
			s.stop()
		}
		s.reveal()
		return true
	case timerBarHide:
		pKillTimer.Call(s.hwnd, timerBarHide)
		if s.shown && !s.drag {
			s.shown = false
			invalidate(s.hwnd)
		}
		return true
	}
	return false
}

// thumb returns the top and height of the bar's handle inside a window of height h.
func (s *scroller) thumb(h int32) (y, th int32, ok bool) {
	m, total := s.max(), s.total()
	if m == 0 || total <= 0 {
		return 0, 0, false
	}
	track := h - dp(8)
	th = int32(int64(track) * int64(h) / int64(total))
	if th < dp(32) {
		th = dp(32)
	}
	if th > track {
		th = track
	}
	y = dp(4) + int32(int64(track-th)*int64(*s.pos)/int64(m))
	return y, th, true
}

// paint draws the handle while the pointer is inside or the content just moved.
func (s *scroller) paint(g gfx, w, h int32, base uint32) {
	if !(s.hot || s.shown || s.drag) {
		return
	}
	y, th, ok := s.thumb(h)
	if !ok {
		return
	}
	bw, strength := dp(3), uint32(34)
	if s.zone || s.drag {
		bw, strength = dp(6), 52
	}
	g.pill(w-dp(3)-bw, y, bw, th, mix(pal.fg, base, strength))
}

func (s *scroller) leave() {
	s.tracking = false
	if s.hot || s.zone {
		s.hot, s.zone = false, false
		invalidate(s.hwnd)
	}
}

// mouse handles pointer messages for the bar; it reports whether the message was consumed.
func (s *scroller) mouse(msg uint32, x, y, w, h int32) bool {
	switch msg {
	case wmMouseMove:
		if !s.tracking {
			s.tracking = true
			trackLeave(s.hwnd)
		}
		if s.drag {
			_, th, ok := s.thumb(h)
			if span := h - dp(8) - th; ok && span > 0 {
				s.stop()
				s.set(int32(int64(y-s.dragOff-dp(4)) * int64(s.max()) / int64(span)))
			}
			return true
		}
		zone := x >= w-dp(14) && s.max() > 0
		if !s.hot || zone != s.zone {
			s.hot, s.zone = true, zone
			invalidate(s.hwnd)
		}
		return false
	case wmLButtonDown:
		if x < w-dp(14) || s.max() == 0 {
			return false
		}
		ty, th, _ := s.thumb(h)
		switch {
		case y < ty:
			s.by(-s.page() * 9 / 10)
		case y >= ty+th:
			s.by(s.page() * 9 / 10)
		default:
			s.drag, s.dragOff = true, y-ty
			pSetCapture.Call(s.hwnd)
		}
		return true
	case wmLButtonUp:
		if s.drag {
			s.drag = false
			pReleaseCapture.Call()
			s.reveal()
			invalidate(s.hwnd)
			return true
		}
	}
	return false
}

func lpPoint(lp uintptr) (int32, int32) { return int32(int16(lp)), int32(int16(lp >> 16)) }

var _ = unsafe.Sizeof(0)
