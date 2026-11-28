package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCardThumbnailAreaTracksFlatCardTransforms(t *testing.T) {
	box := ui.Rect{X: 100, Y: 200, W: 80, H: 120}
	window := ui.Rect{X: 10, Y: 20, W: 40, H: 60}
	got := cardThumbnailArea(box, window, 0, 0.5, 1, 0, 5, -3, 1)
	want := ui.Rect{X: 115, Y: 217, W: 40, H: 60}
	if got != want {
		t.Fatalf("flat card area = %+v, want %+v", got, want)
	}

	got = cardThumbnailArea(box, window, 0, 0.5, 1.1, 6, 5, -3, 1.2)
	if got.W != 52.8 || got.H != 79.2 || got.X >= want.X || got.Y >= want.Y {
		t.Fatalf("lifted and scaled area = %+v", got)
	}
}
