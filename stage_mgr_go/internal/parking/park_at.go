//go:build windows

package parking

import (
	"github.com/e1roy/stage_manager_for_windows/internal/catalog"
	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
)

// ParkImmediateAt parks a window without first moving it to its requested
// home position. This is used when ending a native drag: WM_CANCELMODE can
// restore the window to its drag-start position, and moving it to home before
// parking would expose that intermediate position for a composed frame.
// The requested home is recorded in the parking journal while the window
// moves directly from its current position to its off-screen parking spot.
func (s *Service) ParkImmediateAt(h winapi.HWND, home winapi.Rect) bool {
	if s.IsParked(h) {
		return false
	}
	from := winapi.WindowRect(h)
	home = onDesktop(home)
	placeAt := func(winapi.Rect) (winapi.Rect, winapi.Point) {
		return home, spotFor(home)
	}
	if !s.parkOneUsing(h, 0, placeAt, moveImmediate) {
		return false
	}

	// Move owned dialogs by the same displacement as their owner, preserving
	// their relative positions and recording their corresponding homes.
	root := s.parked[h]
	dx, dy := root.Spot.X-from.Left, root.Spot.Y-from.Top
	shiftX, shiftY := root.Home.Left-from.Left, root.Home.Top-from.Top
	along := func(r winapi.Rect) (winapi.Rect, winapi.Point) {
		if at := r.Offset(dx, dy); !winapi.OnAnyMonitor(at) {
			return r.Offset(shiftX, shiftY), winapi.Point{X: at.Left, Y: at.Top}
		}
		return alone(r)
	}
	for _, owned := range catalog.OwnedWindows(h) {
		if !s.IsParked(owned) {
			s.parkOneUsing(owned, h, along, moveImmediate)
		}
	}
	s.save()
	return true
}
