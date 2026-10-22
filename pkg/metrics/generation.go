package metrics

import (
	"strconv"
	"sync"
	"time"
)

const GenerationContextKey = "prometheus_generation"
const GenerationStartContextKey = "prometheus_request_started"

// Generation tracks a client request across provider retries. It stores times and
// counts only; no request content, credentials, URLs or model names are retained.
type Generation struct {
	mu          sync.Mutex
	monitor     *Metrics
	started     time.Time
	first, last time.Time
	stream      bool
	tokens      int64
	outcome     string
	finished    bool
	classified  bool
	attempt     *ProviderAttempt
}

type ProviderAttempt struct {
	mu                     sync.Mutex
	generation             *Generation
	channelID, channelType int
	started, first, last   time.Time
	stream                 bool
	status                 int
	finished               bool
}

func (m *Metrics) NewGeneration(started time.Time) *Generation {
	return &Generation{monitor: m, started: started, outcome: "failure"}
}

func (g *Generation) BeginAttempt(channelID, channelType int, stream bool, started time.Time) *ProviderAttempt {
	a := &ProviderAttempt{generation: g, channelID: channelID, channelType: channelType, stream: stream, started: started}
	g.mu.Lock()
	g.attempt = a
	g.stream = stream
	g.mu.Unlock()
	return a
}

// ObserveProviderPayload records wire arrival only for recognized generation payloads.
func (g *Generation) ObserveProviderPayload(data string, at time.Time) {
	if g == nil || !HasGenerationContent(data) {
		return
	}
	g.ObserveProviderContent(at)
}

// ObserveProviderContent is for adaptors that already validated a native content delta.
func (g *Generation) ObserveProviderContent(at time.Time) {
	if g == nil {
		return
	}
	g.mu.Lock()
	a := g.attempt
	g.mu.Unlock()
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.finished {
		return
	}
	if a.first.IsZero() {
		a.first = at
	}
	a.stream = true
	a.last = at
}

func (g *Generation) SiteTTFT() (time.Duration, bool) {
	if g == nil {
		return 0, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.stream || g.first.IsZero() || g.first.Before(g.started) {
		return 0, false
	}
	return g.first.Sub(g.started), true
}

// ObserveSitePayload is called after a successful write/flush, never for pings.
func (g *Generation) ObserveSitePayload(data string, at time.Time) {
	if g == nil || !HasGenerationContent(data) {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.finished {
		return
	}
	if g.first.IsZero() {
		g.first = at
	}
	g.last = at
}

func (a *ProviderAttempt) SetStatus(status int) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.status == 0 {
		a.status = status
	}
	a.mu.Unlock()
}

func (g *Generation) SetProviderStatus(status int) {
	if g == nil {
		return
	}
	g.mu.Lock()
	a := g.attempt
	g.mu.Unlock()
	if a != nil {
		a.mu.Lock()
		a.status = status
		a.mu.Unlock()
	}
}

func (a *ProviderAttempt) Finish(tokens int64, outcome string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.finished {
		a.mu.Unlock()
		return
	}
	a.finished = true
	switch outcome {
	case "success", "failure", "ignored":
	default:
		outcome = "failure"
	}
	a.generation.monitor.observeGeneration("provider", a.channelID, a.channelType, a.stream, a.started, a.first, a.last, tokens, a.status, outcome)
	a.mu.Unlock()
	a.generation.mu.Lock()
	a.generation.tokens = tokens
	if !a.generation.classified {
		a.generation.outcome = outcome
	}
	a.generation.mu.Unlock()
}

func (g *Generation) SetOutcome(outcome string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.outcome = outcome
	g.classified = true
	g.mu.Unlock()
}

func (g *Generation) FinishSite(status int) {
	if g == nil {
		return
	}
	g.mu.Lock()
	a := g.attempt
	g.mu.Unlock()
	a.Finish(0, "failure")
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.finished {
		return
	}
	g.finished = true
	g.monitor.observeGeneration("site", 0, 0, g.stream, g.started, g.first, g.last, g.tokens, status, g.outcome)
}

func (m *Metrics) observeGeneration(scope string, channelID, channelType int, stream bool, started, first, last time.Time, tokens int64, status int, outcome string) {
	channel, provider := strconv.Itoa(channelID), strconv.Itoa(channelType)
	statusLabel := "none"
	if status >= 100 && status <= 599 {
		statusLabel = strconv.Itoa(status)
	}
	m.generationRequests.WithLabelValues(scope, channel, provider, strconv.FormatBool(stream), statusLabel, outcome).Inc()
	if tokens > 0 {
		m.generationTokens.WithLabelValues(scope, channel, provider, outcome).Add(float64(tokens))
	}
	if stream && !first.IsZero() && !first.Before(started) {
		m.generationTTFT.WithLabelValues(scope, channel, provider).Observe(first.Sub(started).Seconds())
	}
	reason := "measured"
	switch {
	case !stream:
		reason = "non_stream"
	case outcome != "success":
		reason = "failed"
	case first.IsZero():
		reason = "no_content"
	case tokens <= 1:
		reason = "no_usage"
	case !last.After(first):
		reason = "insufficient_content_timing"
	default:
		interval := last.Sub(first).Seconds() / float64(tokens-1)
		m.generationTPOT.WithLabelValues(scope, channel, provider).Observe(interval)
		m.generationTPS.WithLabelValues(scope, channel, provider).Observe(1 / interval)
	}
	m.generationSamples.WithLabelValues(scope, channel, provider, reason).Inc()
}
