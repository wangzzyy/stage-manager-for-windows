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
// behind the strip. The strip can therefore draw its icons and controls over
// the pictures, unlike thumbnails registered against the strip itself.
type cardThumbs struct {
	s *strip
	win *mygo.Window
	hwnd winapi.HWND
	thumbs map[winapi.HWND]*cardThumb
	failed map[winapi.HWND]bool
	keepSet map[winapi.HWND]bool
	want map[winapi.HWND]cardThumbRequest
}

type cardThumb struct {
	t winapi.Thumbnail
	dest winapi.Rect
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
	clear(c.keepSet)
	clear(c.want)
}

func (c *cardThumbs) keep(h winapi.HWND) {
	if h != 0 {
		c.keepSet[h] = true
	}
}

// add asks DWM to draw h at dest. It returns true once a thumbnail exists,
// so the cached still can be omitted from the strip's own drawing.
func (c *cardThumbs) add(h winapi.HWND, dest winapi.Rect, opacity float64) bool {
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
	}
	c.want[h] = cardThumbRequest{dest: dest, opacity: math.Max(0, math.Min(opacity, 1))}
	return true
}

func (c *cardThumbs) endFrame() {
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
		if !th.visible || at.dest != th.dest || math.Abs(at.opacity-th.opacity) >= 0.004 {
			th.t.Show(at.dest, src, at.opacity)
			th.dest, th.opacity, th.visible = at.dest, at.opacity, at.opacity > 0
		}
		visible = visible || th.visible
	}
	if visible {
		c.place()
		if !winapi.IsWindowVisible(c.hwnd) {
			c.win.ShowInactive()
			c.place()
		}
	} else {
		c.hide()
	}
}

func (c *cardThumbs) place() {
	if c.hwnd == 0 || c.s.hwnd == 0 || !winapi.IsWindow(c.hwnd) {
		return
	}
	r := c.s.rect
	winapi.SetWindowPos(c.hwnd, c.s.hwnd, r.Left, r.Top, r.W(), r.H(),
		winapi.SwpNoActivate|winapi.SwpNoOwnerZOrder)
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

// cardThumbHandles returns registered source windows in a stable order for
// tests and diagnostics.
func (c *cardThumbs) cardThumbHandles() []winapi.HWND {
	out := make([]winapi.HWND, 0, len(c.thumbs))
	for h := range c.thumbs {
		out = append(out, h)
	}
	slices.Sort(out)
	return out
}
