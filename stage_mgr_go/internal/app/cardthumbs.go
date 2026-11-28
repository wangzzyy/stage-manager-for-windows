//go:build windows

package app

import (
	"math"
	"slices"

	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// cardThumbs keeps DWM's live window pictures in a transparent window just
// above the strip. The live picture intentionally covers the card's decorative
// UI while that card is being operated.
type cardThumbs struct {
	s *strip
	win *mygo.Window
	hwnd winapi.HWND
	rect winapi.Rect
	placed bool
	thumbs map[winapi.HWND]*cardThumb
	failed map[winapi.HWND]bool
	keepSet map[winapi.HWND]bool
	want map[winapi.HWND]cardThumbRequest
	frameOrder []winapi.HWND
	frameSeen map[winapi.HWND]bool
	order []winapi.HWND
}

type cardThumb struct {
	t winapi.Thumbnail
	dest winapi.Rect
	src winapi.Rect
	opacity float64
	visible bool
}

type cardThumbRequest struct {
	dest winapi.Rect
	opacity float64
}

func newCardThumbs(s *strip) *cardThumbs {
	c := &cardThumbs{
		s: s,
		thumbs: make(map[winapi.HWND]*cardThumb),
		failed: make(map[winapi.HWND]bool),
		keepSet: make(map[winapi.HWND]bool),
		want: make(map[winapi.HWND]cardThumbRequest),
		frameSeen: make(map[winapi.HWND]bool),
	}
	c.win = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray: s.a.tray, Hidden: true, Width: int(s.a.settings.Width), Height: 400,
		Content: ui.View(func(ctx *ui.Context) { ctx.Root().Background(ui.Transparent) }),
	})
	c.hwnd = winapi.HWND(c.win.NativeHandle())
	winapi.SetLayered(c.hwnd)
	winapi.SetExStyleBits(c.hwnd, winapi.WsExTransparent, 0)
	winapi.SetTransitionsDisabled(c.hwnd, true)
	return c
}

func (c *cardThumbs) beginFrame() {
	if c == nil { return }
	clear(c.keepSet)
	clear(c.want)
	clear(c.frameSeen)
	c.frameOrder = c.frameOrder[:0]
}

func (c *cardThumbs) keep(h winapi.HWND) {
	if c != nil && h != 0 {
		c.keepSet[h] = true
		if !c.frameSeen[h] {
			c.frameSeen[h] = true
			c.frameOrder = append(c.frameOrder, h)
		}
	}
}

// add asks DWM to draw h at dest. It returns true once a thumbnail exists,
// so the cached still can be omitted from the strip's own drawing.
func (c *cardThumbs) add(h winapi.HWND, dest winapi.Rect, opacity float64) bool {
	if c == nil { return false }
	c.keep(h)
	if dest.Empty() || opacity <= 0 || !winapi.IsWindow(h) {
		return false
	}
	th := c.thumbs[h]
	if th == nil {
		if c.failed[h] {
			return false
		}
		t, ok := winapi.RegisterThumbnail(c.hwnd, h)
		if !ok {
			c.failed[h] = true
			debugf("card DWM: no thumbnail of %x", h)
			return false
		}
		th = &cardThumb{t: t}
		c.thumbs[h] = th
		c.order = append(c.order, h)
	}
	c.want[h] = cardThumbRequest{dest: dest, opacity: math.Max(0, math.Min(opacity, 1))}
	return true
}

func (c *cardThumbs) endFrame() {
	if c == nil { return }
	c.syncOrder()
	visible := false
	for h, th := range c.thumbs {
		if !c.keepSet[h] {
			th.t.Unregister()
			delete(c.thumbs, h)
			delete(c.failed, h)
			continue
		}
		at, ok := c.want[h]
		if !ok || !c.s.visible || !c.s.shown || c.s.away || c.s.d.inOverview {
			if th.visible {
				th.t.Hide()
				th.visible = false
			}
			continue
		}
		frame, win := winapi.FrameBounds(h), winapi.WindowRect(h)
		if frame.Empty() || win.Empty() {
			th.t.Hide()
			th.visible = false
			continue
		}
		src := winapi.Rect{
			Left: frame.Left - win.Left, Top: frame.Top - win.Top,
			Right: frame.Right - win.Left, Bottom: frame.Bottom - win.Top,
		}
		if !th.visible || at.dest != th.dest || src != th.src || math.Abs(at.opacity-th.opacity) >= 0.004 {
			th.t.Show(at.dest, src, at.opacity)
			th.dest, th.src, th.opacity, th.visible = at.dest, src, at.opacity, at.opacity > 0
		}
		visible = visible || th.visible
	}
	if visible {
		if !winapi.IsWindowVisible(c.hwnd) {
			c.win.ShowInactive()
			c.place()
		} else if !c.placed || c.rect != c.s.rect {
			c.place()
		}
	} else {
		c.hide()
	}
}

// syncOrder re-registers thumbnails in the same back-to-front order used
// to draw the card windows. DWM establishes thumbnail overlap order when
// relationships are registered; changing destination rectangles alone does
// not reorder them. This matters after two cards are merged.
func (c *cardThumbs) syncOrder() {
	want := make([]winapi.HWND, 0, len(c.frameOrder))
	for _, h := range c.frameOrder {
		if c.thumbs[h] != nil {
			want = append(want, h)
		}
	}
	if slices.Equal(want, c.order) {
		return
	}

	old := c.thumbs
	for _, th := range old {
		th.t.Unregister()
	}
	c.thumbs = make(map[winapi.HWND]*cardThumb, len(want))
	c.order = c.order[:0]
	for _, h := range want {
		th := old[h]
		if th == nil {
			continue
		}
		t, ok := winapi.RegisterThumbnail(c.hwnd, h)
		if !ok {
			c.failed[h] = true
			continue
		}
		th.t = t
		th.visible = false
		c.thumbs[h] = th
		c.order = append(c.order, h)
	}
}

func (c *cardThumbs) place() {
	if c == nil || c.hwnd == 0 || c.s.hwnd == 0 || !winapi.IsWindow(c.hwnd) {
		return
	}
	r := c.s.rect
	// Keep the host in the same z-order band as the strip, then put the strip
	// immediately behind it. This makes DWM content cover the whole card UI
	// without leaving the host topmost when the strip itself is lowered.
	z := c.s.zOrder()
	winapi.SetWindowPos(c.hwnd, z, r.Left, r.Top, r.W(), r.H(),
		winapi.SwpNoActivate|winapi.SwpNoOwnerZOrder)
	winapi.SetWindowPos(c.s.hwnd, c.hwnd, 0, 0, 0, 0,
		winapi.SwpNoMove|winapi.SwpNoSize|winapi.SwpNoActivate|winapi.SwpNoOwnerZOrder)
	c.rect, c.placed = r, true
}

func (c *cardThumbs) hide() {
	if c != nil && c.hwnd != 0 && winapi.IsWindowVisible(c.hwnd) {
		winapi.Hide(c.hwnd)
	}
}

func (c *cardThumbs) close() {
	if c == nil {
		return
	}
	for h, th := range c.thumbs {
		th.t.Unregister()
		delete(c.thumbs, h)
	}
	if c.win != nil {
		c.win.Destroy()
	}
}

