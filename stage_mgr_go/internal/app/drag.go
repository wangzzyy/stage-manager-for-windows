package app

import (
	"slices"
	"time"

	"github.com/e1roy/stage_manager_for_windows/internal/catalog"
	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
)

// dragPhase describes the preview lifecycle. Stage ownership is deliberately
// untouched while the pointer is moving; only windowMoved commits a drop.
type dragPhase uint8

const (
	dragTracking dragPhase = iota
	dragOverStrip
	dragCommitted
	dragCancelled
)

// drag is a window the user is moving, from the start of the move to its end.
type drag struct {
	h      winapi.HWND
	from   winapi.Rect // where it was as the move began
	home   *display    // the display it was on then
	at     *display    // display currently under the window
	target *display    // strip currently previewed, if any
	phase  dragPhase
}

// setTarget changes the preview destination without changing either display's
// stage. Returning false means the pointer stayed in the same preview state.
func (g *drag) setTarget(d *display) bool {
	if g.target == d {
		return false
	}
	g.target = d
	if d == nil {
		g.phase = dragTracking
	} else {
		g.phase = dragOverStrip
	}
	return true
}

// startDrag follows window h, which the user began to move. This is a
// snapshot only: it must not park the window or switch any stage.
func (a *App) startDrag(h winapi.HWND) {
	home := a.displayByHandle(winapi.MonitorFromRect(winapi.FrameBounds(h)))
	a.drag = &drag{
		h: h, from: winapi.WindowRect(h), home: home, at: home,
		phase: dragTracking,
	}
}

// dragOver tracks the current monitor and strip target. It is called for
// every native move event, so it must be idempotent and must not start,
// reverse, or commit a stage transition. The window itself remains live
// throughout the preview; leaving a strip therefore resumes from its exact
// current native rectangle rather than replaying from the drag origin.
func (a *App) dragOver() {
	g := a.drag
	if !a.ready || g == nil {
		return
	}
	g.at = a.displayByHandle(winapi.MonitorFromRect(winapi.FrameBounds(g.h)))
	g.setTarget(a.stripUnder(winapi.Cursor()))
}

// windowMoved commits the final destination only after Windows ends its
// move/size loop. Until this point dragOver has only tracked preview state.
func (a *App) windowMoved(h winapi.HWND) {
	g := a.drag
	moved := g != nil && g.h == h
	if moved {
		a.dragOver()
		g.phase = dragCommitted
	}
	a.drag = nil
	if !moved {
		// A move whose start went unseen.
		g = &drag{phase: dragCancelled}
	}
	if !a.ready || a.park.IsParked(h) {
		return
	}
	a.windows = a.cat.Windows()
	i := slices.IndexFunc(a.windows, func(w catalog.Window) bool { return w.HWND == h })
	if i < 0 {
		return
	}
	w := a.windows[i]
	if d := a.stripUnder(winapi.Cursor()); moved && d != nil {
		switch {
		case d != g.home && a.settings.HideInactive:
			stage := a.heading(d)
			a.leave(g.home, w)
			a.sendAcross(d, w, stage)
			return
		case d == g.home && slices.Contains(a.heading(d), a.groupKey(w)):
			// A resize that ends over the strip is no drag into it.
			if r := winapi.WindowRect(h); r.W() == g.from.W() && r.H() == g.from.H() {
				a.sendToStrip(d, w, g.from)
				return
			}
		}
	}
	d := a.displayOf(w)
	if d != g.home {
		a.leave(g.home, w)
	}
	// A stage that did not make way during the drag, as one whose windows
	// are never hidden or one let go of at once, takes the window now.
	if d != nil && a.managed(d) && !slices.Contains(a.heading(d), a.groupKey(w)) {
		a.switchStage(d, []string{a.groupKey(w)}, w.HWND, false, false)
	}
}

// leave lets display d go of the app of window w, just moved off it, at
// once rather than once its stage settles, unless a window of the app
// stays there: should the stage be left empty, the one before comes back.
func (a *App) leave(d *display, w catalog.Window) {
	key := a.groupKey(w)
	if d == nil || !d.model.OnStage(key) || slices.ContainsFunc(a.windows, func(o catalog.Window) bool {
		return a.groupKey(o) == key && o.HWND != w.HWND && a.displayOf(o) == d
	}) {
		return
	}
	d.model.Leave(key)
	if d.model.Empty() {
		d.refill = true
	}
}

// followMoved puts the app of the window in front on the stage of its
// display, should it have moved there without a drag, as with
// Win+Shift+Arrow or by itself: it takes the stage as a dropped window
// does, rather than be parked for a stray once it loses the focus. It
// reports whether it started a switch.
func (a *App) followMoved() bool {
	if !a.settings.HideInactive || a.drag != nil || a.fg.h != 0 || a.fg.timer != nil {
		return false
	}
	fg := catalog.RootOwner(winapi.Foreground())
	i := slices.IndexFunc(a.windows, func(w catalog.Window) bool { return w.HWND == fg })
	if i < 0 || a.park.IsParked(fg) {
		return false
	}
	w := a.windows[i]
	d := a.displayOf(w)
	if d == nil || !a.managed(d) || d.model.OnStage(a.groupKey(w)) || d.model.Settling(time.Now()) {
		return false
	}
	debugf("%q moved to %s", w.Title, d.mon.Device)
	a.switchStage(d, []string{a.groupKey(w)}, w.HWND, false, false)
	return true
}

// stripUnder returns the display whose strip shows under a point.
func (a *App) stripUnder(p winapi.Point) *display {
	d := a.displayByHandle(winapi.MonitorFromPoint(p))
	if d == nil || !a.managed(d) || d.strip == nil || !d.strip.shown || !d.strip.rect.Contains(p) {
		return nil
	}
	return d
}

// sendToStrip takes the app of a window dragged into a display's strip off
// the stage, into a card of its own; the others stay. The window goes back
// to where the drag began, for when its app returns.
func (a *App) sendToStrip(d *display, w catalog.Window, from winapi.Rect) {
	debugf("%q dragged into the strip", w.Title)
	next := slices.DeleteFunc(a.heading(d), func(app string) bool { return app == a.groupKey(w) })
	if a.settings.HideInactive {
		a.dropHomes[w.HWND] = from
	} else {
		// Nothing is parked: the window goes back at once.
		winapi.SetWindowPos(w.HWND, 0, from.Left, from.Top, 0, 0,
			winapi.SwpNoSize|winapi.SwpNoZOrder|winapi.SwpNoActivate)
	}
	a.switchStage(d, next, 0, true, false)
}

// sendAcross puts the app of a window dragged into the strip of another
// display into a card there, and the stage there back as it was before
// the window came over. The window moves onto that display, clear of the
// strip, for when its app comes out of the card.
func (a *App) sendAcross(d *display, w catalog.Window, stage []string) {
	debugf("%q dragged into the strip of %s", w.Title, d.mon.Device)
	f := winapi.FitInto(w.Frame, d.mon.Handle)
	winapi.SetWindowPos(w.HWND, 0, w.Rect.Left+f.Left-w.Frame.Left, w.Rect.Top+f.Top-w.Frame.Top, 0, 0,
		winapi.SwpNoSize|winapi.SwpNoZOrder|winapi.SwpNoActivate)
	next := slices.DeleteFunc(slices.Clone(stage), func(app string) bool { return app == a.groupKey(w) })
	a.switchStage(d, next, 0, false, false)
}
