package app

import (
	"time"

	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
)

const cardTearoffTime = 190 * time.Millisecond

type cardTearoff struct {
	hwnd    winapi.HWND
	from    winapi.Rect
	toSize  winapi.Point
	started time.Time
}

// startCardTearoff turns a dragged card into its real front window. The
// existing strip poll drives the short expansion; no additional ticker or
// animation window is created.
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
	// Put the real window exactly over its card still before revealing it.
	winapi.SetWindowPos(h, winapi.HwndTop, from.Left, from.Top, from.W(), from.H(),
		winapi.SwpNoActivate|winapi.SwpNoOwnerZOrder|winapi.SwpShowWindow)
	winapi.ShowNoActivate(h)
	a.cardTearoff = &cardTearoff{
		hwnd: h, from: from,
		toSize: winapi.Point{X: home.W(), Y: home.H()},
		started: time.Now(),
	}
	st.dragging, st.dx, st.dy = false, 0, 0
	st.poses = nil
	a.setCardDrag(nil)
	if a.flights != nil {
		a.flights.endDrag(d, 0)
	}
}

// tickCardTearoff is driven by the strip's existing 16 ms pointer poll.
// The pointer remains the title-bar grab point throughout the animation;
// at its end Windows takes over the still-held mouse button natively.
func (a *App) tickCardTearoff(now time.Time) {
	t := a.cardTearoff
	if t == nil {
		return
	}
	if !winapi.IsWindow(t.hwnd) {
		a.cardTearoff = nil
		return
	}
	p := winapi.Cursor()
	w, h := t.toSize.X, t.toSize.Y
	if w <= 0 || h <= 0 {
		a.cardTearoff = nil
		return
	}
	// Keep the pointer at the center of the title bar. 32 px at 96 DPI
	// is a conservative caption-height estimate.
	caption := int32(32 * winapi.DpiForWindow(t.hwnd) / 96)
	target := winapi.Rect{
		Left: p.X - w/2, Top: p.Y - caption/2,
		Right: p.X - w/2 + w, Bottom: p.Y - caption/2 + h,
	}
	u := min(float64(now.Sub(t.started))/float64(cardTearoffTime), 1)
	e := 1 - (1-u)*(1-u)*(1-u)
	x := int32(float64(t.from.Left) + float64(target.Left-t.from.Left)*e)
	y := int32(float64(t.from.Top) + float64(target.Top-t.from.Top)*e)
	cw := int32(float64(t.from.W()) + float64(w-t.from.W())*e)
	ch := int32(float64(t.from.H()) + float64(h-t.from.H())*e)
	winapi.SetWindowPos(t.hwnd, winapi.HwndTop, x, y, cw, ch,
		winapi.SwpNoActivate|winapi.SwpNoOwnerZOrder|winapi.SwpShowWindow)
	if u < 1 {
		return
	}
	// Re-read the pointer on the final frame so the native move starts
	// without a positional jump if the user kept moving during the flight.
	p = winapi.Cursor()
	x, y = p.X-w/2, p.Y-caption/2
	winapi.SetWindowPos(t.hwnd, winapi.HwndTop, x, y, w, h,
		winapi.SwpNoActivate|winapi.SwpNoOwnerZOrder|winapi.SwpShowWindow)
	a.cardTearoff = nil
	winapi.BeginCaptionDrag(t.hwnd, p)
}
