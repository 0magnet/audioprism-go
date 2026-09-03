package wsaudio

import (
	"bytes"
	"encoding/hex"
	"math"
	"math/rand"
	"testing"
)

// THE TEST THAT KEEPS THE TWO REPOS IN STEP.
//
// datagram.go is a copy of chaosrack's pkg/audiosrc/datagram.go, and a
// copy drifts. A round-trip test cannot catch that: split and reassemble
// agree with each other no matter what the header says, so a framing that
// disagreed with chaosrack's byte for byte would pass every other test in
// this file while a chaosrack page talking to an audioprism-go server got
// silence.
//
// So the bytes are written out literally. If either repo ever changes the
// header, this fails on the spot and says which byte moved.
//
// The contract, stated once so it can be checked rather than remembered:
//
//	[0:2] uint16 message ID, LITTLE-endian
//	[2]   uint8  fragment index, from 0
//	[3]   uint8  fragment count
//	[4:]  payload, the little-endian float32 bytes Float32ToBytes produces
func TestDatagramHeaderIsExactlyTheseBytes(t *testing.T) {
	// Ten bytes into four-byte fragments: three datagrams, the last
	// short. Deliberately not a multiple of the payload size, so the
	// count and the final short fragment are both pinned.
	payload := []byte{
		0x00, 0x01, 0x02, 0x03,
		0x04, 0x05, 0x06, 0x07,
		0x08, 0x09,
	}
	// 0xBEEF has different high and low bytes, so a big-endian header
	// would be caught here rather than by luck.
	got := SplitDatagrams(0xBEEF, payload, DatagramHeaderSize+4)
	want := [][]byte{
		{0xEF, 0xBE, 0x00, 0x03, 0x00, 0x01, 0x02, 0x03},
		{0xEF, 0xBE, 0x01, 0x03, 0x04, 0x05, 0x06, 0x07},
		{0xEF, 0xBE, 0x02, 0x03, 0x08, 0x09},
	}
	if len(got) != len(want) {
		t.Fatalf("split into %d datagrams, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Errorf("datagram %d is %s, want %s", i, hex.EncodeToString(got[i]), hex.EncodeToString(want[i]))
		}
	}

	// A message ID of 0 and a single fragment: the smallest legal
	// datagram, and the one an implementation is most likely to get
	// right by accident and wrong at the edges.
	if one := SplitDatagrams(0, []byte{0xAA, 0xBB, 0xCC, 0xDD}, DefaultMaxDatagramSize); len(one) != 1 ||
		!bytes.Equal(one[0], []byte{0x00, 0x00, 0x00, 0x01, 0xAA, 0xBB, 0xCC, 0xDD}) {
		t.Errorf("a one-fragment message is %v, want 00 00 00 01 AA BB CC DD", one)
	}

	// The wrap value, so nobody "fixes" the ID into a big-endian or a
	// signed thing without hearing about it.
	if hi := SplitDatagrams(0xFFFF, []byte{1, 2, 3, 4}, DefaultMaxDatagramSize); len(hi) != 1 ||
		hi[0][0] != 0xFF || hi[0][1] != 0xFF || hi[0][2] != 0x00 || hi[0][3] != 0x01 {
		t.Errorf("message ID 0xFFFF produced the header % x, want ff ff 00 01", hi[0][:4])
	}

	// And the real case: one 0.1 s PulseAudio chunk at 24 kHz, 2400
	// samples, at the default datagram size. 9600 payload bytes over
	// 1196 per datagram is 9 fragments (8 full = 9568 bytes), the last
	// carrying the remaining 32.
	chunkDgs := SplitDatagrams(1, Float32ToBytes(make([]float32, 2400)), DefaultMaxDatagramSize)
	if len(chunkDgs) != 9 {
		t.Fatalf("a real 2400-sample chunk split into %d datagrams, want 9", len(chunkDgs))
	}
	for i, dg := range chunkDgs {
		wantHdr := []byte{0x01, 0x00, byte(i), 0x09} //nolint:gosec // i < 9
		if !bytes.Equal(dg[:DatagramHeaderSize], wantHdr) {
			t.Errorf("chunk fragment %d header is % x, want % x", i, dg[:DatagramHeaderSize], wantHdr)
		}
	}
	if len(chunkDgs[8]) != DatagramHeaderSize+32 {
		t.Errorf("the last fragment is %d bytes, want %d", len(chunkDgs[8]), DatagramHeaderSize+32)
	}
}

