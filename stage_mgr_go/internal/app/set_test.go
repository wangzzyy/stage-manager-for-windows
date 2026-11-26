package app

import (
	"slices"
	"testing"

	"github.com/e1roy/stage_manager_for_windows/internal/catalog"
)

// A merged card follows the model's app order even when hidden windows'
// cached Z values disagree with it. Each app's own windows remain grouped
// with its front window first.
func TestSetPreservesModelOrderForMergedApps(t *testing.T) {
	w := func(z int, app string) catalog.Window { return catalog.Window{App: app, Z: z} }
	a := &group{app: "a", z: 0, windows: []catalog.Window{w(0, "a"), w(1, "a"), w(3, "a")}}
	b := &group{app: "b", z: 1, windows: []catalog.Window{w(2, "b")}}

	// b is the dragged/source card and must precede a, the target, even
	// though the target's cached Z value is lower.
	s := newSet([]*group{b, a})
	var got []string
	for _, x := range s.windows {
		got = append(got, x.App)
	}
	if want := []string{"b", "a", "a", "a"}; !slices.Equal(got, want) {
		t.Fatalf("app order %v, want %v", got, want)
	}
	if !slices.ContainsFunc(s.stacked(), func(x catalog.Window) bool { return x.App == "b" }) {
		t.Fatalf("source app b does not show in the card: %v", s.stacked())
	}
}
