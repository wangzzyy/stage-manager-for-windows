package app

import (
	"time"

	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
)

const cardTearoffTime = 190 * time.Millisecond

type cardTearoff struct {
	hwnd    winapi.HWND
	from    winapi.Rect
	to      winapi.Rect
	grab    winapi.Point
	started time.Time
}

// startCardTearoff turns a dragged card into its real front window. The
// expansion destination is captured once: moving the pointer during the
// animation must not move the window or change its animation target.
func (a *App) startCardTearoff(d *display, g *set, st *cardState) {
	if a.cardTearoff != nil || d == nil || g == nil || len(g.windows) == 0 || st.poses == nil {
		return
	}
	h := g.windows[0].HWND
	if !winapi.IsWindow(h) || winapi.IsIconic(h) || winapi.IsHung(h) {
		return
	}
	home := winapi.WindowRect(h)
	if r, ok := a.park.Home(h); ok {
		home = r
		a.park.Unpark(h)
	}
	if home.Empty() {
		return
	}
	p, ok := st.poses[h]
	if !ok || p.w < 1 || p.h < 1 {
		return
	}
	from := winapi.Rect{
		Left: int32(p.x), Top: int32(p.y),
		Right: int32(p.x + p.w), Bottom: int32(p.y + p.h),
	}
	if from.Empty() {
		return
	}
	grab := winapi.Cursor()
	caption := int32(32 * winapi.DpiForWindow(h) / 96)
	to := winapi.Rect{
		Left: grab.X - home.W()/2, Top: grab.Y - caption/2,
		Right: grab.X - home.W()/2 + home.W(),
		Bottom: grab.Y - caption/2 + home.H(),
	}
	// Put the real window exactly over its card before revealing it.
	winapi.SetWindowPos(h, winapi.HwndTop, from.Left, from.Top, from.W(), from.H(),
		winapi.SwpNoActivate|winapi.SwpNoOwnerZOrder|winapi.SwpShowWindow)
	winapi.ShowNoActivate(h)
	a.cardTearoff = &cardTearoff{
		hwnd: h, from: from, to: to, grab: grab, started: time.Now(),
	}
	st.dragging, st.dx, st.dy = false, 0, 0
	st.poses = nil
	a.setCardDrag(nil)
	if a.flights != nil {
		a.flights.endDrag(d, 0)
	}
}

// tickCardTearoff is driven by the strip's existing 16 ms pointer poll.
// It expands to a fixed rectangle, independent of pointer movement. Only
// after the final frame does it synchronously hand the still-held press to
// Windows' native caption mover.
func (a *App) tickCardTearoff(now time.Time) {
	t := a.cardTearoff
	if t == nil {
		return
	}
	if !winapi.IsWindow(t.hwnd) {
		a.cardTearoff = nil
		return
	}
	u := min(float64(now.Sub(t.started))/float64(cardTearoffTime), 1)
	e := 1 - (1-u)*(1-u)*(1-u)
	x := int32(float64(t.from.Left) + float64(t.to.Left-t.from.Left)*e)
	y := int32(float64(t.from.Top) + float64(t.to.Top-t.from.Top)*e)
	w := int32(float64(t.from.W()) + float64(t.to.W()-t.from.W())*e)
	h := int32(float64(t.from.H()) + float64(t.to.H()-t.from.H())*e)
	winapi.SetWindowPos(t.hwnd, winapi.HwndTop, x, y, w, h,
		winapi.SwpNoActivate|winapi.SwpNoOwnerZOrder|winapi.SwpShowWindow)
	if u < 1 {
		return
	}
	// The animation is complete regardless of whether the user released
	// early. In that case leave the expanded window in place, but never
	// start a native move after the release.
	a.cardTearoff = nil
	if winapi.IsLeftButtonDown() {
		winapi.BeginCaptionDrag(t.hwnd, winapi.Cursor())
	}
}
