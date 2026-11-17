package app

import (
	"testing"

	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
	"github.com/egoist/mygo/ui"
)

func TestCardUnderUsesStripScaleAndOrigin(t *testing.T) {
	first := &set{key: "first"}
	second := &set{key: "second"}
	d := &display{
		sets: []*set{first, second},
		layout: stripLayout{cards: []ui.Rect{
			{X: 10, Y: 20, W: 40, H: 50},
			{X: 10, Y: 80, W: 40, H: 50},
		}},
	}
	d.strip = &strip{
		rect:  winapi.Rect{Left: 100, Top: 200},
		scale: 2,
	}

	if got := (&App{}).cardUnder(d, winapi.Point{X: 140, Y: 260}); got != first {
		t.Fatalf("cardUnder() = %v, want first card", got)
	}
	if got := (&App{}).cardUnder(d, winapi.Point{X: 140, Y: 380}); got != second {
		t.Fatalf("cardUnder() = %v, want second card", got)
	}
	if got := (&App{}).cardUnder(d, winapi.Point{X: 199, Y: 260}); got != nil {
		t.Fatalf("cardUnder() outside card = %v, want nil", got)
	}
}

func TestCardUnderWithoutStrip(t *testing.T) {
	if got := (&App{}).cardUnder(&display{}, winapi.Point{}); got != nil {
		t.Fatalf("cardUnder() without strip = %v, want nil", got)
	}
}
