package app

import "testing"

func TestEdgeWakeAllowed(t *testing.T) {
	const top, bottom, deadZone = 100, 1100, 80
	tests := []struct {
		name string
		y    int32
		want bool
	}{
		{name: "top boundary excluded", y: top + deadZone - 1, want: false},
		{name: "top boundary included", y: top + deadZone, want: true},
		{name: "middle included", y: 600, want: true},
		{name: "bottom boundary excluded", y: bottom - deadZone, want: false},
		{name: "bottom interior included", y: bottom - deadZone - 1, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := edgeWakeAllowed(tt.y, top, bottom, deadZone); got != tt.want {
				t.Fatalf("edgeWakeAllowed(%d) = %v, want %v", tt.y, got, tt.want)
			}
		})
	}
}
