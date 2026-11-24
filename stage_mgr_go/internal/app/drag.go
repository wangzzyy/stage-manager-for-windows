package app

import (
	"slices"
	"time"

	"github.com/e1roy/stage_manager_for_windows/internal/catalog"
	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
)

// dwell is how long a window dragged onto another display rests there
// before the stage there makes way, so that one only passing over, on to
// a third display or back, does not set it flying.
const dwell = 150 * time.Millisecond

// maximizedBeforeMove remembers the last maximized state observed before
// Windows restores a window as its native title-bar drag begins.
var maximizedBeforeMove = map[winapi.HWND]time.Time{}

// drag is a window the user is moving, from the start of the move to its
// end.
type drag struct {
	h    winapi.HWND
	from winapi.Rect // where it was as the move began
	home *display    // the display it was on then
	// at is the display it is over, since since; over the one whose stage
	// made way for it, and stage what was on that stage.
	at, over *display
	since    time.Time
	stage    []string
	dwell         *time.Timer
	wasMaximized bool // Windows restored this maximized window as its native move loop began
	converted    bool // the window has already become a strip card while still being dragged
}

// startDrag follows window h, which the user began to move.
func (a *App) startDrag(h winapi.HWND) {
	if a.drag != nil && a.drag.dwell != nil {
		a.drag.dwell.Stop()
	}
	wasMaximized := winapi.IsZoomed(h)
	if at, ok := maximizedBeforeMove[h]; ok {
		wasMaximized = wasMaximized || time.Since(at) < time.Second
		delete(maximizedBeforeMove, h)
	}
	home := a.displayByHandle(winapi.MonitorFromRect(winapi.FrameBounds(h)))
	if wasMaximized {
		// Suppress DWM's restore transition for the remainder of this native
		// move loop. Do not call Restore: Windows already owns the gesture.
		winapi.SetTransitionsDisabled(h, true)
	}
	a.drag = &drag{h: h, from: winapi.WindowRect(h), home: home, at: home, wasMaximized: wasMaximized}
}

// dragOver makes way for a window dragged onto another display, as macOS
// does: the stage there flies into the strip once the window comes over,
// and back out should the window leave again before it is let go.
func (a *App) dragOver() {
	g := a.drag
	if !a.ready || g == nil || g.converted {
		return
	}
	// By its visible frame, as displayOf goes: a switch must not take the
	// window for one of the stage it leaves, and park it under the pointer.
	// Not by its size: crossing to a display of another DPI resizes it.
	d := a.displayByHandle(winapi.MonitorFromRect(winapi.FrameBounds(g.h)))
	if d != g.at {
		g.at, g.since = d, time.Now()
	}
	// Turn the dragged window into a real card as soon as the pointer enters
	// a visible strip. The ordinary stage switch owns the capture, flight,
	// parking and card creation; mouse-up must not run the drop a second time.
	if target := a.stripUnder(winapi.Cursor()); target != nil {
		if i := slices.IndexFunc(a.windows, func(w catalog.Window) bool { return w.HWND == g.h }); i >= 0 {
			w := a.windows[i]
			key := a.groupKey(w)
			if target == g.home && slices.Contains(a.heading(target), key) {
				origin := winapi.FrameBounds(g.h)
				// Keep the real window visible until the flight layer has drawn
				// its first frame over it. The handoff callback then temporarily cloaks it with DWM
				// before cancelling the native move loop and parking it.
				g.converted = true
				// Start the card's external drag before the native move loop
				// ends. The loop-end event may clear a.drag before the switch
				// has finished arranging the new card.
				a.beginWindowCardDrag(target, key)
				a.sendDraggedToStrip(target, w, slices.DeleteFunc(a.heading(target), func(app string) bool { return app == key }), g.from, origin, g.wasMaximized)
				return
			}
			if target != g.home && a.settings.HideInactive {
				origin := winapi.FrameBounds(g.h)
				stage := g.stage
				if target != g.over {
					stage = a.heading(target)
				}
				// Keep the real window visible until the flight layer has drawn
				// its first frame over it. The handoff callback then temporarily cloaks it with DWM
				// before cancelling the native move loop and parking it.
				g.converted = true
				// Preserve the pointer gesture independently of a.drag: the
				// native move-size-end event can arrive before card creation.
				a.beginWindowCardDrag(target, key)
				a.leave(g.home, w)
				x, y := w.Rect.Left+winapi.FitInto(w.Frame, target.mon.Handle).Left-w.Frame.Left, w.Rect.Top+winapi.FitInto(w.Frame, target.mon.Handle).Top-w.Frame.Top
				home := winapi.Rect{Left: x, Top: y, Right: x + w.Rect.W(), Bottom: y + w.Rect.H()}
				a.sendDraggedToStrip(target, w, slices.DeleteFunc(slices.Clone(stage), func(app string) bool { return app == key }), home, origin, g.wasMaximized)
				return
			}
		}
	}
	if d == g.over {
		return
	}
	if o := g.over; o != nil {
		g.over = nil
		a.switchStage(o, g.stage, g.h, false, false)
	}
	if d == nil || d == g.home || !a.managed(d) || !a.settings.HideInactive {
		return
	}
	if wait := dwell - time.Since(g.since); wait > 0 {
		if g.dwell == nil {
			g.dwell = after(wait, func() {
				g.dwell = nil
				if a.drag == g {
					a.dragOver()
				}
			})
		}
		return
	}
	i := slices.IndexFunc(a.windows, func(w catalog.Window) bool { return w.HWND == g.h })
	if i < 0 {
		return
	}
	w := a.windows[i]
	stage := a.heading(d)
	if slices.Contains(stage, a.groupKey(w)) {
		return
	}
	g.over, g.stage = d, stage
	a.switchStage(d, []string{a.groupKey(w)}, g.h, false, false)
}

