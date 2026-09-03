package wsaudio

import (
	"errors"
	"strings"
	"testing"
)

// The one rule that must never break: nothing short of an explicit
// ?audio=wt takes the page off the WebSocket. WebTransport is an option.
func TestWebSocketIsTheDefault(t *testing.T) {
	ok := WTProbe{Supported: true}
	for _, param := range []string{"", "ws", "websocket", "mic", "WS", " ", "wtf", "web transport"} {
		kind, reason := SelectTransport(param, ok)
		if kind != TransportWebSocket {
			t.Errorf("?audio=%q selected %q, want %q", param, kind, TransportWebSocket)
		}
		if reason != "" {
			t.Errorf("?audio=%q produced the status %q, want silence", param, reason)
		}
	}
}

func TestWebTransportIsOptIn(t *testing.T) {
	for _, param := range []string{"wt", "webtransport", "WT", " WebTransport "} {
		kind, reason := SelectTransport(param, WTProbe{Supported: true})
		if kind != TransportWebTransport {
			t.Errorf("?audio=%q selected %q, want %q", param, kind, TransportWebTransport)
		}
		if reason != "" {
			t.Errorf("?audio=%q produced the status %q on success, want silence", param, reason)
		}
	}
}

// Every way the optional path can fail must land back on the WebSocket
// with something to show the user — a silent fallback looks identical to
// WebTransport working, which is the failure mode most likely to waste
// somebody's afternoon.
func TestFallbackIsAlwaysToWebSocketAndAlwaysSaysSo(t *testing.T) {
	cases := []struct {
		name  string
		probe WTProbe
		want  string // a substring the message must carry
	}{
		{"no browser support (Safari, old Chrome)", WTProbe{}, "not supported"},
		{"server has no WebTransport listener", WTProbe{Supported: true, InfoErr: errors.New("404 fetching /wt-info")}, "404 fetching /wt-info"},
		{"handshake or certificate refused", WTProbe{Supported: true, DialErr: errors.New("cert hash rejected")}, "cert hash rejected"},
		{"UDP blocked on the path", WTProbe{Supported: true, DialErr: errors.New("connection timed out")}, "connection timed out"},
	}
	for _, c := range cases {
		kind, reason := SelectTransport("wt", c.probe)
		if kind != TransportWebSocket {
			t.Errorf("%s: selected %q, want a fallback to %q", c.name, kind, TransportWebSocket)
		}
		if !strings.Contains(reason, c.want) {
			t.Errorf("%s: status %q does not mention %q", c.name, reason, c.want)
		}
		if !strings.Contains(reason, "WebSocket") {
			t.Errorf("%s: status %q does not say what it fell back to", c.name, reason)
		}
	}
}

// The probe is filled in stage by stage, so a later failure must not be
// hidden by an earlier success, and an unsupported browser must be
// reported as such rather than as whatever error the dial then produced.
func TestFallbackReportsTheEarliestCause(t *testing.T) {
	_, reason := SelectTransport("wt", WTProbe{
		Supported: false,
		InfoErr:   errors.New("info"),
		DialErr:   errors.New("dial"),
	})
	if !strings.Contains(reason, "not supported") {
		t.Errorf("status %q, want the missing browser support reported first", reason)
	}
	_, reason = SelectTransport("wt", WTProbe{
		Supported: true,
		InfoErr:   errors.New("info"),
		DialErr:   errors.New("dial"),
	})
	if !strings.Contains(reason, "info") || strings.Contains(reason, "dial") {
		t.Errorf("status %q, want the /wt-info failure reported before the dial", reason)
	}
}
