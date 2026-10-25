package channel

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
)

type upstreamTimingPhase struct{ start, end time.Time }

// upstreamTiming measures one HTTP attempt through its first body byte. Phase
// windows can overlap (connection acquisition includes DNS/TCP/TLS); absent
// phases are omitted, especially when a pooled connection is reused.
type upstreamTiming struct {
	mu           sync.Mutex
	once         sync.Once
	bodyOnce     sync.Once
	started      time.Time
	phases       map[string]upstreamTimingPhase
	phase        string
	ctx, request context.Context
	channelID    int
	attempt      int
	stream       bool
	status       int
	reused       bool
	deferLog     bool
	bodyOutcome  string
	bodyAt       time.Time
}

func newUpstreamTiming(ctx context.Context, req *http.Request, channelID, attempt int, stream, deferLog bool) (*upstreamTiming, *http.Request) {
	t := &upstreamTiming{started: time.Now(), phases: make(map[string]upstreamTimingPhase), phase: "connection_acquire",
		ctx: ctx, request: req.Context(), channelID: channelID, attempt: attempt, stream: stream, deferLog: deferLog}
	trace := &httptrace.ClientTrace{
		GetConn: func(string) { t.start("connection_acquire") },
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			t.reused = info.Reused
			t.mu.Unlock()
			t.end("connection_acquire")
			t.start("request_write")
		},
		DNSStart:          func(httptrace.DNSStartInfo) { t.start("dns") },
		DNSDone:           func(httptrace.DNSDoneInfo) { t.end("dns") },
		ConnectStart:      func(string, string) { t.start("tcp_connect") },
		ConnectDone:       func(string, string, error) { t.end("tcp_connect") },
		TLSHandshakeStart: func() { t.start("tls_handshake") },
		TLSHandshakeDone:  func(tls.ConnectionState, error) { t.end("tls_handshake") },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			t.end("request_write")
			if info.Err == nil {
				t.start("response_headers")
			}
		},
	}
	return t, req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
}

func (t *upstreamTiming) start(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.phases[name]
	if p.start.IsZero() {
		p.start = time.Now()
	}
	p.end = time.Time{}
	t.phases[name], t.phase = p, name
}

func (t *upstreamTiming) end(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.phases[name]
	if !p.start.IsZero() {
		p.end = time.Now()
		t.phases[name] = p
	}
}

func (t *upstreamTiming) response(resp *http.Response) {
	t.end("response_headers")
	t.mu.Lock()
	t.status = resp.StatusCode
	t.mu.Unlock()
	t.start("first_body_wait")
	resp.Body = &upstreamTimingBody{ReadCloser: resp.Body, timing: t}
}

func (t *upstreamTiming) finish(outcome string, err error) {
	t.once.Do(func() {
		t.mu.Lock()
		observedAt := time.Now()
		if err == nil && t.bodyOutcome != "" {
			outcome, observedAt = t.bodyOutcome, t.bodyAt
		}
		t.mu.Unlock()
		switch {
		case t.ctx.Err() != nil:
			outcome = "client_canceled"
		case errors.Is(context.Cause(t.request), errFirstChunkTimeout):
			outcome = "first_chunk_timeout"
		case err != nil:
			var networkError net.Error
			if errors.As(err, &networkError) && networkError.Timeout() {
				outcome = "network_timeout"
			}
		}
		t.mu.Lock()
		now := observedAt
		durations := make(map[string]float64, len(t.phases))
		incomplete := make(map[string]bool)
		for name, p := range t.phases {
			if p.start.After(now) {
				continue
			}
			end := p.end
			if end.IsZero() || end.After(now) {
				end = now
				incomplete[name] = true
			}
			durations[name] = float64(end.Sub(p.start)) / float64(time.Millisecond)
		}
		record := map[string]any{"channel_id": t.channelID, "attempt": t.attempt, "stream": t.stream,
			"status": t.status, "outcome": outcome, "phase": t.phase, "connection_reused": t.reused,
			"elapsed_ms": float64(now.Sub(t.started)) / float64(time.Millisecond), "durations_ms": durations, "incomplete_phases": incomplete}
		t.mu.Unlock()
		// No URLs, addresses, headers, bodies, or raw errors enter this log.
		if data, marshalErr := common.Marshal(record); marshalErr == nil {
			logger.LogInfo(t.ctx, "upstream timing: "+string(data))
		}
	})
}

type upstreamTimingBody struct {
	io.ReadCloser
	timing *upstreamTiming
}

func (b *upstreamTimingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	outcome := ""
	if n > 0 {
		outcome = "first_chunk"
	} else if err != nil {
		outcome = "body_read_error"
		if errors.Is(err, io.EOF) {
			outcome = "empty_body"
		}
	}
	if outcome != "" {
		b.timing.bodyOnce.Do(func() {
			b.timing.mu.Lock()
			b.timing.bodyOutcome, b.timing.bodyAt = outcome, time.Now()
			if outcome == "first_chunk" || outcome == "empty_body" {
				phase := b.timing.phases["first_body_wait"]
				phase.end = b.timing.bodyAt
				b.timing.phases["first_body_wait"] = phase
			}
			b.timing.mu.Unlock()
			// With TTFT enabled, the caller logs only after disarming its timer:
			// slow log I/O must never turn a timely first byte into a timeout.
			if !b.timing.deferLog {
				b.timing.finish(outcome, err)
			}
		})
	}
	return n, err
}

func (b *upstreamTimingBody) Close() error {
	err := b.ReadCloser.Close()
	if !b.timing.deferLog {
		b.timing.finish("closed_before_first_chunk", err)
	}
	return err
}
