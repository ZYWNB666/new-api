// Package metrics exposes process-local Prometheus metrics without database queries or Redis commands.
package metrics

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var current atomic.Pointer[Metrics]

type Metrics struct {
	registry              *prometheus.Registry
	dimensionsMu          sync.Mutex
	dimensions            map[generationDimensions]struct{}
	dimensionLimit        int
	dimensionOverflow     prometheus.Counter
	dimensionCount        prometheus.Gauge
	dimensionCapacity     prometheus.Gauge
	httpRequests          *prometheus.CounterVec
	httpDuration          *prometheus.HistogramVec
	httpInFlight          prometheus.Gauge
	relayRequests         *prometheus.CounterVec
	relayTTFT             prometheus.Histogram
	relayRetries          prometheus.Histogram
	streamEnds            *prometheus.CounterVec
	upstreamRequests      *prometheus.CounterVec
	upstreamDuration      *prometheus.HistogramVec
	upstreamInFlight      prometheus.Gauge
	connectionAcquire     prometheus.Histogram
	connections           *prometheus.CounterVec
	tokenizerDuration     *prometheus.HistogramVec
	tokenizerBytes        *prometheus.CounterVec
	tokenizerTokens       *prometheus.CounterVec
	tokenizerInFlight     prometheus.Gauge
	generationRequests    *prometheus.CounterVec
	generationStarted     *prometheus.CounterVec
	generationInFlight    *prometheus.GaugeVec
	generationUsageTokens *prometheus.CounterVec
	generationTTFT        *prometheus.HistogramVec
	generationTPOT        *prometheus.HistogramVec
	generationTPS         *prometheus.HistogramVec
	generationTokens      *prometheus.CounterVec
	generationSamples     *prometheus.CounterVec
	generationInput       *prometheus.CounterVec
	generationCachedInput *prometheus.CounterVec
	generationCacheUsage  *prometheus.CounterVec
}

// Enable activates instrumentation. Call once during startup, before serving requests.
func Enable() *Metrics {
	m := New()
	if current.CompareAndSwap(nil, m) {
		return m
	}
	return current.Load()
}

// Current returns nil when Prometheus instrumentation is disabled.
func Current() *Metrics {
	return current.Load()
}

