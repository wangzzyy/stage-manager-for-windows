//go:build windows

package app

import (
	"math"

	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
	"github.com/egoist/mygo/ui"
)

// cardThumb is the live DWM picture of a parked window in its card.
type cardThumb struct {
	t winapi.Thumbnail
	dest, src winapi.Rect
	visible bool
}

// showCardThumb places a live DWM picture over one flat card image. The
// caller only enables this for a settled, untransformed, single-window card;
// every other state keeps using the captured bitmap and flight layer.
func (s *strip) showCardThumb(h winapi.HWND, at ui.Rect) bool {
	if s.hwnd == 0 || s.scale <= 0 || at.W <= 0 || at.H <= 0 || !winapi.IsWindow(h) {
		return false
	}
	if s.cardThumbs == nil {
		s.cardThumbs = make(map[winapi.HWND]*cardThumb)
	}
	th := s.cardThumbs[h]
	if th == nil {
		t, ok := winapi.RegisterThumbnail(s.hwnd, h)
		if !ok {
			return false
		}
		th = &cardThumb{t: t}
		s.cardThumbs[h] = th
	}
	frame, win := winapi.FrameBounds(h), winapi.WindowRect(h)
	if frame.Empty() || win.Empty() {
		th.t.Hide()
		th.visible = false
		return false
	}
	src := winapi.Rect{
		Left: frame.Left - win.Left, Top: frame.Top - win.Top,
		Right: frame.Right - win.Left, Bottom: frame.Bottom - win.Top,
	}
	fit := fitIn(at, float64(frame.W())/float64(max(frame.H(), 1)))
	px := func(v float32) int32 { return int32(math.Round(float64(v) * s.scale)) }
	dest := winapi.Rect{
		Left: px(fit.X), Top: px(fit.Y),
		Right: px(fit.X + fit.W), Bottom: px(fit.Y + fit.H),
	}
	if dest.Empty() || src.Empty() {
		th.t.Hide()
		th.visible = false
		return false
	}
	if dest != th.dest || src != th.src || !th.visible {
		th.t.Show(dest, src, 1)
		th.dest, th.src, th.visible = dest, src, true
	}
	s.cardThumbSeen[h] = true
	return true
}

// placeCardThumbs hides live pictures as soon as a card starts moving,
// tilting, hovering, or flying. The static bitmap remains underneath and
// takes over in that same frame.
func (s *strip) placeCardThumbs() {
	for h, th := range s.cardThumbs {
		if !s.cardThumbSeen[h] && th.visible {
			th.t.Hide()
			th.visible = false
		}
		if !winapi.IsWindow(h) {
			th.t.Unregister()
			delete(s.cardThumbs, h)
		}
	}
}

// dropCardThumbs releases every DWM registration when the strip closes.
func (s *strip) dropCardThumbs() {
	for h, th := range s.cardThumbs {
		th.t.Unregister()
		delete(s.cardThumbs, h)
	}
}
