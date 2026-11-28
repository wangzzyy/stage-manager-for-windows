package app

import (
	"math"

	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
	"github.com/egoist/mygo/ui"
)

// cardThumbAt is the destination and opacity requested for one window's
// live DWM picture in the current strip frame.
type cardThumbAt struct {
	area    ui.Rect
	opacity float32
}

// cardThumbnailArea maps a window's card-local rectangle through the same
// lift, vertical compensation, translation, and outer scale as the card.
// It is used only when tilt is zero, where the result is axis-aligned.
func cardThumbnailArea(box, window ui.Rect, originX, originY, lift, verticalCompensation, translateX, translateY, outerScale float32) ui.Rect {
	x := originX*box.W + (window.X-originX*box.W)*lift
	y := originY*box.H + (window.Y-verticalCompensation-originY*box.H)*lift
	left := box.X + translateX + x
	top := box.Y + translateY + y
	centerX := box.X + translateX + box.W/2
	centerY := box.Y + translateY + box.H/2
	return ui.Rect{
		X: centerX + (left-centerX)*outerScale,
		Y: centerY + (top-centerY)*outerScale,
		W: window.W * lift * outerScale,
		H: window.H * lift * outerScale,
	}
}

// placeCardThumbs updates the live DWM layer after the strip's card UI has
// been built. Thumbnails absent from this frame are hidden; windows no longer
// represented by a card are unregistered.
func (s *strip) placeCardThumbs() {
	if s.hwnd == 0 {
		return
	}
	if s.cardThumbs == nil {
		s.cardThumbs = make(map[winapi.HWND]*lineThumb)
		s.cardThumbFailed = make(map[winapi.HWND]bool)
	}
	present := make(map[winapi.HWND]bool)
	for _, g := range s.d.sets {
		for _, w := range g.windows {
			present[w.HWND] = true
		}
	}
	for h, th := range s.cardThumbs {
		if !present[h] {
			th.t.Unregister()
			delete(s.cardThumbs, h)
			delete(s.cardThumbFailed, h)
			continue
		}
		if _, wanted := s.cardThumbWant[h]; !wanted && th.visible {
			th.t.Hide()
			th.visible = false
		}
	}
	px := func(v float32) int32 { return int32(math.Round(float64(v) * s.scale)) }
	for h, at := range s.cardThumbWant {
		if at.opacity <= 0 || at.area.W <= 0 || at.area.H <= 0 {
			continue
		}
		th := s.cardThumbs[h]
		if th == nil {
			if s.cardThumbFailed[h] {
				continue
			}
			t, ok := winapi.RegisterThumbnail(s.hwnd, h)
			if !ok {
				debugf("cards: no DWM thumbnail of %x", h)
				s.cardThumbFailed[h] = true
				continue
			}
			th = &lineThumb{t: t}
			s.cardThumbs[h] = th
		}
		frame, win := winapi.FrameBounds(h), winapi.WindowRect(h)
		if frame.Empty() {
			frame = win
		}
		if frame.Empty() || win.Empty() {
			th.t.Hide()
			th.visible = false
			continue
		}
		src := winapi.Rect{
			Left: frame.Left - win.Left, Top: frame.Top - win.Top,
			Right: frame.Right - win.Left, Bottom: frame.Bottom - win.Top,
		}
		dest := winapi.Rect{
			Left: px(at.area.X + s.viewOffsetX), Top: px(at.area.Y),
			Right: px(at.area.X + s.viewOffsetX + at.area.W), Bottom: px(at.area.Y + at.area.H),
		}
		opacity := float64(at.opacity)
		if th.visible && dest == th.dest && src == th.src && math.Abs(opacity-th.op) < 0.004 {
			continue
		}
		th.t.Show(dest, src, opacity)
		th.dest, th.src, th.op, th.visible = dest, src, opacity, opacity > 0
	}
}

// dropCardThumbs releases every DWM picture owned by the card overlay.
func (s *strip) dropCardThumbs() {
	for h, th := range s.cardThumbs {
		th.t.Unregister()
		delete(s.cardThumbs, h)
	}
	s.cardThumbFailed = nil
	s.cardThumbWant = nil
}
