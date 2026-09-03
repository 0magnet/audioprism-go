//go:build js && wasm

package main

import (
	"encoding/base64"
	"errors"
	"log"
	"strings"
	"syscall/js"

	"github.com/0magnet/audioprism-go/pkg/wsaudio"
)

// The browser end of the optional WebTransport audio path (?audio=wt).
//
// Kept as thin as it can be, on purpose: nothing in this file can be unit
// tested — there is no WebTransport in Node, so `make test-wasm` cannot
// reach a line of it. Every decision it makes is therefore made by code
// that CAN be tested natively: wsaudio.SelectTransport decides whether to
// fall back and what to say about it, wsaudio.Reassembler puts the
// datagrams back together, and wsaudio.BytesToFloat32 decodes the samples
// — the same function the WebSocket path uses, because both transports
// carry identical bytes. See pkg/wsaudio/datagram.go for why datagrams
// rather than a stream, and why the framing is what it is.
//
// What is left here is plumbing: fetch the certificate hash, open the
// session, pump the datagram reader, and hand the pieces to the code
// above.

// wtInfoURL is where the server publishes the WebTransport endpoint and
// its certificate fingerprint. The fingerprint cannot be compiled in: the
// certificate is generated per run because the browser API that pins one
// by hash refuses anything valid for more than 14 days.
const wtInfoURL = "/wt-info"

// wt is the live session, and fellBack records that this page has given
// up on WebTransport — several promise handlers can fire for the same
// failure (a rejected handshake also closes the session) and only the
// first one counts.
var (
	wt       js.Value
	wtReader js.Value
	wtRA     wsaudio.Reassembler
	wtBuf    []byte // scratch for one datagram; Reassembler.Push copies
	fellBack bool
)

// transportNotice is the fallback reason, shown in the FPS overlay when
// there is one and always written to the console. A silent fallback would
// be indistinguishable from WebTransport working, which is the worst
// possible outcome for a feature whose entire point is what happens on a
// bad link.
var transportNotice string

// initAudioTransport picks the transport from ?audio= and starts it.
//
// The WebSocket is the default and the fallback; nothing short of an
// explicit ?audio=wt takes this page off it.
func initAudioTransport() {
	param := getQueryParam("audio")
	if param == "<null>" {
		param = ""
	}
	supported := isWebTransportSupported()
	kind, reason := wsaudio.SelectTransport(param, wsaudio.WTProbe{Supported: supported})
	if kind != wsaudio.TransportWebTransport {
		if reason != "" {
			noteFallback(reason)
		}
		initWS()
		return
	}
	fetchWTInfo()
}

func isWebTransportSupported() bool {
	v := js.Global().Get("WebTransport")
	return !v.IsUndefined() && !v.IsNull()
}

// fetchWTInfo asks the page's own origin where the WebTransport endpoint
// is and what certificate to trust. A failure here is the ordinary case
// of a server started with --wt=false, so it falls back quietly.
func fetchWTInfo() {
	onErr := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		wtFallback(wsaudio.WTProbe{Supported: true, InfoErr: jsError(args, "cannot reach "+wtInfoURL)})
		return nil
	})
	onJSON := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			wtFallback(wsaudio.WTProbe{Supported: true, InfoErr: errors.New("empty " + wtInfoURL)})
			return nil
		}
		info := args[0]
		u, hash := info.Get("url"), info.Get("certHash")
		if !u.Truthy() || !hash.Truthy() {
			wtFallback(wsaudio.WTProbe{Supported: true, InfoErr: errors.New(wtInfoURL + " has no url/certHash")})
			return nil
		}
		wtDial(u.String(), hash.String())
		return nil
	})
	onResp := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) == 0 || !args[0].Get("ok").Bool() {
			wtFallback(wsaudio.WTProbe{Supported: true, InfoErr: errors.New(wtInfoURL + " is not being served")})
			return nil
		}
		args[0].Call("json").Call("then", onJSON).Call("catch", onErr)
		return nil
	})
	js.Global().Call("fetch", wtInfoURL).Call("then", onResp).Call("catch", onErr)
}

