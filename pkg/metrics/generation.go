package metrics

import (
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
)

const GenerationContextKey = "prometheus_generation"
const GenerationStartContextKey = "prometheus_request_started"

// Generation tracks a client request across provider retries. It stores times and
// counts plus the requested model and numeric tenant ID; no content or credentials are retained.
type Generation struct {
	mu                          sync.Mutex
	monitor                     *Metrics
	model, tenantID             string
	started                     time.Time
	first, last                 time.Time
	stream                      bool
	tokens                      int64
	outcome                     string
	finished                    bool
	classified                  bool
	attempt                     *ProviderAttempt
	promptUsage                 promptCacheUsage
	metricModel, metricTenantID string
	inFlight                    prometheus.Gauge
}

type ProviderAttempt struct {
	mu                          sync.Mutex
	generation                  *Generation
	channelID, channelType      int
	started, first, last        time.Time
	stream                      bool
	status                      int
	finished                    bool
	promptUsage                 promptCacheUsage
	metricModel, metricTenantID string
	inFlight                    prometheus.Gauge
}

func (m *Metrics) NewGeneration(started time.Time) *Generation {
	return m.NewGenerationForTenant(started, "", 0, false)
}

// NewGenerationForTenant uses the client-requested model, before channel mapping.
func (m *Metrics) NewGenerationForTenant(started time.Time, model string, tenantID int, stream bool) *Generation {
	if model == "" || len(model) > 256 || !utf8.ValidString(model) {
		model = "unknown"
	}
	tenant := "unknown"
	if tenantID > 0 {
		tenant = strconv.Itoa(tenantID)
	}
	metricModel, metricTenant := m.generationLabels(model, tenant, 0, 0)
	labels := []string{"site", "0", "0", metricModel, metricTenant}
	inFlight := m.generationInFlight.WithLabelValues(labels...)
	inFlight.Inc()
	m.generationStarted.WithLabelValues(labels...).Inc()
	return &Generation{monitor: m, started: started, outcome: "failure", model: model, tenantID: tenant, stream: stream,
		metricModel: metricModel, metricTenantID: metricTenant, inFlight: inFlight}
}

type generationDimensions struct {
	model, tenantID        string
	channelID, channelType int
}

// A process-wide budget covers the full cross product, not just distinct model names.
func (m *Metrics) generationLabels(model, tenantID string, channelID, channelType int) (string, string) {
	key := generationDimensions{model: model, tenantID: tenantID, channelID: channelID, channelType: channelType}
	m.dimensionsMu.Lock()
	if _, found := m.dimensions[key]; found {
		m.dimensionsMu.Unlock()
		return model, tenantID
	}
	if len(m.dimensions) < m.dimensionLimit {
		m.dimensions[key] = struct{}{}
		m.dimensionCount.Inc()
		m.dimensionsMu.Unlock()
		return model, tenantID
	}
	m.dimensionsMu.Unlock()
	m.dimensionOverflow.Inc()
	return "__other__", "__other__"
}

func (g *Generation) BeginAttempt(channelID, channelType int, stream bool, started time.Time) *ProviderAttempt {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.finished {
		return nil
	}
	model, tenant := g.monitor.generationLabels(g.model, g.tenantID, channelID, channelType)
	labels := []string{"provider", strconv.Itoa(channelID), strconv.Itoa(channelType), model, tenant}
	inFlight := g.monitor.generationInFlight.WithLabelValues(labels...)
	inFlight.Inc()
	g.monitor.generationStarted.WithLabelValues(labels...).Inc()
	a := &ProviderAttempt{generation: g, channelID: channelID, channelType: channelType, stream: stream, started: started,
		metricModel: model, metricTenantID: tenant, inFlight: inFlight}
	g.attempt = a
	g.stream = stream
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

type promptCacheUsage struct {
	input, cached int64
	source        string
}

// SetPromptCacheUsage stores one usage sample on the active attempt. Recording at
// Finish makes repeated settlement notifications idempotent and keeps retries distinct.
func (g *Generation) SetPromptCacheUsage(input, cached int64, source string) {
	if g == nil {
		return
	}
	if source != "reported" && source != "estimated" && source != "missing" {
		source = "invalid"
	}
	if input < 0 || cached < 0 || cached > input {
		source = "invalid"
	}
	g.mu.Lock()
	a := g.attempt
	g.mu.Unlock()
	if a == nil {
		return
	}
	a.mu.Lock()
	if !a.finished {
		a.promptUsage = promptCacheUsage{input: input, cached: cached, source: source}
	}
	a.mu.Unlock()
}

func (m *Metrics) observePromptCache(model, tenantID, scope string, channelID, channelType int, usage promptCacheUsage) {
	labels := []string{scope, strconv.Itoa(channelID), strconv.Itoa(channelType), model, tenantID}
	source := usage.source
	if source == "" {
		source = "missing"
	}
	m.generationCacheUsage.WithLabelValues(append(labels, source)...).Inc()
	if source != "reported" {
		return
	}
	m.generationInput.WithLabelValues(labels...).Add(float64(usage.input))
	m.generationCachedInput.WithLabelValues(labels...).Add(float64(usage.cached))
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
	a.inFlight.Dec()
	switch outcome {
	case "success", "failure", "ignored":
	default:
		outcome = "failure"
	}
	a.generation.monitor.observeGeneration(a.metricModel, a.metricTenantID, "provider", a.channelID, a.channelType, a.stream, a.started, a.first, a.last, tokens, a.status, outcome, a.promptUsage)
	usage := a.promptUsage
	a.mu.Unlock()
	a.generation.mu.Lock()
	a.generation.promptUsage = usage
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
	g.inFlight.Dec()
	g.monitor.observeGeneration(g.metricModel, g.metricTenantID, "site", 0, 0, g.stream, g.started, g.first, g.last, g.tokens, status, g.outcome, g.promptUsage)
}

func (m *Metrics) observeGeneration(model, tenantID, scope string, channelID, channelType int, stream bool, started, first, last time.Time, tokens int64, status int, outcome string, usage promptCacheUsage) {
	m.observePromptCache(model, tenantID, scope, channelID, channelType, usage)
	channel, provider := strconv.Itoa(channelID), strconv.Itoa(channelType)
	labels := []string{scope, channel, provider, model, tenantID}
	if usage.source == "reported" || usage.source == "estimated" {
		m.generationUsageTokens.WithLabelValues(append(labels, "input")...).Add(float64(usage.input))
	}
	if tokens > 0 {
		m.generationUsageTokens.WithLabelValues(append(labels, "output")...).Add(float64(tokens))
	}
	statusLabel := "none"
	if status >= 100 && status <= 599 {
		statusLabel = strconv.Itoa(status)
	}
	m.generationRequests.WithLabelValues(scope, channel, provider, model, tenantID, strconv.FormatBool(stream), statusLabel, outcome).Inc()
	if tokens > 0 {
		m.generationTokens.WithLabelValues(scope, channel, provider, model, tenantID, outcome).Add(float64(tokens))
	}
	if stream && !first.IsZero() && !first.Before(started) {
		m.generationTTFT.WithLabelValues(scope, channel, provider, model, tenantID).Observe(first.Sub(started).Seconds())
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
		m.generationTPOT.WithLabelValues(scope, channel, provider, model, tenantID).Observe(interval)
		m.generationTPS.WithLabelValues(scope, channel, provider, model, tenantID).Observe(1 / interval)
	}
	m.generationSamples.WithLabelValues(scope, channel, provider, model, tenantID, reason).Inc()
}