// New creates an isolated registry, including Go runtime and process collectors.
func New() *Metrics {
	limit := 2000
	if configured, err := strconv.Atoi(os.Getenv("METRICS_MAX_GENERATION_LABELSETS")); err == nil && configured > 0 && configured <= 100000 {
		limit = configured
	}
	m := &Metrics{
		dimensions:        make(map[generationDimensions]struct{}),
		dimensionLimit:    limit,
		dimensionOverflow: prometheus.NewCounter(prometheus.CounterOpts{Name: "new_api_generation_dimension_overflow_total", Help: "Generation observations aggregated into __other__ after the model/tenant/channel labelset budget is reached; request and token totals remain counted."}),
		dimensionCount:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "new_api_generation_dimension_labelsets", Help: "Admitted model/tenant/channel labelsets retained in this process."}),
		dimensionCapacity: prometheus.NewGauge(prometheus.GaugeOpts{Name: "new_api_generation_dimension_labelset_limit", Help: "Configured model/tenant/channel labelset budget per process."}),
		registry:          prometheus.NewRegistry(),
		generationRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_generation_requests_total", Help: "Finished site requests or provider adaptor calls, including HTTP status and protocol outcome. Provider calls include SDK paths; SDK-internal retries are one adaptor call.",
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id", "stream", "status", "outcome"}),
		generationStarted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_generation_started_total", Help: "Site requests entering relay processing or provider attempts starting I/O. Includes local rejections at site scope; use rate times 60 for RPM.",
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id"}),
		generationInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "new_api_generation_requests_in_flight", Help: "Active site requests or provider attempts across their full lifetime, including response bodies and streams. Retries count once at site scope; excludes management HTTP requests.",
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id"}),
		generationUsageTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_generation_tokens_total", Help: "Input and output tokens recorded when requests finish, including adaptor estimates. Input includes cache reads and writes; missing or invalid input usage is excluded. Use summed rate times 60 for TPM; not billing quantities.",
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id", "direction"}),
		generationTTFT: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "new_api_generation_time_to_first_token_seconds", Help: "Time to first nonempty text, reasoning or tool payload; provider measures receipt, site measures successful forwarding. Non-streaming responses are excluded.",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id"}),
		generationTPOT: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "new_api_generation_time_per_output_token_seconds", Help: "Request-average decoding interval: (last content time - first content time) / (output tokens - 1). Uses adaptor usage, including estimates and hidden reasoning when reported; not per-token timestamps.",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 5},
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id"}),
		generationTPS: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "new_api_generation_tokens_per_second", Help: "Request-average decoding speed: (output tokens - 1) / (last content time - first content time). Only successful streaming calls with usable content timing and usage are sampled.",
			Buckets: []float64{1, 2, 5, 10, 20, 30, 50, 75, 100, 150, 250, 500, 1000},
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id"}),
		generationTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_generation_output_tokens_total", Help: "Output tokens reported by adaptors, including local usage estimates. Use rate for aggregate output throughput; provider retries are separate, site requests count once.",
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id", "outcome"}),
		generationSamples: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_generation_samples_total", Help: "Generation speed sample eligibility: measured, non_stream, failed, no_content, no_usage, or insufficient_content_timing.",
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id", "reason"}),
		generationInput: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_generation_input_tokens_total", Help: "Total input tokens from valid adaptor usage not explicitly marked as estimated, including cache reads and cache creation. Anthropic exclusive input is normalized; not billing quantities.",
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id"}),
		generationCachedInput: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_generation_cached_input_tokens_total", Help: "Cache-read input tokens within generation_input_tokens_total. Divide summed rates to obtain token-weighted prompt cache hit ratio; cache creation is not a hit.",
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id"}),
		generationCacheUsage: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_generation_cache_usage_requests_total", Help: "Prompt cache usage sample quality: reported, estimated, missing, or invalid. Only reported samples contribute input and cached input tokens; omitted cache fields in reported usage count as zero.",
		}, []string{"scope", "channel_id", "channel_type", "model", "tenant_id", "source"}),
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_http_requests_total", Help: "Completed HTTP requests by route template, method and status code.",
		}, []string{"route", "method", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "new_api_http_request_duration_seconds", Help: "HTTP request duration, including the full lifetime of streaming requests.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
		}, []string{"route", "method"}),
		httpInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "new_api_http_requests_in_flight", Help: "HTTP requests currently being handled, including open streams and WebSockets.",
		}),
		relayRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_relay_requests_total", Help: "Finished relays by health outcome and streaming mode, including ignored business rejections and cancellations.",
		}, []string{"outcome", "stream"}),
		relayTTFT: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "new_api_relay_time_to_first_token_seconds", Help: "Time from relay start to first successfully forwarded generation content for finished, non-ignored streaming relays.",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		}),
		tokenizerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "new_api_tokenizer_duration_seconds", Help: "Local text token counting duration by encoding (estimate denotes heuristic counting) and status.",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 5},
		}, []string{"encoding", "status"}),
		tokenizerBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_tokenizer_input_bytes_total", Help: "UTF-8 input bytes processed by local text token counting.",
		}, []string{"encoding"}),
		tokenizerTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_tokenizer_tokens_total", Help: "Tokens counted locally, including repeated input and estimates; not billable usage.",
		}, []string{"encoding"}),
		tokenizerInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "new_api_tokenizer_calls_in_flight", Help: "Local text token counting calls currently executing.",
		}),
		relayRetries: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "new_api_relay_retry_rounds", Help: "Retry rounds entered per finished relay, including rounds that fail channel selection.",
			Buckets: []float64{0, 1, 2, 3, 5, 10},
		}),
		streamEnds: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_relay_stream_ends_total", Help: "Final streaming relay termination reasons; intermediate retry attempts are excluded.",
		}, []string{"reason"}),
		upstreamRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_upstream_http_requests_total", Help: "Shared upstream HTTP calls completed at response headers or transport failure, excluding body consumption.",
		}, []string{"status", "error", "channel_id", "channel_type"}),
		upstreamDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "new_api_upstream_response_header_duration_seconds", Help: "Shared upstream HTTP time until response headers or transport failure, excluding body consumption.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		}, []string{"error"}),
		upstreamInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "new_api_upstream_http_requests_in_flight", Help: "Shared upstream HTTP calls waiting for response headers; excludes open response bodies.",
		}),
		connectionAcquire: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "new_api_upstream_connection_acquire_duration_seconds", Help: "Time to obtain an upstream connection, including pool wait, DNS, dialing and TLS when applicable.",
			Buckets: prometheus.DefBuckets,
		}),
		connections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "new_api_upstream_connections_acquired_total", Help: "Upstream HTTP connection acquisitions by reuse status, including transparent transport retries.",
		}, []string{"reused"}),
	}
	m.dimensionCapacity.Set(float64(limit))
	m.registry.MustRegister(
		m.dimensionOverflow, m.dimensionCount, m.dimensionCapacity,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.httpRequests, m.httpDuration, m.httpInFlight, m.relayRequests, m.relayTTFT,
		m.relayRetries, m.streamEnds, m.upstreamRequests, m.upstreamDuration, m.upstreamInFlight,
		m.connectionAcquire, m.connections,
		m.tokenizerDuration, m.tokenizerBytes, m.tokenizerTokens, m.tokenizerInFlight,
		m.generationRequests, m.generationTTFT, m.generationTPOT, m.generationTPS, m.generationTokens, m.generationSamples, m.generationInput, m.generationCachedInput, m.generationCacheUsage,
		m.generationStarted, m.generationInFlight, m.generationUsageTokens,
	)
	return m
}