// wtDial opens the session, pinning the certificate by fingerprint.
//
// serverCertificateHashes is what makes a generated certificate usable
// from a browser at all: no CA has signed it, and without this the only
// alternatives are installing it in the machine's trust store or running
// the browser with --ignore-certificate-errors. Its cost is the 14-day
// validity cap, which is why the fingerprint is fetched at runtime rather
// than built in.
func wtDial(url, certHash string) {
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(certHash, "="))
	if err != nil {
		wtFallback(wsaudio.WTProbe{Supported: true, DialErr: errors.New("unreadable certificate hash")})
		return
	}
	hashBytes := js.Global().Get("Uint8Array").New(len(raw))
	js.CopyBytesToJS(hashBytes, raw)

	entry := js.Global().Get("Object").New()
	entry.Set("algorithm", "sha-256")
	entry.Set("value", hashBytes)
	hashes := js.Global().Get("Array").New()
	hashes.Call("push", entry)
	init := js.Global().Get("Object").New()
	init.Set("serverCertificateHashes", hashes)

	// The constructor throws synchronously on a malformed URL, which in
	// Go/wasm arrives as a panic rather than an error.
	defer func() {
		if r := recover(); r != nil {
			wtFallback(wsaudio.WTProbe{Supported: true, DialErr: errors.New("cannot open " + url)})
		}
	}()
	sess := js.Global().Get("WebTransport").New(url, init)
	wt = sess

	onDialErr := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		wtFallback(wsaudio.WTProbe{Supported: true, DialErr: jsError(args, "handshake to "+url+" failed")})
		return nil
	})
	onReady := js.FuncOf(func(js.Value, []js.Value) interface{} {
		log.Printf("WebTransport connected to %s", url)
		wtStartReading()
		return nil
	})
	sess.Get("ready").Call("then", onReady).Call("catch", onDialErr)
	// A session that dies after working falls back too. Reconnecting the
	// WebTransport instead would mean re-fetching the fingerprint and
	// re-dialing on a link that has just proved unreliable, while the
	// WebSocket path already reconnects itself every 2 s — so the simpler
	// path is also the one that recovers.
	sess.Get("closed").Call("then", js.FuncOf(func(js.Value, []js.Value) interface{} {
		wtFallback(wsaudio.WTProbe{Supported: true, DialErr: errors.New("session closed")})
		return nil
	})).Call("catch", onDialErr)
}

// wtStartReading pumps the datagram reader. Each datagram goes to the
// reassembler, which returns a complete audio chunk or nothing.
func wtStartReading() {
	if fellBack {
		return
	}
	dgrams := wt.Get("datagrams")
	if !dgrams.Truthy() || !dgrams.Get("readable").Truthy() {
		wtFallback(wsaudio.WTProbe{Supported: true, DialErr: errors.New("session has no datagram support")})
		return
	}
	wtReader = dgrams.Get("readable").Call("getReader")

	onReadErr := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		wtFallback(wsaudio.WTProbe{Supported: true, DialErr: jsError(args, "datagram stream ended")})
		return nil
	})
	// Declared before it is defined because it re-arms itself: read()
	// resolves once per datagram, so the handler has to schedule the next
	// read from inside itself.
	var onChunk js.Func
	onChunk = js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if fellBack {
			return nil
		}
		if len(args) == 0 || args[0].Get("done").Bool() {
			wtFallback(wsaudio.WTProbe{Supported: true, DialErr: errors.New("datagram stream ended")})
			return nil
		}
		wtConsume(args[0].Get("value"))
		// Re-arm before returning: read() resolves once per datagram.
		wtReader.Call("read").Call("then", onChunk).Call("catch", onReadErr)
		return nil
	})
	wtReader.Call("read").Call("then", onChunk).Call("catch", onReadErr)
}

// wtConsume copies one datagram into Go and feeds it to the reassembler.
// A complete chunk goes to exactly the same two calls the WebSocket
// message handler makes, so the spectrogram and the audio engine cannot
// tell which transport delivered it.
func wtConsume(val js.Value) {
	if !val.Truthy() {
		return
	}
	n := val.Get("length").Int()
	if n <= 0 {
		return
	}
	if cap(wtBuf) < n {
		wtBuf = make([]byte, n)
	}
	b := wtBuf[:n]
	js.CopyBytesToGo(b, val)
	payload := wtRA.Push(b)
	if payload == nil {
		return
	}
	if samples := wsaudio.BytesToFloat32(payload); len(samples) > 0 {
		enqueueAudio(samples)
		processAudio(samples)
	}
}

// wtFallback runs the probe through the same decision function
// initAudioTransport used, so there is exactly one place that decides
// what a failure means and how it is worded, then hands the job to the
// WebSocket. Idempotent.
func wtFallback(p wsaudio.WTProbe) {
	if fellBack {
		return
	}
	_, reason := wsaudio.SelectTransport("wt", p)
	if reason == "" {
		// A probe carrying no failure at all; there is nothing to report
		// but we were called, so something went wrong.
		reason = "WebTransport stopped for an unstated reason — using WebSocket"
	}
	fellBack = true
	noteFallback(reason)
	wtRA.Reset()
	if !wt.IsUndefined() && wt.Truthy() {
		wt.Call("close")
		wt = js.Undefined()
	}
	initWS()
}

// noteFallback records the reason on the page's existing status paths:
// the console, which is where every other connection message goes, and
// the FPS overlay when one was asked for, so it is visible without
// opening devtools.
func noteFallback(reason string) {
	transportNotice = reason
	log.Println(reason)
}

// jsError turns a rejected promise's argument into a Go error, falling
// back to a description of what was being attempted. Browsers word these
// rejections differently and some of them carry nothing useful, so the
// caller's own phrasing is what the user usually sees.
func jsError(args []js.Value, fallback string) error {
	if len(args) > 0 && args[0].Truthy() {
		if msg := args[0].Get("message"); msg.Truthy() {
			return errors.New(msg.String())
		}
		if args[0].Type() == js.TypeString {
			return errors.New(args[0].String())
		}
	}
	return errors.New(fallback)
}
