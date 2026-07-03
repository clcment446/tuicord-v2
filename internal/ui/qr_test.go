package ui

import (
	"testing"

	"awesomeProject/internal/tui/screen"
)

func TestHalfBlockGlyphs(t *testing.T) {
	on := screen.Style{Fg: screen.RGB(0, 0, 0), Bg: screen.RGB(255, 255, 255)}
	tests := []struct {
		upper, lower bool
		want         string
	}{
		{true, true, " "},   // both dark → solid background
		{false, false, " "}, // both light → solid background
		{true, false, "▀"},  // dark top
		{false, true, "▄"},  // dark bottom
	}
	for _, tt := range tests {
		if got := halfBlock(tt.upper, tt.lower, on).Content; got != tt.want {
			t.Errorf("halfBlock(%v,%v) = %q, want %q", tt.upper, tt.lower, got, tt.want)
		}
	}
}

func TestDrawQRRowsPacked(t *testing.T) {
	// 4 module rows should pack into 2 character rows (two modules per cell).
	matrix := [][]bool{
		{true, false},
		{false, true},
		{true, true},
		{false, false},
	}
	buf := screen.NewBuffer(2, 4)
	rows, ok := drawQR(buf.Clip(buf.Bounds()), matrix)
	if !ok {
		t.Fatal("drawQR reported matrix did not fit")
	}
	if rows != 2 {
		t.Errorf("drawQR packed into %d rows, want 2", rows)
	}
	// Top-left cell: upper=dark, lower=light → "▀".
	if got := buf.Cell(0, 0).Content; got != "▀" {
		t.Errorf("cell(0,0) = %q, want ▀", got)
	}
}

func TestDrawQRRejectsClipping(t *testing.T) {
	matrix := [][]bool{
		{true, false, true},
		{false, true, false},
	}
	buf := screen.NewBuffer(2, 1)
	if _, ok := drawQR(buf.Clip(buf.Bounds()), matrix); ok {
		t.Fatal("drawQR reported fit for a QR wider than the region")
	}
}

func TestQRPanelDrawShowsStatus(t *testing.T) {
	p := &QRPanel{styles: Styles{}, status: "Connecting…"}
	buf := screen.NewBuffer(20, 3)
	p.Draw(buf.Clip(buf.Bounds()))
	if rowText(buf, 1) != "Connecting…" {
		t.Errorf("status row = %q, want Connecting…", rowText(buf, 1))
	}
}
