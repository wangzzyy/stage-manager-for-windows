package app

import "testing"

func TestLerpPoseCarriesCardProjection(t *testing.T) {
	from := pose{x: 10, y: 20, w: 100, h: 80, tilt: 0, originY: 0.5, perspective: cardPerspective}
	to := pose{x: 30, y: 40, w: 60, h: 40, tilt: 24, originY: 1.25, perspective: 900}
	got := lerpPose(from, to, 0.5)
	wantPerspective := float32(1 / ((1 / float64(from.perspective) + 1 / float64(to.perspective)) / 2))
	if got.tilt != 12 || got.originY != 0.875 || abs32(got.perspective-wantPerspective) > 0.01 {
		t.Fatalf("projection did not interpolate with flight: %+v; want perspective %v", got, wantPerspective)
	}
}

func TestOverviewProjectionAnimatesToFlatAndBack(t *testing.T) {
	card := pose{tilt: 18, originY: 1.2, perspective: cardPerspective}
	flat := pose{tilt: 0, originY: 0.5, perspective: overviewFlatPerspective}

	opening := lerpPose(card, flat, 1)
	if opening.tilt != flat.tilt || opening.originY != flat.originY || opening.perspective != flat.perspective {
		t.Fatalf("opening animation did not land flat: %+v", opening)
	}

	closing := lerpPose(flat, card, 1)
	if closing.tilt != card.tilt || closing.originY != card.originY || closing.perspective != card.perspective {
		t.Fatalf("closing animation did not land on card projection: %+v", closing)
	}
}

func TestPerspectiveInterpolationChangesStrengthSmoothly(t *testing.T) {
	flat := float32(overviewFlatPerspective)
	card := cardPerspective
	quarter := lerpPerspective(flat, card, 0.25)
	half := lerpPerspective(flat, card, 0.5)
	threeQuarter := lerpPerspective(flat, card, 0.75)
	if !(quarter > half && half > threeQuarter && threeQuarter > card) {
		t.Fatalf("perspective distance should decrease smoothly toward the card: %v, %v, %v, %v", quarter, half, threeQuarter, card)
	}
	if got := lerpPerspective(flat, card, 1); got != card {
		t.Fatalf("perspective endpoint = %v, want %v", got, card)
	}
	if got := lerpPerspective(flat, card, 0); got != flat {
		t.Fatalf("flat endpoint = %v, want %v", got, flat)
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
