package captcha

import "testing"

func TestViewportBrowserPoint(t *testing.T) {
	tests := []struct {
		name         string
		v            Viewport
		x, y         int
		wantX, wantY int
		wantOK       bool
	}{
		{name: "top left", v: Viewport{80, 24, 1280, 720}, wantOK: true},
		{name: "scaled point", v: Viewport{80, 24, 1280, 720}, x: 40, y: 12, wantX: 640, wantY: 360, wantOK: true},
		{name: "right edge stays inside", v: Viewport{80, 24, 1280, 720}, x: 79, y: 23, wantX: 1264, wantY: 690, wantOK: true},
		{name: "outside", v: Viewport{80, 24, 1280, 720}, x: 80, y: 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, ok := tt.v.BrowserPoint(tt.x, tt.y)
			if x != tt.wantX || y != tt.wantY || ok != tt.wantOK {
				t.Fatalf("BrowserPoint() = (%d, %d, %t), want (%d, %d, %t)", x, y, ok, tt.wantX, tt.wantY, tt.wantOK)
			}
		})
	}
}