// Handler serves only GET /metrics. The dedicated listener does not expose application routes or pprof.
func (m *Metrics) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		MaxRequestsInFlight: 2,
		Timeout:             10 * time.Second,
	}))
	return mux
}

// Middleware observes requests without wrapping the writer, preserving SSE flushing and WebSocket hijacking.
// Install before recovery middleware so recovered panics are recorded with their final status.
func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Set(GenerationStartContextKey, started)
		m.httpInFlight.Inc()
		completed := false
		defer func() {
			m.httpInFlight.Dec()
			route := c.FullPath()
			if route == "" {
				route = "unmatched"
			}
			method := c.Request.Method
			switch method {
			case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace:
			default:
				method = "OTHER"
			}
			status := c.Writer.Status()
			if !completed && status < http.StatusBadRequest {
				status = http.StatusInternalServerError
			}
			m.httpRequests.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
			m.httpDuration.WithLabelValues(route, method).Observe(time.Since(started).Seconds())
			if value, ok := c.Get(GenerationContextKey); ok {
				if generation, ok := value.(*Generation); ok {
					generation.FinishSite(status)
				}
			}
		}()
		c.Next()
		completed = true
	}
}

func (m *Metrics) ObserveRelay(outcome string, stream bool, ttft time.Duration, hasTTFT bool) {
	m.relayRequests.WithLabelValues(outcome, strconv.FormatBool(stream)).Inc()
	if hasTTFT && ttft >= 0 {
		m.relayTTFT.Observe(ttft.Seconds())
	}
}

func (m *Metrics) StartTokenCount() time.Time {
	m.tokenizerInFlight.Inc()
	return time.Now()
}

func (m *Metrics) ObserveRelayDetails(retryIndex int, stream bool, endReason string) {
	m.relayRetries.Observe(float64(max(0, retryIndex)))
	if !stream {
		return
	}
	switch endReason {
	case "done", "timeout", "client_gone", "scanner_error", "handler_stop", "eof", "panic", "ping_fail":
	default:
		endReason = "unknown"
	}
	m.streamEnds.WithLabelValues(endReason).Inc()
}

func (m *Metrics) FinishTokenCount(encoding string, started time.Time, inputBytes, tokens int, err error) {
	m.tokenizerInFlight.Dec()
	status := "success"
	if err != nil {
		status = "error"
	}
	m.tokenizerDuration.WithLabelValues(encoding, status).Observe(time.Since(started).Seconds())
	m.tokenizerBytes.WithLabelValues(encoding).Add(float64(inputBytes))
	if err == nil && tokens > 0 {
		m.tokenizerTokens.WithLabelValues(encoding).Add(float64(tokens))
	}
}
