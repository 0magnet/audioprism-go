package wsaudio

import (
	"math"
	"testing"
)

// The byte order is the whole contract with the browser: a decoder that
// reads big-endian gets plausible-looking noise rather than an error, so
// the layout is pinned to literal bytes here and not just round-tripped.
func TestFloat32ToBytesLittleEndian(t *testing.T) {
	got := Float32ToBytes([]float32{1, -2})
	want := []byte{0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0xc0}
	if len(got) != len(want) {
		t.Fatalf("encoded %d bytes, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("byte %d = %#02x, want %#02x (whole: % x)", i, got[i], want[i], got)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	in := make([]float32, 512)
	for i := range in {
		in[i] = float32(math.Sin(float64(i) * 0.05))
	}
	got := BytesToFloat32(Float32ToBytes(in))
	if len(got) != len(in) {
		t.Fatalf("decoded %d samples, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Fatalf("sample %d = %v, want %v (not bit-exact)", i, got[i], in[i])
		}
	}
}

// float32 is copied bit-for-bit, never through a float64 or an arithmetic
// op, so the non-finite values a dying capture can emit survive the trip
// instead of turning into 0 or a different NaN.
func TestNonFinite(t *testing.T) {
	in := []float32{
		float32(math.Inf(1)),
		float32(math.Inf(-1)),
		float32(math.NaN()),
		math.Float32frombits(0x7fc00001), // quiet NaN carrying a payload
		math.Float32frombits(0xffc00001), // negative quiet NaN
		0,
		math.Float32frombits(0x80000000), // negative zero
	}
	got := BytesToFloat32(Float32ToBytes(in))
	if len(got) != len(in) {
		t.Fatalf("decoded %d samples, want %d", len(got), len(in))
	}
	for i := range in {
		if math.Float32bits(got[i]) != math.Float32bits(in[i]) {
			t.Errorf("sample %d = %#08x, want %#08x", i, math.Float32bits(got[i]), math.Float32bits(in[i]))
		}
	}
}

func TestBytesToFloat32Short(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3} {
		if got := BytesToFloat32(make([]byte, n)); got != nil {
			t.Errorf("%d bytes decoded to %v, want nil", n, got)
		}
	}
	// A trailing partial sample is dropped, and the complete ones kept.
	b := append(Float32ToBytes([]float32{3, 4}), 0x01, 0x02)
	got := BytesToFloat32(b)
	if len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Errorf("decoded %v from two samples plus a two-byte tail, want [3 4]", got)
	}
}