// A fragment boundary must never fall inside a float32, so the usable
// payload is the header-less remainder rounded down to a multiple of four.
func TestDatagramPayloadSize(t *testing.T) {
	cases := []struct{ max, want int }{
		{0, 0},
		{DatagramHeaderSize, 0},
		{DatagramHeaderSize + 3, 0}, // not even one sample fits
		{DatagramHeaderSize + 4, 4},
		{DatagramHeaderSize + 7, 4}, // three bytes wasted, deliberately
		{1200, 1196},
		{1201, 1196},
	}
	for _, c := range cases {
		if got := DatagramPayloadSize(c.max); got != c.want {
			t.Errorf("DatagramPayloadSize(%d) = %d, want %d", c.max, got, c.want)
		}
	}
	if DatagramPayloadSize(DefaultMaxDatagramSize)%4 != 0 {
		t.Error("the default datagram payload is not a whole number of samples")
	}
}

// The round trip is the contract: whatever is split must reassemble
// bit-exact, including the boundary case the split is most likely to get
// wrong — a payload that exactly fills a datagram, where an off-by-one
// produces either a spurious empty second fragment or a lost last sample.
func TestSplitReassembleRoundTrip(t *testing.T) {
	const maxSize = 1200
	per := DatagramPayloadSize(maxSize)

	sizes := []int{
		4,         // one sample
		per - 4,   // one short of full
		per,       // EXACTLY one datagram
		per + 4,   // one sample into a second datagram
		2 * per,   // exactly two datagrams
		3*per - 4, // three, the last one short
		9600,      // a real PulseAudio chunk: 0.1 s at 24 kHz float32
		255 * per, // the largest message the framing can carry
	}
	for _, n := range sizes {
		payload := make([]byte, n)
		for i := range payload {
			payload[i] = byte(i * 7)
		}
		dgs := SplitDatagrams(1234, payload, maxSize)
		if len(dgs) == 0 {
			t.Errorf("%d bytes split into nothing", n)
			continue
		}
		wantCount := (n + per - 1) / per
		if len(dgs) != wantCount {
			t.Errorf("%d bytes split into %d datagrams, want %d", n, len(dgs), wantCount)
		}
		for i, dg := range dgs {
			if len(dg) > maxSize {
				t.Errorf("%d bytes: datagram %d is %d bytes, over the %d limit", n, i, len(dg), maxSize)
			}
			// Every datagram but the last carries a full payload; a
			// short one in the middle would mean lost samples.
			if i < len(dgs)-1 && len(dg)-DatagramHeaderSize != per {
				t.Errorf("%d bytes: datagram %d carries %d payload bytes, want a full %d", n, i, len(dg)-DatagramHeaderSize, per)
			}
		}

		var r Reassembler
		var got []byte
		for i, dg := range dgs {
			out := r.Push(dg)
			if i < len(dgs)-1 && out != nil {
				t.Errorf("%d bytes: reassembly completed early, at fragment %d of %d", n, i, len(dgs))
			}
			if out != nil {
				got = out
			}
		}
		if !bytes.Equal(got, payload) {
			t.Errorf("%d bytes did not reassemble bit-exact (got %d bytes back)", n, len(got))
		}
		if r.Dropped != 0 {
			t.Errorf("%d bytes: dropped %d messages on a lossless round trip", n, r.Dropped)
		}
	}
}

