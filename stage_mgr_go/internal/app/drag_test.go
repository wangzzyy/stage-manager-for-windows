package app

import (
	"testing"

	"github.com/e1roy/stage_manager_for_windows/internal/winapi"
)

func TestDropIntoStrip(t *testing.T) {
	home, other := &display{}, &display{}
	from := winapi.Rect{Left: 10, Top: 20, Right: 810, Bottom: 620}
	g := &drag{h: 42, from: from, home: home}
	cases := []struct {
		name   string
		g     *drag
		to    *display
		h     winapi.HWND
		stage bool
		r     winapi.Rect
		want  bool
	}{
		{name: "same display staged unchanged size", g: g, to: home, h: 42, stage: true, r: from, want: true},
		{name: "not staged", g: g, to: home, h: 42, stage: false, r: from},
		{name: "different display", g: g, to: other, h: 42, stage: true, r: from},
		{name: "different window", g: g, to: home, h: 43, stage: true, r: from},
		{name: "resized window", g: g, to: home, h: 42, stage: true, r: winapi.Rect{Left: 10, Top: 20, Right: 900, Bottom: 620}},
		{name: "missing drag", g: nil, to: home, h: 42, stage: true, r: from},
		{name: "missing display", g: g, to: nil, h: 42, stage: true, r: from},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dropIntoStrip(tc.g, tc.to, tc.h, tc.stage, tc.r); got != tc.want {
				t.Fatalf("dropIntoStrip = %v, want %v", got, tc.want)
			}
		})
	}
}
}
