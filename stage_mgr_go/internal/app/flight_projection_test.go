package app

import "testing"

func TestLerpPoseCarriesTiltAndRotationOrigin(t *testing.T) {
	from := pose{x: 10, y: 20, w: 100, h: 80, tilt: 0, originY: 0.5}
	to := pose{x: 30, y: 40, w: 60, h: 40, tilt: 24, originY: 1.25}
	got := lerpPose(from, to, 0.5)
	if got.tilt != 12 || got.originY != 0.875 {
		t.Fatalf("rotation pose did not interpolate with flight: %+v", got)
	}
}