// The samples are what actually has to survive, not the bytes: split,
// reassemble and decode with the same functions the WebSocket path uses.
func TestDatagramsCarryTheSameSamplesAsTheWebSocket(t *testing.T) {
	in := make([]float32, 2400) // 0.1 s at 24 kHz
	for i := range in {
		in[i] = float32(math.Sin(float64(i) * 0.01))
	}
	var r Reassembler
	var out []byte
	for _, dg := range SplitDatagrams(7, Float32ToBytes(in), DefaultMaxDatagramSize) {
		if p := r.Push(dg); p != nil {
			out = p
		}
	}
	got := BytesToFloat32(out)
	if len(got) != len(in) {
		t.Fatalf("decoded %d samples, want %d", len(got), len(in))
	}
	for i := range in {
		if math.Float32bits(got[i]) != math.Float32bits(in[i]) {
			t.Fatalf("sample %d = %v, want %v", i, got[i], in[i])
		}
	}
}

func TestSplitRefusesTheImpossible(t *testing.T) {
	if got := SplitDatagrams(0, nil, DefaultMaxDatagramSize); got != nil {
		t.Errorf("an empty payload split into %d datagrams", len(got))
	}
	if got := SplitDatagrams(0, []byte{1, 2, 3, 4}, DatagramHeaderSize+3); got != nil {
		t.Errorf("a datagram too small for one sample split into %d datagrams", len(got))
	}
	per := DatagramPayloadSize(DefaultMaxDatagramSize)
	if got := SplitDatagrams(0, make([]byte, 256*per), DefaultMaxDatagramSize); got != nil {
		t.Errorf("a payload needing 256 fragments split into %d datagrams instead of being refused", len(got))
	}
}

// Out-of-order delivery is normal for datagrams; only loss should cost
// anything.
func TestReassembleOutOfOrder(t *testing.T) {
	payload := make([]byte, 3000)
	for i := range payload {
		payload[i] = byte(i)
	}
	dgs := SplitDatagrams(42, payload, DefaultMaxDatagramSize)
	if len(dgs) != 3 {
		t.Fatalf("expected 3 fragments, got %d", len(dgs))
	}
	var r Reassembler
	var got []byte
	for _, i := range []int{2, 0, 1} {
		if out := r.Push(dgs[i]); out != nil {
			got = out
		}
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("a shuffled message reassembled to %d bytes, want %d intact", len(got), len(payload))
	}
	if r.Dropped != 0 {
		t.Errorf("shuffling counted as %d dropped messages", r.Dropped)
	}
}

// A lost fragment must cost exactly its own message and nothing after it.
// This is the entire reason the transport exists, so it is checked rather
// than asserted in a comment.
func TestLossCostsOneMessageOnly(t *testing.T) {
	payload := make([]byte, 3000)
	first := SplitDatagrams(1, payload, DefaultMaxDatagramSize)
	second := SplitDatagrams(2, payload, DefaultMaxDatagramSize)

	var r Reassembler
	for _, dg := range first[:len(first)-1] { // the last fragment never arrives
		if out := r.Push(dg); out != nil {
			t.Fatal("an incomplete message was delivered")
		}
	}
	var got []byte
	for _, dg := range second {
		if out := r.Push(dg); out != nil {
			got = out
		}
	}
	if len(got) != len(payload) {
		t.Fatalf("the message after a loss reassembled to %d bytes, want %d", len(got), len(payload))
	}
	if r.Dropped != 1 {
		t.Errorf("Dropped = %d after one lost fragment, want 1", r.Dropped)
	}
}

// Duplicates, stragglers from an abandoned message, and garbage must all
// be ignored rather than corrupt the message being assembled.
func TestReassemblerIgnoresJunk(t *testing.T) {
	payload := make([]byte, 3000)
	for i := range payload {
		payload[i] = 0xAB
	}
	dgs := SplitDatagrams(10, payload, DefaultMaxDatagramSize)

	var r Reassembler
	for _, bad := range [][]byte{
		nil,
		{1, 2, 3},                 // shorter than the header
		{0, 0, 0, 0},              // header only, no payload
		{10, 0, 0, 0, 1, 2, 3, 4}, // fragment count 0
		{10, 0, 5, 2, 1, 2, 3, 4}, // fragment index beyond the count
	} {
		if out := r.Push(bad); out != nil {
			t.Errorf("junk %v was accepted as a complete message", bad)
		}
	}

	var got []byte
	for _, dg := range dgs {
		// Sent twice: the duplicate must change nothing.
		if out := r.Push(dg); out != nil {
			got = out
		}
		if out := r.Push(dg); out != nil {
			got = out
		}
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("duplicated fragments reassembled to %d bytes, want %d intact", len(got), len(payload))
	}
	// A straggler from the completed message must not restart it.
	if out := r.Push(dgs[0]); out != nil {
		t.Error("a late fragment of a completed message produced another message")
	}
	// Nor must one from a message older than the current one.
	older := SplitDatagrams(9, payload, DefaultMaxDatagramSize)
	if out := r.Push(older[0]); out != nil {
		t.Error("a fragment of an older message was accepted")
	}
}

