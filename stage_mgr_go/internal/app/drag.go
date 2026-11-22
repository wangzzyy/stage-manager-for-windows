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

// drag is a window the user is moving, from the start of the move to its
// end.
type drag struct {
	h    winapi.HWND
	from winapi.Rect // where it was as the move began
	home *display    // the display it was on then
	// at is the display it is over, since since; over the one whose stage
	// made way for it, and stage what was on that stage.
	at, over  *display
	since     time.Time
	stage     []string
	dwell     *time.Timer
	converted bool // the window has already become a strip card while still being dragged
}

// startDrag follows window h, which the user began to move.
func (a *App) startDrag(h winapi.HWND) {
	if a.drag != nil && a.drag.dwell != nil {
		a.drag.dwell.Stop()
	}
	home := a.displayByHandle(winapi.MonitorFromRect(winapi.FrameBounds(h)))
	a.drag = &drag{h: h, from: winapi.WindowRect(h), home: home, at: home}
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
				// Let sendDraggedToStrip capture and present the first flight
				// frame before it hides the window and cancels the native move.
				g.converted = true
				// Start the card's external drag before the native move loop
				// ends. The loop-end event may clear a.drag before the switch
				// has finished arranging the new card.
				a.beginWindowCardDrag(target, key)
				a.sendDraggedToStrip(target, w, slices.DeleteFunc(a.heading(target), func(app string) bool { return app == key }), g.from, origin)
				return
			}
			if target != g.home && a.settings.HideInactive {
				origin := winapi.FrameBounds(g.h)
				stage := g.stage
				if target != g.over {
					stage = a.heading(target)
				}
				// Let sendDraggedToStrip capture and present the first flight
				// frame before it hides the window and cancels the native move.
				g.converted = true
				// Preserve the pointer gesture independently of a.drag: the
				// native move-size-end event can arrive before card creation.
				a.beginWindowCardDrag(target, key)
				a.leave(g.home, w)
				x, y := w.Rect.Left+winapi.FitInto(w.Frame, target.mon.Handle).Left-w.Frame.Left, w.Rect.Top+winapi.FitInto(w.Frame, target.mon.Handle).Top-w.Frame.Top
				home := winapi.Rect{Left: x, Top: y, Right: x + w.Rect.W(), Bottom: y + w.Rect.H()}
				a.sendDraggedToStrip(target, w, slices.DeleteFunc(slices.Clone(stage), func(app string) bool { return app == key }), home, origin)
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

// sendDraggedToStrip converts a native window drag directly into a card.
// The real window is parked immediately; its captured image then flies from
// the live drag position into the card's measured layout position.
func (a *App) sendDraggedToStrip(d *display, w catalog.Window, next []string, home, origin winapi.Rect) {
	// Capture returns an image whose rectangle is local to the bitmap
	// (normally starting at 0,0), not its screen position. Keep the screen
	// frame sampled by dragOver as the flight origin.
	from := poseOf(origin)

	// Create the card before drawing its landing animation. Refreshing while
	// the native move loop is still active is safe: dragging keeps the old
	// stage from pruning the window during this handoff.
	d.model.Switch(next, time.Now())
	a.windows = a.cat.Windows()
	a.refresh()

	// Capture synchronously while the real window is still at the sampled
	// drag position. The native move loop must not be cancelled before this:
	// WM_CANCELMODE may restore the window to its pre-drag position.
	height := cardStillHeight
	if a.media.full {
		height = stillHeight(w.HWND, a.media.light)
	}
	img := winapi.Capture(w.HWND, height)
	if img != nil && img.Rect.Dx() > 0 && img.Rect.Dy() > 0 {
		card := img
		if img.Rect.Dy() > cardStillHeight {
			card = shrink(img, cardStillHeight)
		}
		a.media.take([]shot{{h: w.HWND, img: img, card: card}})
		// PrintWindow and shrinking can take long enough for a fast native
		// drag to move the HWND well beyond the rectangle sampled by dragOver.
		// The captured pixels are position-independent, so anchor the flight
		// to the latest frame only after all capture preparation is complete.
		if latest := winapi.FrameBounds(w.HWND); !latest.Empty() {
			from = poseOf(latest)
		}
	}

	// Finish the native drag only after the landing layer has built its
	// first frame and DWM has composed it. PaintNow only processes WM_PAINT;
	// it cannot prove that this newly shown window has produced a visible frame.
	finish := func() {
		// Cloak the HWND in DWM before ending the native modal move loop.
		// WM_CANCELMODE can restore the drag-start rectangle; without a
		// composition barrier that rollback may be visible for one frame
		// before the synchronous parking move takes effect. DWM cloaking
		// leaves the HWND visible to Win32 (unlike SW_HIDE), so its show and
		// thumbnail state are not toggled during the handoff.
		cloaked := winapi.SetCloaked(w.HWND, true)
		if cloaked {
			// Do not cancel the move loop until DWM has committed the cloak.
			winapi.DwmFlush()
		} else {
			debugf("drag conversion: could not cloak %x before cancelling move-size", w.HWND)
		}
		if !winapi.CancelMoveSize(w.HWND) {
			if cloaked {
				winapi.SetCloaked(w.HWND, false)
				winapi.DwmFlush()
			}
			debugf("drag conversion: could not cancel move-size for %x", w.HWND)
			return
		}
		if a.settings.HideInactive {
			parked := a.park.IsParked(w.HWND)
			if !parked {
				parked = a.park.ParkImmediateAt(w.HWND, home)
			}
			if parked {
				a.park.Rehome(w.HWND, home)
			} else if !a.park.IsParked(w.HWND) {
				winapi.SetWindowPos(w.HWND, 0, home.Left, home.Top, 0, 0,
					winapi.SwpNoSize|winapi.SwpNoZOrder|winapi.SwpNoActivate)
				debugf("drag conversion: could not park %x; leaving it visible", w.HWND)
			}
		} else {
			winapi.SetWindowPos(w.HWND, 0, home.Left, home.Top, 0, 0,
				winapi.SwpNoSize|winapi.SwpNoZOrder|winapi.SwpNoActivate)
		}
		if cloaked {
			// Commit the new off-screen/home position before allowing DWM to
			// compose the window again. A parked window stays cloaked only for
			// this short handoff; it is not left cloaked while in the strip.
			winapi.DwmFlush()
			if !winapi.SetCloaked(w.HWND, false) {
				debugf("drag conversion: could not uncloak %x after parking", w.HWND)
			} else {
				winapi.DwmFlush()
			}
		}
		a.redraw()
	}
	if img != nil && img.Rect.Dx() > 0 && img.Rect.Dy() > 0 && a.flights != nil {
		a.flights.dragIntoCard(d, w, from)
		if l := a.flights.layers[d.mon.Device]; l != nil && l.landing != nil {
			// view() invokes this only after drawing the landing image. DwmFlush
			// then waits for that frame's composition before the handoff.
			l.presented = func() {
				winapi.DwmFlush()
				finish()
			}
			return
		}
	}
	// Without a usable captured image there is no overlay frame to await.
	finish()
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
