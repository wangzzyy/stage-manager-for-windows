package app

import (
	"slices"
	"testing"

	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
	"github.com/egoist/mygo/ui"
)

func TestAroundLeavesTheHole(t *testing.T) {
	area := ui.Rect{W: 100, H: 80}
	got := around(area, ui.Rect{X: 30, Y: 20, W: 40, H: 30})
	want := []ui.Rect{
		{X: 0, Y: 0, W: 100, H: 20},
		{X: 0, Y: 50, W: 100, H: 30},
		{X: 0, Y: 20, W: 30, H: 30},
		{X: 70, Y: 20, W: 30, H: 30},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("around %v, want %v", got, want)
	}
	// A window dragged half off the display leaves three bands.
	got = around(area, ui.Rect{X: -50, Y: 20, W: 100, H: 30})
	want = []ui.Rect{
		{X: 0, Y: 0, W: 100, H: 20},
		{X: 0, Y: 50, W: 100, H: 30},
		{X: 50, Y: 20, W: 50, H: 30},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("around %v, want %v", got, want)
	}
}

func TestStageHomeForMaximizedWindowUsesWorkArea(t *testing.T) {
	frame := winapi.Rect{Left: 8, Top: 0, Right: 1912, Bottom: 1040}
	window := winapi.Rect{Left: 0, Top: -8, Right: 1920, Bottom: 1048}
	parkedHome := winapi.Rect{Left: 1920, Top: -8, Right: 3840, Bottom: 1048}
	work := winapi.Rect{Left: 1920, Top: 0, Right: 3840, Bottom: 1040}

	got := stageHome(frame, window, parkedHome, work, true, true)
	if got != work {
		t.Fatalf("stageHome = %v, want maximized work area %v", got, work)
	}
}

func TestStageHomePreservesRestoredFrameOffset(t *testing.T) {
	frame := winapi.Rect{Left: 12, Top: 20, Right: 812, Bottom: 620}
	window := winapi.Rect{Left: 4, Top: 0, Right: 820, Bottom: 640}
	parkedHome := winapi.Rect{Left: 1000, Top: 200, Right: 1816, Bottom: 840}
	want := winapi.Rect{Left: 1008, Top: 220, Right: 1808, Bottom: 820}

	got := stageHome(frame, window, parkedHome, winapi.Rect{}, true, false)
	if got != want {
		t.Fatalf("stageHome = %v, want %v", got, want)
	}
}

func TestArrivalShadeOnlyCarriesForDWMShadow(t *testing.T) {
	card := cardShade(160)
	if got := arrivalShade(card, false); got != (shade{}) {
		t.Fatalf("arrivalShade without DWM shadow = %+v, want no shadow", got)
	}
	if got := arrivalShade(card, true); got != card {
		t.Fatalf("arrivalShade with DWM shadow = %+v, want %+v", got, card)
	}
}

func TestLerpPoseKeepsSharedPivotInScreenCoordinates(t *testing.T) {
	from := pose{x: 10, y: 20, w: 100, h: 80, originX: 0.25, originY: 0.5, perspective: cardPerspective}
	to := pose{x: 50, y: 60, w: 200, h: 160, originX: 1.5, originY: 1.25, perspective: cardPerspective}
	got := lerpPose(from, to, 0.5)
	wantX := (10 + 100*0.25 + 50 + 200*1.5) / 2
	wantY := (20 + 80*0.5 + 60 + 160*1.25) / 2
	if pivotX := got.x + got.w*float64(got.originX); pivotX != wantX {
		t.Fatalf("lerpPose horizontal pivot = %v, want %v", pivotX, wantX)
	}
	if pivotY := got.y + got.h*float64(got.originY); pivotY != wantY {
		t.Fatalf("lerpPose vertical pivot = %v, want %v", pivotY, wantY)
	}
}