// The message ID is a uint16 and the stream runs for hours, so it wraps.
// A comparison that got this wrong would stall the feed for 32768
// messages — about an hour — every time it happened.
func TestMessageIDWrap(t *testing.T) {
	payload := make([]byte, 3000)
	var r Reassembler
	for _, id := range []uint16{65534, 65535, 0, 1} {
		var got []byte
		for _, dg := range SplitDatagrams(id, payload, DefaultMaxDatagramSize) {
			if out := r.Push(dg); out != nil {
				got = out
			}
		}
		if len(got) != len(payload) {
			t.Fatalf("message %d reassembled to %d bytes, want %d", id, len(got), len(payload))
		}
	}
	if r.Dropped != 0 {
		t.Errorf("Dropped = %d across the uint16 wrap, want 0", r.Dropped)
	}
	if !newerMsgID(0, 65535) || newerMsgID(65535, 0) {
		t.Error("newerMsgID does not handle the wrap")
	}
}

// Reset is what a reconnecting client calls: the fragments in flight
// belong to a session that is gone, and holding them would splice audio
// from before the break onto audio from after it.
func TestResetForgetsAPartialMessage(t *testing.T) {
	payload := make([]byte, 3000)
	dgs := SplitDatagrams(5, payload, DefaultMaxDatagramSize)
	var r Reassembler
	r.Push(dgs[0])
	r.Reset()
	// The same message ID again, from the top: after a Reset it must be
	// accepted as new rather than read as a straggler.
	var got []byte
	for _, dg := range SplitDatagrams(5, payload, DefaultMaxDatagramSize) {
		if out := r.Push(dg); out != nil {
			got = out
		}
	}
	if len(got) != len(payload) {
		t.Fatalf("after Reset the message reassembled to %d bytes, want %d", len(got), len(payload))
	}
}

// A shuffled, lossy link end to end: nothing may be delivered corrupt,
// and everything that is delivered must be one of the messages sent.
func TestLossyLinkNeverCorrupts(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // reproducible test input, not a key
	payload := make([]byte, 9600)      // a real chunk
	for i := range payload {
		payload[i] = byte(i * 3)
	}
	var r Reassembler
	delivered, sent := 0, 0
	for id := 0; id < 200; id++ {
		sent++
		dgs := SplitDatagrams(uint16(id), payload, DefaultMaxDatagramSize) //nolint:gosec // the wrap is the point
		rng.Shuffle(len(dgs), func(i, j int) { dgs[i], dgs[j] = dgs[j], dgs[i] })
		for _, dg := range dgs {
			if rng.Intn(20) == 0 {
				continue // 5% loss
			}
			out := r.Push(dg)
			if out == nil {
				continue
			}
			delivered++
			if !bytes.Equal(out, payload) {
				t.Fatalf("a delivered message does not match what was sent (%d bytes)", len(out))
			}
		}
	}
	// One last message with nothing dropped, so the message in flight at
	// the end of the loop is resolved and the accounting below is exact.
	sent++
	for _, dg := range SplitDatagrams(1000, payload, DefaultMaxDatagramSize) {
		if out := r.Push(dg); out != nil {
			delivered++
		}
	}
	if delivered == 0 {
		t.Fatal("5% loss delivered nothing at all")
	}
	if delivered+r.Dropped != sent {
		t.Errorf("%d delivered + %d dropped != %d sent", delivered, r.Dropped, sent)
	}
	t.Logf("5%% datagram loss over %d chunks: %d delivered, %d dropped", sent, delivered, r.Dropped)
}
