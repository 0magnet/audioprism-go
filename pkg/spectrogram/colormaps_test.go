package spectrogram

import (
	"image/color"
	"testing"
)

// The tables are data, and data can be transcribed wrongly without anything
// failing to compile. These are the published endpoints of each map, which is
// the cheapest way to catch a table that was generated from the wrong source or
// scaled the wrong way.
func TestColormapEndpointsMatchThePublishedValues(t *testing.T) {
	cases := []struct {
		name      string
		fn        func(float64) color.Color
		low, high [3]uint8
	}{
		// viridis famously runs #440154 to #FDE725.
		{"viridis", ValueToPixelViridis, [3]uint8{68, 1, 84}, [3]uint8{253, 231, 37}},
		{"magma", ValueToPixelMagma, [3]uint8{0, 0, 4}, [3]uint8{252, 253, 191}},
		{"turbo", ValueToPixelTurbo, [3]uint8{48, 18, 59}, [3]uint8{122, 4, 3}},
	}
	for _, c := range cases {
		for _, tc := range []struct {
			at   float64
			want [3]uint8
		}{{0, c.low}, {1, c.high}} {
			// As color.RGBA rather than through RGBA(), whose uint32s would
			// need narrowing back to uint8 and trip the overflow check.
			rgba, ok := c.fn(tc.at).(color.RGBA)
			if !ok {
				t.Fatalf("%s returned %T, want color.RGBA", c.name, c.fn(tc.at))
			}
			if got := [3]uint8{rgba.R, rgba.G, rgba.B}; got != tc.want {
				t.Errorf("%s at %v = %v, want %v", c.name, tc.at, got, tc.want)
			}
			if rgba.A != 255 {
				t.Errorf("%s at %v is not opaque", c.name, tc.at)
			}
		}
	}
}

// Out-of-range input has to clamp rather than index past the end: a magnitude
// outside the configured min/max is ordinary, and a panic in wasm is a blank
// page rather than a bad pixel.
func TestColormapsClampRatherThanPanic(t *testing.T) {
	for _, fn := range []func(float64) color.Color{
		ValueToPixelTurbo, ValueToPixelViridis, ValueToPixelMagma,
	} {
		for _, v := range []float64{-5, -0.001, 1.001, 42} {
			fn(v) // must not panic
		}
	}
}

// Every scheme has to be reachable by the name the flags advertise, and come
// back under that name, or a --colors value is accepted and quietly ignored.
func TestEveryColorSchemeRoundTripsByName(t *testing.T) {
	s := DefaultSettings()
	for _, name := range []string{"heat", "blue", "grayscale", "turbo", "viridis", "magma"} {
		s.SetColorByName(name)
		if got := s.ColorName(); got != name {
			t.Errorf("set %q, got back %q", name, got)
		}
	}
}

// The new schemes must actually reach the renderer. Selecting one and getting
// heat back is the failure that a name round trip alone would not catch.
func TestSelectedSchemeReachesTheRenderer(t *testing.T) {
	s := DefaultSettings()
	s.SetColorByName("viridis")
	s.SetScaleByName("linear")
	s.SetMagMin(0)
	s.SetMagMax(1)

	rgba, ok := MagnitudeToPixelWith(0, s).(color.RGBA)
	if !ok {
		t.Fatal("the renderer did not return a color.RGBA")
	}
	want := [3]uint8{68, 1, 84}
	if got := [3]uint8{rgba.R, rgba.G, rgba.B}; got != want {
		t.Errorf("a viridis floor rendered %v, want viridis's %v", got, want)
	}
}
