package spectrogram

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// tone builds n samples of a sine, which is enough to give the transform
// something to find rather than a flat line.
const cyclesPerSample = 0.05

func tone(n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(math.Sin(2 * math.Pi * cyclesPerSample * float64(i)))
	}
	return out
}

func settings(t *testing.T) *Settings {
	t.Helper()
	s := DefaultSettings()
	return s
}

func TestOrientedRect(t *testing.T) {
	if got := orientedRect(100, 40, Horizontal); got != image.Rect(0, 0, 100, 40) {
		t.Errorf("horizontal rect = %v, want time along x", got)
	}
	if got := orientedRect(100, 40, Vertical); got != image.Rect(0, 0, 40, 100) {
		t.Errorf("vertical rect = %v, want time along y", got)
	}
}

// The image has one column per transform, which is what makes a spectrogram
// as long as the audio rather than an arbitrary size.
func TestRenderHasOneColumnPerTransform(t *testing.T) {
	s := settings(t)
	size, step := s.GetDFTSize(), s.StepSize()
	if step < 1 {
		t.Fatalf("step size is %d", step)
	}
	const extra = 5
	samples := tone(size + step*extra)

	img := Render(samples, 64, Horizontal, s)
	if img == nil {
		t.Fatal("Render returned nothing for a signal longer than one window")
	}
	want := (len(samples)-size)/step + 1
	if got := img.Bounds().Dx(); got != want {
		t.Errorf("the image is %d columns wide, want %d", got, want)
	}
	if got := img.Bounds().Dy(); got != 64 {
		t.Errorf("the image is %d tall, want the 64 spectrum pixels asked for", got)
	}
}

func TestRenderVerticalTransposesTheImage(t *testing.T) {
	s := settings(t)
	samples := tone(s.GetDFTSize() + s.StepSize()*3)

	h := Render(samples, 32, Horizontal, s)
	v := Render(samples, 32, Vertical, s)
	if h == nil || v == nil {
		t.Fatal("Render returned nothing")
	}
	if h.Bounds().Dx() != v.Bounds().Dy() || h.Bounds().Dy() != v.Bounds().Dx() {
		t.Errorf("horizontal is %v and vertical is %v; they should be transposes",
			h.Bounds(), v.Bounds())
	}
}

// Too little audio to fill one window has no spectrum at all, and returning
// nothing is what the callers check for.
func TestRenderNeedsAtLeastOneWindow(t *testing.T) {
	s := settings(t)
	for _, n := range []int{0, 1, s.GetDFTSize() - 1} {
		if img := Render(tone(n), 32, Horizontal, s); img != nil {
			t.Errorf("%d samples produced an image of %v", n, img.Bounds())
		}
	}
	// Exactly one window is one column, not nothing.
	if img := Render(tone(s.GetDFTSize()), 32, Horizontal, s); img == nil {
		t.Error("exactly one window produced no image")
	} else if got := img.Bounds().Dx(); got != 1 {
		t.Errorf("exactly one window produced %d columns, want 1", got)
	}
}

// A spectrum has to be at least one pixel wide; zero or negative is a caller
// mistake that would otherwise divide by zero in renderSpectrum.
func TestRenderRejectsANonPositiveWidth(t *testing.T) {
	s := settings(t)
	samples := tone(s.GetDFTSize() * 2)
	for _, w := range []int{0, -1, -100} {
		if img := Render(samples, w, Horizontal, s); img != nil {
			t.Errorf("width %d produced an image of %v", w, img.Bounds())
		}
	}
}

// Passing no settings uses the shared ones rather than crashing, which is what
// the UIs that never build a Settings rely on.
func TestRenderWithNilSettingsUsesTheSharedOnes(t *testing.T) {
	samples := tone(S.GetDFTSize() * 2)
	if img := Render(samples, 16, Horizontal, nil); img == nil {
		t.Error("Render with nil settings produced nothing")
	}
}

// Every pixel has to be opaque: a spectrogram drawn onto a dark page with a
// transparent alpha is invisible.
func TestRenderProducesOpaquePixels(t *testing.T) {
	s := settings(t)
	img := Render(tone(s.GetDFTSize()*2), 16, Horizontal, s)
	if img == nil {
		t.Fatal("no image")
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				t.Fatalf("pixel %d,%d has alpha %d, want opaque", x, y, a)
			}
		}
	}
}

// ── renderSpectrum ───────────────────────────────────────────────────────────

// The magnitudes are spread across however many pixels there are, so a line
// wider or narrower than the bin count still fills exactly.
func TestRenderSpectrumFillsEveryPixel(t *testing.T) {
	s := settings(t)
	mags := make([]float64, 64)
	for i := range mags {
		mags[i] = float64(i) / 64
	}
	for _, width := range []int{1, 8, 64, 200} {
		line := make([]color.Color, width)
		renderSpectrum(line, mags, s)
		for i, px := range line {
			if px == nil {
				t.Fatalf("width %d: pixel %d was not filled", width, i)
			}
		}
	}
}

// The last pixel must not index past the last bin, which is what the clamp in
// renderSpectrum is for.
func TestRenderSpectrumStaysInsideTheBins(t *testing.T) {
	s := settings(t)
	mags := []float64{0.1, 0.2, 0.3}
	// More pixels than bins is the case where rounding can overshoot.
	line := make([]color.Color, 1000)
	renderSpectrum(line, mags, s) // must not panic
	if line[len(line)-1] == nil {
		t.Error("the last pixel was not filled")
	}
}

// No bins is silence rather than a panic; the line is left as it was.
func TestRenderSpectrumWithNoBins(t *testing.T) {
	s := settings(t)
	line := make([]color.Color, 4)
	renderSpectrum(line, nil, s)
	for i, px := range line {
		if px != nil {
			t.Errorf("pixel %d was written from no bins: %v", i, px)
		}
	}
}

// A brighter magnitude has to give a different pixel than a dim one, or the
// spectrogram is a flat field.
func TestRenderSpectrumDistinguishesMagnitudes(t *testing.T) {
	s := settings(t)
	quiet := make([]color.Color, 1)
	loud := make([]color.Color, 1)
	renderSpectrum(quiet, []float64{0}, s)
	renderSpectrum(loud, []float64{1e6}, s)
	if quiet[0] == loud[0] {
		t.Error("silence and a loud bin render as the same pixel")
	}
}
