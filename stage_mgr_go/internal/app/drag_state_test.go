package app

import "testing"

func TestDragTargetTransitions(t *testing.T) {
	a, b := &display{}, &display{}
	g := &drag{phase: dragTracking}

	if !g.setTarget(a) || g.phase != dragOverStrip || g.target != a {
		t.Fatal("entering a strip must start one preview")
	}
	if g.setTarget(a) {
		t.Fatal("remaining over the same strip must not restart the preview")
	}
	if !g.setTarget(b) || g.phase != dragOverStrip || g.target != b {
		t.Fatal("changing strips must retarget the preview")
	}
	if !g.setTarget(nil) || g.phase != dragTracking || g.target != nil {
		t.Fatal("leaving strips must return to tracking")
	}
	if g.setTarget(nil) {
		t.Fatal("remaining outside strips must not restart the transition")
	}
}

func TestDragStartDoesNotCommitStage(t *testing.T) {
	g := &drag{phase: dragTracking}
	before := g.phase
	g.setTarget(&display{})
	if before != dragTracking || g.phase != dragOverStrip {
		t.Fatal("target preview should be represented separately from stage ownership")
	}
}
