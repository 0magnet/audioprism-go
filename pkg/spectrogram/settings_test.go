package spectrogram

import "testing"

// These pin the values against the C++ original this package is a port of,
// because reading that code got them wrong once already. The --help text there
// advertises a default magnitude maximum of 50.0; the code it documents starts
// at 45.0 and only reaches 50.0 when the `l` key toggles the scale. Rendering
// the same WAV through both settled it — at 45 the two agree pixel for pixel —
// so the number is nailed down here rather than left to the next reader of a
// header file.
//
// References are to the original's src/main/Configuration.hpp (Settings and
// Limits) and src/main/InterfaceThread.cpp (_handleKeyDown).

func TestDefaultsMatchOriginal(t *testing.T) {
	s := DefaultSettings()
	// Configuration.hpp, struct Settings.
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"MagMin", s.MagMin, 0.0},
		{"MagMax", s.MagMax, 45.0},
		{"Overlap", s.Overlap, 0.50},
		{"DFTSize", float64(s.DFTSize), 1024},
	} {
		if c.got != c.want {
			t.Errorf("default %s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if s.Color != ColorHeat || s.Window != WindowHann || s.Mag != ScaleLog {
		t.Errorf("default color/window/scale = %v/%v/%v, want heat/hann/log",
			s.ColorName(), s.WindowName(), s.ScaleName())
	}
}

// InterfaceThread.cpp `l`: both directions reset to InitialSettings'
// magnitude{Log,Linear}{Min,Max}, which are 0..50 either way. 1000 is the far
// end of Limits.magnitudeLinearMax — where the user may adjust to, not where
// the toggle lands them.
func TestToggleScaleWindows(t *testing.T) {
	s := DefaultSettings()
	s.ToggleScale()
	if s.Mag != ScaleLinear || s.MagMin != 0.0 || s.MagMax != 50.0 {
		t.Errorf("after toggle to linear: %s %v..%v, want linear 0..50",
			s.ScaleName(), s.MagMin, s.MagMax)
	}
	s.ToggleScale()
	if s.Mag != ScaleLog || s.MagMin != 0.0 || s.MagMax != 50.0 {
		t.Errorf("after toggle back to log: %s %v..%v, want logarithmic 0..50",
			s.ScaleName(), s.MagMin, s.MagMax)
	}
}

// The window must not be able to invert. A minimum driven above the maximum
// gives Normalize a negative span, which paints the spectrogram backwards
// rather than failing.
func TestMagnitudeWindowCannotInvert(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scale Scale
		push  func(*Settings)
	}{
		{"log, min pushed up", ScaleLog, func(s *Settings) {
			for i := 0; i < 100; i++ {
				s.AdjustMin(1)
			}
		}},
		{"log, max pushed down", ScaleLog, func(s *Settings) {
			for i := 0; i < 100; i++ {
				s.AdjustMax(-1)
			}
		}},
		{"linear, min pushed up", ScaleLinear, func(s *Settings) {
			for i := 0; i < 100; i++ {
				s.AdjustMin(1)
			}
		}},
		{"linear, max pushed down", ScaleLinear, func(s *Settings) {
			for i := 0; i < 100; i++ {
				s.AdjustMax(-1)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := DefaultSettings()
			s.Mag = tc.scale
			tc.push(s)
			if s.MagMin >= s.MagMax {
				t.Fatalf("window inverted: %v..%v", s.MagMin, s.MagMax)
			}
		})
	}
}

// Configuration.hpp, struct Limits: the log window may range over -80..80 in
// steps of 5, the linear one over 0..1000 in steps of 25.
func TestMagnitudeOuterLimits(t *testing.T) {
	s := DefaultSettings()
	for i := 0; i < 100; i++ {
		s.AdjustMin(-1)
	}
	if s.MagMin != -80 {
		t.Errorf("log MagMin floor = %v, want -80", s.MagMin)
	}
	for i := 0; i < 100; i++ {
		s.AdjustMax(1)
	}
	if s.MagMax != 80 {
		t.Errorf("log MagMax ceiling = %v, want 80", s.MagMax)
	}

	s = DefaultSettings()
	s.ToggleScale() // linear
	for i := 0; i < 100; i++ {
		s.AdjustMax(1)
	}
	if s.MagMax != 1000 {
		t.Errorf("linear MagMax ceiling = %v, want 1000", s.MagMax)
	}
	for i := 0; i < 100; i++ {
		s.AdjustMin(-1)
	}
	if s.MagMin != 0 {
		t.Errorf("linear MagMin floor = %v, want 0", s.MagMin)
	}
}

// InterfaceThread.cpp Left/Right: the size doubles or halves within 64..8192,
// and each change resets the overlap to 50%.
func TestDFTSizeStepsAndResetsOverlap(t *testing.T) {
	s := DefaultSettings()
	s.SetOverlap(0.90)
	s.DoubleFFTSize()
	if s.DFTSize != 2048 {
		t.Errorf("DFTSize after double = %d, want 2048", s.DFTSize)
	}
	if s.Overlap != 0.50 {
		t.Errorf("overlap after DFT change = %v, want 0.50 (the original resets it)", s.Overlap)
	}
	for i := 0; i < 20; i++ {
		s.DoubleFFTSize()
	}
	if s.DFTSize != 8192 {
		t.Errorf("DFTSize ceiling = %d, want 8192", s.DFTSize)
	}
	for i := 0; i < 20; i++ {
		s.HalveFFTSize()
	}
	if s.DFTSize != 64 {
		t.Errorf("DFTSize floor = %d, want 64", s.DFTSize)
	}
}

func TestSetDFTSizeRoundsUpWithinLimits(t *testing.T) {
	s := DefaultSettings()
	for _, tc := range []struct{ in, want int }{
		{1, 64}, {64, 64}, {65, 128}, {1000, 1024}, {1024, 1024}, {99999, 8192},
	} {
		s.SetDFTSize(tc.in)
		if s.DFTSize != tc.want {
			t.Errorf("SetDFTSize(%d) = %d, want %d", tc.in, s.DFTSize, tc.want)
		}
	}
}

func TestOverlapLimits(t *testing.T) {
	s := DefaultSettings()
	for i := 0; i < 200; i++ {
		s.AdjustOverlap(0.01)
	}
	if s.Overlap != 0.95 {
		t.Errorf("overlap ceiling = %v, want 0.95", s.Overlap)
	}
	for i := 0; i < 200; i++ {
		s.AdjustOverlap(-0.01)
	}
	if s.Overlap != 0.05 {
		t.Errorf("overlap floor = %v, want 0.05", s.Overlap)
	}
}

// Every name the original's CLI accepts has to land somewhere, and the cycles
// have to visit every value — a color scheme that exists in the enum but is
// unreachable from both the flag and the key is not actually offered.
func TestNamesAndCyclesCoverEveryValue(t *testing.T) {
	s := DefaultSettings()

	colors := []string{"heat", "blue", "grayscale"}
	for _, name := range colors {
		s.SetColorByName(name)
		if s.ColorName() != name {
			t.Errorf("SetColorByName(%q) → %q", name, s.ColorName())
		}
	}
	windows := []string{"hann", "hamming", "bartlett", "rectangular"}
	for _, name := range windows {
		s.SetWindowByName(name)
		if s.WindowName() != name {
			t.Errorf("SetWindowByName(%q) → %q", name, s.WindowName())
		}
	}
	for _, name := range []string{"logarithmic", "linear"} {
		s.SetScaleByName(name)
		if s.ScaleName() != name {
			t.Errorf("SetScaleByName(%q) → %q", name, s.ScaleName())
		}
	}

	seen := map[string]bool{}
	s.SetColorByName("heat")
	for i := 0; i < len(colors); i++ {
		seen[s.ColorName()] = true
		s.CycleColor()
	}
	for _, name := range colors {
		if !seen[name] {
			t.Errorf("CycleColor never reaches %q", name)
		}
	}
	if s.ColorName() != "heat" {
		t.Errorf("CycleColor does not return to its start after %d steps", len(colors))
	}

	seen = map[string]bool{}
	s.SetWindowByName("hann")
	for i := 0; i < len(windows); i++ {
		seen[s.WindowName()] = true
		s.CycleWindow()
	}
	for _, name := range windows {
		if !seen[name] {
			t.Errorf("CycleWindow never reaches %q", name)
		}
	}
	if s.WindowName() != "hann" {
		t.Errorf("CycleWindow does not return to its start after %d steps", len(windows))
	}
}

// The original takes the overlap in samples and subtracts it from the size;
// computing size×(1-overlap) directly rounds at a different point and comes out
// one sample short wherever size×overlap is not whole. One sample per column is
// a drift, not a rounding detail — it accumulates across the whole picture, and
// it made every overlap except 50% disagree with the original pixel for pixel.
func TestStepSizeRoundsAsTheOriginalDoes(t *testing.T) {
	s := DefaultSettings()
	for _, tc := range []struct {
		size    int
		overlap float64
		want    int
	}{
		{1024, 0.50, 512}, // where the two orderings agree
		{1024, 0.05, 973}, // 51.2 truncates to 51; 1024-51, not int(972.8)
		{1024, 0.10, 922},
		{1024, 0.90, 103},
		{1024, 0.95, 52},
		{256, 0.33, 172},
	} {
		s.SetDFTSize(tc.size)
		s.SetOverlap(tc.overlap)
		if got := s.StepSize(); got != tc.want {
			t.Errorf("StepSize at %d points, %g overlap = %d, want %d",
				tc.size, tc.overlap, got, tc.want)
		}
	}
}

// The original's --overlap is a percentage. These flags were declared as a
// ratio under the same name, so following the original's documentation and
// passing 50 clamped to 0.95 without saying so.
func TestOverlapFromFlagAcceptsBothSpellings(t *testing.T) {
	for _, tc := range []struct{ in, want float64 }{
		{50, 0.50}, {5, 0.05}, {95, 0.95}, // percentages, as the original takes
		{0.50, 0.50}, {0.05, 0.05}, {0.95, 0.95}, // ratios, as these flags took
	} {
		if got := OverlapFromFlag(tc.in); got != tc.want {
			t.Errorf("OverlapFromFlag(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// ComputeFFT returns every bin of a real DFT including Nyquist — N/2+1, as the
// original's RealDft does — because the offline renderer spreads however many
// bins there are across a fixed number of pixels, so the count is part of the
// mapping and not just a length.
func TestFFTReturnsNyquistBin(t *testing.T) {
	s := DefaultSettings()
	for _, n := range []int{64, 256, 1024} {
		s.SetDFTSize(n)
		mags := ComputeFFTWith(make([]float32, n), s)
		if len(mags) != n/2+1 {
			t.Errorf("ComputeFFT over %d samples returned %d magnitudes, want %d", n, len(mags), n/2+1)
		}
	}
}

// StepSize is what the frame loop advances by, so an overlap that produces a
// step of zero would wedge it.
func TestStepSizeFollowsOverlap(t *testing.T) {
	s := DefaultSettings()
	if got := s.StepSize(); got != 512 {
		t.Errorf("StepSize at 1024/50%% = %d, want 512", got)
	}
	s.SetOverlap(0.95)
	if got := s.StepSize(); got <= 0 {
		t.Errorf("StepSize at maximum overlap = %d, must stay positive", got)
	}
	s.SetDFTSize(64)
	s.SetOverlap(0.95)
	if got := s.StepSize(); got <= 0 {
		t.Errorf("StepSize at the smallest DFT and largest overlap = %d, must stay positive", got)
	}
}