// windowMoved puts the app of a window moved to another display on that
// display's stage: the user put it there to use it, so it must not be taken
// for a stray and parked. A window dragged into a strip takes its app off
// the stage instead.
func (a *App) windowMoved(h winapi.HWND) {
	g := a.drag
	moved := g != nil && g.h == h
	if moved {
		// Where it was let go.
		a.dragOver()
	}
	if g != nil && g.dwell != nil {
		g.dwell.Stop()
	}
	a.drag = nil
	if moved {
		delete(maximizedBeforeMove, h)
		if g.wasMaximized {
			winapi.SetTransitionsDisabled(h, false)
		}
	}
	if moved && g.converted {
		// The stage switch already owns this window and its card. The end of
		// the native move only ends the pointer gesture; it is not another drop.
		a.refreshSoon()
		return
	}
	if !moved {
		// A move whose start went unseen.
		g = &drag{}
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
			stage := g.stage
			if d != g.over {
				stage = a.heading(d)
			}
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

// sendDraggedToStrip prepares a card handoff. The real window stays visible
// until the flight layer has drawn its first frame over it; only then is the
// native move loop cancelled and the real window parked.
func (a *App) sendDraggedToStrip(d *display, w catalog.Window, next []string, home, origin winapi.Rect, wasMaximized bool) {
	debugf("%q prepared for direct strip-card handoff", w.Title)
	// The overlay must have a real, drawable frame before the native window
	// is hidden. In particular, Invalidate alone is only a paint request.
	handoff := func() {
		// The overlay has built its first frame. Cloak the real HWND before
		// ending the native move loop: WM_CANCELMODE may make Windows restore
		// the loop's starting rectangle, and merely waiting for the loop to
		// unwind cannot prevent that rectangle from being presented.
		// DWMWA_CLOAK can report success without actually suppressing the
		// dragged HWND on some app/window combinations. Hide the real HWND
		// explicitly for the short handoff, and ignore its corresponding
		// WinEvent hide/show so it is not mistaken for an app closing.
		if a.handoffHidden == nil {
			a.handoffHidden = make(map[winapi.HWND]time.Time)
		}
		a.handoffHidden[w.HWND] = time.Now().Add(2 * time.Second)
		winapi.Hide(w.HWND)
		if winapi.IsWindowVisible(w.HWND) {
			winapi.SetWindowPos(w.HWND, 0, 0, 0, 0, 0,
				winapi.SwpHideWindow|winapi.SwpNoMove|winapi.SwpNoSize|winapi.SwpNoZOrder|winapi.SwpNoActivate)
		}
		if winapi.IsWindowVisible(w.HWND) {
			winapi.ShowNoActivate(w.HWND)
			debugf("drag into card: could not hide real window %v; aborting handoff", w.HWND)
			return
		}
		winapi.DwmFlush()
		parkFrom := winapi.WindowRect(w.HWND)
		if !winapi.CancelMoveSize(w.HWND) {
			winapi.ShowNoActivate(w.HWND)
			winapi.DwmFlush()
			debugf("drag into card: could not end native move loop for %v; restored real window", w.HWND)
			return
		}
		parked := false
		if a.settings.HideInactive || wasMaximized {
			parked = a.park.ParkImmediateFrom(w.HWND, parkFrom, wasMaximized)
			if parked {
				a.park.Rehome(w.HWND, home)
			}
		}
		if !parked {
			// If parking is disabled or failed, put the window at its intended
			// home before making it visible again; never expose the stale
			// starting rectangle restored by the native move loop.
			winapi.SetWindowPos(w.HWND, 0, home.Left, home.Top, 0, 0,
				winapi.SwpNoSize|winapi.SwpNoZOrder|winapi.SwpNoActivate)
		}
		// Parked windows remain WS_VISIBLE but sit beyond the desktop. This
		// keeps enumeration and later restoration working as usual.
		winapi.ShowNoActivate(w.HWND)
		winapi.DwmFlush()
		if a.settings.HideInactive && !parked {
			debugf("drag into card: could not park real window %v; restored it at its home", w.HWND)
		}
	}
	// Switch the model first so refresh lays out the destination card. The
	// window's last stage still is already available in the common case; use
	// it immediately rather than waiting for a fresh capture before showing
	// the handoff animation.
	from := poseOf(origin)
	d.model.Switch(next, time.Now())
	// Suppress the destination's real strip card before refresh can draw it.
	// Previously refresh presented the resting card before the async capture
	// completed and dragIntoCard enabled externalLayer, causing a visible
	// final-card flash before the window-to-card animation began.
	key := a.groupKey(w)
	card := d.card(key)
	card.externalLayer = true
	card.externalPresented = false
	card.externalAnimating = false
	card.externalFrom = pose{}
	card.externalStarted = time.Time{}
	a.windows = a.cat.Windows()
	a.refresh()

	startFlight := func() bool {
		return a.flights != nil && a.flights.dragIntoCard(d, w, from, handoff)
	}
	// Capture only the dragged HWND. A desktop BitBlt is not a window
	// capture: it includes whichever windows happen to overlap its rectangle.
	// Use the window's PrintWindow capture path and let its timeout keep a
	// hung application from holding up the handoff.
	a.media.capture([]winapi.HWND{w.HWND}, true, false, 300*time.Millisecond, func() {
		if !startFlight() {
			// No overlay will take over; restore the real card after a failed
			// capture so externalLayer cannot leave it permanently invisible.
			card.externalLayer = false
			card.externalPresented = false
			debugf("drag into card: capture produced no drawable animation; restored real card")
		}
		a.redraw()
	})
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
	x, y := w.Rect.Left+f.Left-w.Frame.Left, w.Rect.Top+f.Top-w.Frame.Top
	winapi.SetWindowPos(w.HWND, 0, x, y, 0, 0, winapi.SwpNoSize|winapi.SwpNoZOrder|winapi.SwpNoActivate)
	// Record the new home before parking so the card returns to this display,
	// and make the handover use synchronous parking to prevent drag-loop races.
	a.dropHomes[w.HWND] = winapi.Rect{Left: x, Top: y, Right: x + w.Rect.W(), Bottom: y + w.Rect.H()}
	next := slices.DeleteFunc(slices.Clone(stage), func(app string) bool { return app == a.groupKey(w) })
	a.switchStage(d, next, 0, false, false)
}
