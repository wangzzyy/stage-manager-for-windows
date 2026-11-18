package app

import "testing"

func TestStaysOnStage(t *testing.T) {
	cases := []struct {
		name         string
		in           bool
		hideInactive bool
		forceDepart  bool
		want         bool
	}{
		{name: "selected app", in: true, hideInactive: true, want: true},
		{name: "selected app explicitly dropped", in: true, hideInactive: true, forceDepart: true},
		{name: "unselected app automatic hide", hideInactive: true},
		{name: "unselected app remains when auto hide disabled", hideInactive: false, want: true},
		{name: "explicit drop hides when auto hide disabled", hideInactive: false, forceDepart: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := staysOnStage(tc.in, tc.hideInactive, tc.forceDepart); got != tc.want {
				t.Fatalf("staysOnStage = %v, want %v", got, tc.want)
			}
			})
		}
		}
	}
}
