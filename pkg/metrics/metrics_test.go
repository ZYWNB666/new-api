package metrics_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"

	"github.com/QuantumNous/new-api/pkg/metrics"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	clientmodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scrapeMetrics(t *testing.T, handler http.Handler) map[string]*clientmodel.MetricFamily {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Header().Get("Content-Type"), "text/plain")
	var parser expfmt.TextParser
	families, err := parser.TextToMetricFamilies(response.Body)
	require.NoError(t, err)
	return families
}

func TestHTTPMetrics(t *testing.T) {
	monitor := metrics.New()
	handler := monitor.Handler()
	engine := gin.New()
	engine.Use(monitor.Middleware(), gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	engine.GET("/items/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	engine.GET("/abort", func(c *gin.Context) { c.AbortWithStatus(http.StatusTooManyRequests) })
	engine.GET("/panic", func(_ *gin.Context) { panic("test") })
	engine.GET("/stream", func(c *gin.Context) {
		families := scrapeMetrics(t, handler)
		assert.Equal(t, float64(1), families["new_api_http_requests_in_flight"].Metric[0].GetGauge().GetValue())
		c.Header("Content-Type", "text/event-stream")
		_, err := c.Writer.WriteString("data: hello\n\n")
		require.NoError(t, err)
		c.Writer.Flush()
	})
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/items/private-id?key=private-key", 204},
		{http.MethodGet, "/items/another-id", 204},
		{http.MethodGet, "/abort", 429},
		{http.MethodGet, "/panic", 500},
		{http.MethodGet, "/stream", 200},
		{http.MethodGet, "/unknown/private-path", 404},
		{"CUSTOM-PRIVATE", "/unknown/another-path", 404},
	} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		assert.Equal(t, tc.status, response.Code)
		if tc.path == "/stream" {
			assert.Equal(t, "data: hello\n\n", response.Body.String())
			assert.True(t, response.Flushed)
		}
	}
	families := scrapeMetrics(t, handler)
	require.Contains(t, families, "go_goroutines")
	require.Contains(t, families, "process_cpu_seconds_total")
	assert.Zero(t, families["new_api_http_requests_in_flight"].Metric[0].GetGauge().GetValue())
	counts := map[string]float64{}
	for _, metric := range families["new_api_http_requests_total"].Metric {
		labels := map[string]string{}
		for _, label := range metric.Label {
			labels[label.GetName()] = label.GetValue()
		}
		counts[labels["route"]+" "+labels["method"]+" "+labels["status"]] = metric.GetCounter().GetValue()
	}
	assert.Equal(t, map[string]float64{
		"/items/:id GET 204": 2, "/abort GET 429": 1, "/panic GET 500": 1,
		"/stream GET 200": 1, "unmatched GET 404": 1, "unmatched OTHER 404": 1,
	}, counts)
	var completed uint64
	for _, metric := range families["new_api_http_request_duration_seconds"].Metric {
		completed += metric.GetHistogram().GetSampleCount()
	}
	assert.Equal(t, uint64(7), completed)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/debug/pprof/", 404},
		{http.MethodGet, "/api/status", 404},
		{http.MethodPost, "/metrics", 405},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		assert.Equal(t, tc.status, response.Code)
	}
}

func TestTokenCountingAndRelayInstrumentation(t *testing.T) {
	service.InitTokenEncoders()
	const text = "hello world"
	const before, estimate = 2, 3
	monitor := metrics.Enable()
	for _, model := range []string{"gpt-4", "gpt-4o", "gpt-custom-private-model"} {
		assert.Equal(t, before, service.CountTextToken(text, model))
	}
	assert.Equal(t, estimate, service.CountTextToken(text, "claude-test"))
	assert.Equal(t, estimate, service.EstimateTokenByModel("claude-test", text))
	assert.Zero(t, service.CountTextToken("", "gpt-4"))

	start := time.Now().Add(-time.Second)
	info := &relaycommon.RelayInfo{StartTime: start, FirstResponseTime: start.Add(250 * time.Millisecond), IsStream: true, RetryIndex: 2}
	info.GenerationMetrics = monitor.NewGeneration(start)
	info.GenerationMetrics.BeginAttempt(1, 1, true, start)
	info.GenerationMetrics.ObserveSitePayload(`{"choices":[{"delta":{"content":"hello"}}]}`, start.Add(250*time.Millisecond))
	// An empty model skips the internal dashboard's storage; Prometheus remains independent of it.
	perfmetrics.RecordRelayResult(context.Background(), info, nil)
	perfmetrics.RecordRelayResult(context.Background(), &relaycommon.RelayInfo{}, types.InitOpenAIError("server_error", 500))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	perfmetrics.RecordRelayResult(canceled, info, nil)
	perfmetrics.RecordRelayResult(context.Background(), nil, nil)

	families := scrapeMetrics(t, monitor.Handler())
	calls := map[string]uint64{}
	for _, metric := range families["new_api_tokenizer_duration_seconds"].Metric {
		for _, label := range metric.Label {
			if label.GetName() == "encoding" {
				calls[label.GetValue()] = metric.GetHistogram().GetSampleCount()
			}
		}
	}
	assert.Equal(t, map[string]uint64{"cl100k_base": 2, "o200k_base": 1, "estimate": 2}, calls)
	for _, metric := range families["new_api_tokenizer_input_bytes_total"].Metric {
		want := 2 * len(text)
		if metric.Label[0].GetValue() == "o200k_base" {
			want = len(text)
		}
		assert.Equal(t, float64(want), metric.GetCounter().GetValue())
	}
	for _, metric := range families["new_api_tokenizer_tokens_total"].Metric {
		want := 2 * before
		switch metric.Label[0].GetValue() {
		case "o200k_base":
			want = before
		case "estimate":
			want = 2 * estimate
		}
		assert.Equal(t, float64(want), metric.GetCounter().GetValue())
	}
	assert.Zero(t, families["new_api_tokenizer_calls_in_flight"].Metric[0].GetGauge().GetValue())
	outcomes := map[string]float64{}
	for _, metric := range families["new_api_relay_requests_total"].Metric {
		outcomes[metric.Label[0].GetValue()+" "+metric.Label[1].GetValue()] = metric.GetCounter().GetValue()
	}
	assert.Equal(t, map[string]float64{"success true": 1, "failure false": 1, "ignored true": 1}, outcomes)
	ttft := families["new_api_relay_time_to_first_token_seconds"].Metric[0].GetHistogram()
	assert.Equal(t, uint64(1), ttft.GetSampleCount())
	assert.Equal(t, 0.25, ttft.GetSampleSum())
	retries := families["new_api_relay_retry_rounds"].Metric[0].GetHistogram()
	assert.Equal(t, uint64(3), retries.GetSampleCount())
	assert.Equal(t, float64(4), retries.GetSampleSum())
	streamEnds := families["new_api_relay_stream_ends_total"].Metric
	require.Len(t, streamEnds, 1)
	assert.Equal(t, "unknown", streamEnds[0].Label[0].GetValue())
	assert.Equal(t, float64(2), streamEnds[0].GetCounter().GetValue())
}

func TestTokenizerErrorMetrics(t *testing.T) {
	monitor := metrics.New()
	started := monitor.StartTokenCount()
	families := scrapeMetrics(t, monitor.Handler())
	assert.Equal(t, float64(1), families["new_api_tokenizer_calls_in_flight"].Metric[0].GetGauge().GetValue())
	monitor.FinishTokenCount("cl100k_base", started, 12, 3, errors.New("count failed"))
	families = scrapeMetrics(t, monitor.Handler())
	assert.Zero(t, families["new_api_tokenizer_calls_in_flight"].Metric[0].GetGauge().GetValue())
	require.Len(t, families["new_api_tokenizer_duration_seconds"].Metric, 1)
	metric := families["new_api_tokenizer_duration_seconds"].Metric[0]
	assert.Equal(t, "error", metric.Label[1].GetValue())
	assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
	assert.NotContains(t, families, "new_api_tokenizer_tokens_total")
}

type failedRoundTripper struct {
	err error
}

func (r failedRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, r.err
}

func TestUpstreamHTTPMetrics(t *testing.T) {
	monitor := metrics.New()
	var sawInFlight atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := httptest.NewRecorder()
		monitor.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		sawInFlight.Store(strings.Contains(response.Body.String(), "new_api_upstream_http_requests_in_flight 1\n"))
		if r.URL.Path == "/limited" {
			w.WriteHeader(http.StatusTooManyRequests)
		}
		_, _ = io.WriteString(w, "upstream response")
	}))
	t.Cleanup(server.Close)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	var existingTraceCalls atomic.Int32
	for _, tc := range []struct {
		path   string
		status int
	}{{"/ok", 200}, {"/limited", 429}} {
		req, err := http.NewRequest(http.MethodGet, server.URL+tc.path, nil)
		require.NoError(t, err)
		ctx := httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
			GotConn: func(_ httptrace.GotConnInfo) { existingTraceCalls.Add(1) },
		})
		resp, err := monitor.DoUpstream(client, req.WithContext(ctx))
		require.NoError(t, err)
		assert.Equal(t, tc.status, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		assert.Equal(t, "upstream response", string(body))
	}
	assert.True(t, sawInFlight.Load())
	assert.Equal(t, int32(2), existingTraceCalls.Load(), "existing HTTP trace hooks must be preserved")
	for _, failure := range []error{
		context.Canceled,
		context.DeadlineExceeded,
		errors.New("private-upstream-host"),
	} {
		req, err := http.NewRequest(http.MethodGet, "https://private-upstream-host/path", nil)
		require.NoError(t, err)
		_, err = monitor.DoUpstream(&http.Client{Transport: failedRoundTripper{err: failure}}, req)
		require.ErrorIs(t, err, failure)
	}
	families := scrapeMetrics(t, monitor.Handler())
	assert.Zero(t, families["new_api_upstream_http_requests_in_flight"].Metric[0].GetGauge().GetValue())
	results := map[string]float64{}
	for _, metric := range families["new_api_upstream_http_requests_total"].Metric {
		labels := map[string]string{}
		for _, label := range metric.Label {
			labels[label.GetName()] = label.GetValue()
		}
		results[labels["error"]+" "+labels["status"]] = metric.GetCounter().GetValue()
	}
	assert.Equal(t, map[string]float64{
		"none 200": 1, "none 429": 1, "canceled none": 1, "timeout none": 1, "transport none": 1,
	}, results)
	var calls uint64
	for _, metric := range families["new_api_upstream_response_header_duration_seconds"].Metric {
		calls += metric.GetHistogram().GetSampleCount()
	}
	assert.Equal(t, uint64(5), calls)
	connections := map[string]float64{}
	for _, metric := range families["new_api_upstream_connections_acquired_total"].Metric {
		connections[metric.Label[0].GetValue()] = metric.GetCounter().GetValue()
	}
	assert.Equal(t, map[string]float64{"false": 1, "true": 1}, connections)
	assert.Equal(t, uint64(2), families["new_api_upstream_connection_acquire_duration_seconds"].Metric[0].GetHistogram().GetSampleCount())
}

// noDialDatabase fails if a collector attempts to access a database server.
type noDialDatabase struct {
	calls atomic.Int32
}

func (d *noDialDatabase) Connect(context.Context) (driver.Conn, error) {
	d.calls.Add(1)
	return nil, errors.New("database access is not allowed during collection")
}

func (d *noDialDatabase) Open(string) (driver.Conn, error) {
	return d.Connect(context.Background())
}

func (d *noDialDatabase) Driver() driver.Driver { return d }

func TestPoolMetricsDoNotAccessServers(t *testing.T) {
	monitor := metrics.New()
	database := &noDialDatabase{}
	db := sql.OpenDB(database)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(7)
	require.NoError(t, monitor.RegisterDatabase("main", db))
	var redisDials atomic.Int32
	rdb := redis.NewClient(&redis.Options{
		Addr: "private-redis:6379",
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			redisDials.Add(1)
			return nil, errors.New("Redis access is not allowed during collection")
		},
	})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })
	require.NoError(t, monitor.RegisterRedis(rdb))
	families := scrapeMetrics(t, monitor.Handler())
	assert.Zero(t, database.calls.Load())
	assert.Zero(t, redisDials.Load())
	maxOpen := families["go_sql_max_open_connections"].Metric[0]
	assert.Equal(t, "main", maxOpen.Label[0].GetValue())
	assert.Equal(t, float64(7), maxOpen.GetGauge().GetValue())
	assert.Zero(t, families["go_sql_open_connections"].Metric[0].GetGauge().GetValue())
	assert.Zero(t, families["go_sql_wait_count_total"].Metric[0].GetCounter().GetValue())
	assert.Zero(t, families["new_api_redis_pool_connections"].Metric[0].GetGauge().GetValue())
	assert.Zero(t, families["new_api_redis_pool_timeouts_total"].Metric[0].GetCounter().GetValue())
}

func generationMetric(t *testing.T, families map[string]*clientmodel.MetricFamily, name, scope, channel string) *clientmodel.Metric {
	t.Helper()
	require.Contains(t, families, name)
	for _, metric := range families[name].Metric {
		labels := map[string]string{}
		for _, label := range metric.Label {
			labels[label.GetName()] = label.GetValue()
		}
		if labels["scope"] == scope && labels["channel_id"] == channel {
			return metric
		}
	}
	require.FailNowf(t, "missing generation metric", "%s for scope=%s channel=%s", name, scope, channel)
	return nil
}

func TestGenerationTimingAcrossProviderRetries(t *testing.T) {
	m := metrics.New()
	start := time.Unix(1000, 0)
	g := m.NewGeneration(start)
	first := g.BeginAttempt(7, 1, true, start.Add(time.Second))
	first.SetStatus(429)
	first.Finish(0, "failure")
	first.Finish(0, "failure")
	second := g.BeginAttempt(8, 14, true, start.Add(3*time.Second))
	second.SetStatus(200)
	const content = `{"choices":[{"delta":{"content":"hello"}}]}`
	g.ObserveProviderPayload(`{"choices":[{"delta":{"role":"assistant"}}]}`, start.Add(4*time.Second))
	g.ObserveProviderPayload(content, start.Add(5*time.Second))
	g.ObserveProviderPayload(content, start.Add(7*time.Second))
	g.ObserveProviderPayload(`{"choices":[],"usage":{"completion_tokens":101}}`, start.Add(20*time.Second))
	g.ObserveSitePayload(content, start.Add(6*time.Second))
	g.ObserveSitePayload(content, start.Add(8*time.Second))
	second.Finish(101, "success")
	g.SetOutcome("success")
	g.FinishSite(200)
	g.FinishSite(200)
	families := scrapeMetrics(t, m.Handler())
	require.Len(t, families["new_api_generation_requests_total"].Metric, 3, "two provider calls and one site request")
	for _, tc := range []struct {
		scope, channel string
		ttft           float64
	}{{"provider", "8", 2}, {"site", "0", 6}} {
		metric := generationMetric(t, families, "new_api_generation_time_to_first_token_seconds", tc.scope, tc.channel).GetHistogram()
		assert.Equal(t, uint64(1), metric.GetSampleCount())
		assert.Equal(t, tc.ttft, metric.GetSampleSum())
		assert.InDelta(t, 0.02, generationMetric(t, families, "new_api_generation_time_per_output_token_seconds", tc.scope, tc.channel).GetHistogram().GetSampleSum(), 1e-12)
		assert.InDelta(t, 50, generationMetric(t, families, "new_api_generation_tokens_per_second", tc.scope, tc.channel).GetHistogram().GetSampleSum(), 1e-12)
		assert.Equal(t, float64(101), generationMetric(t, families, "new_api_generation_output_tokens_total", tc.scope, tc.channel).GetCounter().GetValue())
	}
	failed := generationMetric(t, families, "new_api_generation_requests_total", "provider", "7")
	labels := map[string]string{}
	for _, label := range failed.Label {
		labels[label.GetName()] = label.GetValue()
	}
	assert.Equal(t, "429", labels["status"])
	assert.Equal(t, "failure", labels["outcome"])
	assert.Equal(t, float64(1), failed.GetCounter().GetValue())
}

func TestGenerationSampleEligibility(t *testing.T) {
	for _, tc := range []struct {
		name            string
		stream          bool
		tokens          int64
		chunks          int
		outcome, reason string
	}{
		{"non-stream", false, 10, 0, "success", "non_stream"},
		{"control-only", true, 10, 0, "success", "no_content"},
		{"missing-usage", true, 0, 2, "success", "no_usage"},
		{"one-token", true, 1, 1, "success", "no_usage"},
		{"single-event", true, 10, 1, "success", "insufficient_content_timing"},
		{"failed-stream", true, 10, 2, "failure", "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := metrics.New()
			start := time.Unix(1000, 0)
			g := m.NewGeneration(start)
			a := g.BeginAttempt(1, 1, tc.stream, start)
			a.SetStatus(200)
			g.ObserveProviderPayload(`{"type":"response.created"}`, start.Add(time.Second))
			for i := range tc.chunks {
				g.ObserveProviderContent(start.Add(time.Duration(i+2) * time.Second))
			}
			a.Finish(tc.tokens, tc.outcome)
			families := scrapeMetrics(t, m.Handler())
			assert.NotContains(t, families, "new_api_generation_time_per_output_token_seconds")
			assert.NotContains(t, families, "new_api_generation_tokens_per_second")
			metric := generationMetric(t, families, "new_api_generation_samples_total", "provider", "1")
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			assert.Equal(t, tc.reason, labels["reason"])
		})
	}
}

func TestGenerationContentFiltering(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		content    bool
	}{
		{"role", `{"choices":[{"delta":{"role":"assistant"}}]}`, false},
		{"usage", `{"choices":[],"usage":{"completion_tokens":123}}`, false},
		{"ping", `{"type":"ping"}`, false},
		{"snapshot", `{"type":"response.completed","response":{"output":[{"content":[{"text":"hello"}]}]}}`, false},
		{"empty-delta", `{"type":"response.output_text.delta","delta":""}`, false},
		{"text", `{"choices":[{"delta":{"content":"hello"}}]}`, true},
		{"reasoning", `{"choices":[{"delta":{"reasoning_content":"thinking"}}]}`, true},
		{"tool-arguments", `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{"}}]}}]}`, true},
		{"responses", `{"type":"response.output_text.delta","delta":"hello"}`, true},
		{"claude-start", `{"type":"message_start","message":{"content":[]}}`, false},
		{"claude", `{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`, true},
		{"claude-thinking", `{"type":"content_block_delta","delta":{"thinking":"thinking"}}`, true},
		{"gemini", `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`, true},
		{"ollama", `{"done":false,"message":{"content":"hello"}}`, true},
		{"tencent", `{"Choices":[{"Delta":{"Content":"hello"}}]}`, true},
		{"invalid", "not-json", false},
		{"truncated", `{"choices":[{"delta":{"content":"hello"}}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.content, metrics.HasGenerationContent(tc.data)) })
	}
}

type nativeStatusError struct{ status int }

func (e nativeStatusError) Error() string       { return "SDK upstream error" }
func (e nativeStatusError) HTTPStatusCode() int { return e.status }

type sdkMetricsAdaptor struct {
	relaychannel.Adaptor
	err   *types.NewAPIError
	usage *dto.Usage
}

func (a *sdkMetricsAdaptor) DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error) {
	return nil, nil
}
func (a *sdkMetricsAdaptor) DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return a.usage, a.err
}

func TestSDKProviderMetricsPreserveNativeStatusAndUsage(t *testing.T) {
	m := metrics.Enable()
	for _, tc := range []struct {
		channel int
		status  string
		err     *types.NewAPIError
	}{
		{55, "200", nil},
		{56, "429", types.NewErrorWithStatusCode(nativeStatusError{429}, types.ErrorCodeAwsInvokeError, 429)},
	} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{StartTime: time.Now()}
		info.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: tc.channel, ChannelType: 33}
		original := &dto.Usage{PromptTokens: 100, CompletionTokens: 7, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 60}}
		a := &relaychannel.MeteredAdaptor{Adaptor: &sdkMetricsAdaptor{usage: original, err: tc.err}}
		resp, err := a.DoRequest(c, info, nil)
		require.NoError(t, err)
		assert.Nil(t, resp)
		usage, apiErr := a.DoResponse(c, nil, info)
		assert.Same(t, original, usage)
		assert.Same(t, tc.err, apiErr)
		if apiErr != nil {
			apiErr.StatusCode = 503
		}
		families := scrapeMetrics(t, m.Handler())
		assert.Equal(t, float64(60), generationMetric(t, families, "new_api_generation_cached_input_tokens_total", "provider", strconv.Itoa(tc.channel)).GetCounter().GetValue())
		metric := generationMetric(t, families, "new_api_generation_requests_total", "provider", strconv.Itoa(tc.channel))
		labels := map[string]string{}
		for _, label := range metric.Label {
			labels[label.GetName()] = label.GetValue()
		}
		assert.Equal(t, tc.status, labels["status"], "host remapping must not rewrite provider status")
		assert.Equal(t, float64(1), metric.GetCounter().GetValue())
	}
}

type streamingMetricsAdaptor struct {
	relaychannel.Adaptor
	endpoint       string
	roleWasIgnored bool
}

func (a *streamingMetricsAdaptor) DoRequest(c *gin.Context, _ *relaycommon.RelayInfo, _ io.Reader) (any, error) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, a.endpoint, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func (a *streamingMetricsAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	relayhelper.StreamScannerHandler(c, resp, info, func(data string, _ *relayhelper.StreamResult) {
		_ = relayhelper.StringData(c, data)
		if strings.Contains(data, `"role":"assistant"`) {
			_, observed := info.GenerationMetrics.SiteTTFT()
			a.roleWasIgnored = !observed
		}
	})
	return &dto.Usage{CompletionTokens: 11}, nil
}

func TestStreamingGenerationMetricsEndToEnd(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	m := metrics.Enable()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n"+
			"data: {\"choices\":[],\"usage\":{\"completion_tokens\":11}}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	engine := gin.New()
	engine.Use(m.Middleware())
	delegate := &streamingMetricsAdaptor{endpoint: server.URL}
	engine.POST("/generation", func(c *gin.Context) {
		info := &relaycommon.RelayInfo{StartTime: time.Now(), IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 57, ChannelType: 1}}
		a := &relaychannel.MeteredAdaptor{Adaptor: delegate}
		resp, err := a.DoRequest(c, info, nil)
		require.NoError(t, err)
		_, apiErr := a.DoResponse(c, resp.(*http.Response), info)
		require.Nil(t, apiErr)
		info.GenerationMetrics.SetOutcome("success")
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/generation", nil))
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "hello")
	assert.True(t, response.Flushed)
	assert.True(t, delegate.roleWasIgnored)
	families := scrapeMetrics(t, m.Handler())
	for _, scope := range []struct{ name, channel string }{{"provider", "57"}, {"site", "0"}} {
		for _, name := range []string{"new_api_generation_time_to_first_token_seconds", "new_api_generation_time_per_output_token_seconds", "new_api_generation_tokens_per_second"} {
			assert.Equal(t, uint64(1), generationMetric(t, families, name, scope.name, scope.channel).GetHistogram().GetSampleCount())
		}
		assert.Equal(t, float64(11), generationMetric(t, families, "new_api_generation_output_tokens_total", scope.name, scope.channel).GetCounter().GetValue())
	}
}

type failingStreamWriter struct{ http.ResponseWriter }

func (w failingStreamWriter) Write([]byte) (int, error) { return 0, errors.New("client write failed") }
func (w failingStreamWriter) Flush()                    {}

func TestFailedContentWriteDoesNotProduceSiteTTFT(t *testing.T) {
	metrics.Enable()
	m := metrics.New()
	g := m.NewGeneration(time.Now())
	g.BeginAttempt(1, 1, true, time.Now())
	c, _ := gin.CreateTestContext(failingStreamWriter{httptest.NewRecorder()})
	c.Request = httptest.NewRequest(http.MethodPost, "/generation", nil)
	c.Set(metrics.GenerationContextKey, g)
	_ = relayhelper.StringData(c, `{"choices":[{"delta":{"content":"hello"}}]}`)
	require.NotEmpty(t, c.Errors)
	_, observed := g.SiteTTFT()
	assert.False(t, observed)
}

func TestSDKHTTPRetriesExposeEachNativeStatus(t *testing.T) {
	m := metrics.New()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	t.Cleanup(server.Close)
	g := m.NewGeneration(time.Now())
	a := g.BeginAttempt(58, 33, false, time.Now())
	client := &metrics.ProviderHTTPClient{Client: server.Client(), Monitor: m, Generation: g, ChannelID: 58, ChannelType: 33}
	for range 2 {
		req, err := http.NewRequest(http.MethodPost, server.URL, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	a.Finish(0, "success")
	families := scrapeMetrics(t, m.Handler())
	statuses := map[string]float64{}
	for _, metric := range families["new_api_upstream_http_requests_total"].Metric {
		labels := map[string]string{}
		for _, label := range metric.Label {
			labels[label.GetName()] = label.GetValue()
		}
		assert.Equal(t, "58", labels["channel_id"])
		assert.Equal(t, "33", labels["channel_type"])
		statuses[labels["status"]] = metric.GetCounter().GetValue()
	}
	assert.Equal(t, map[string]float64{"429": 1, "200": 1}, statuses)
	metric := generationMetric(t, families, "new_api_generation_requests_total", "provider", "58")
	for _, label := range metric.Label {
		if label.GetName() == "status" {
			assert.Equal(t, "200", label.GetValue())
		}
	}
}

func TestGenerationMetricsSeparateRequestedModelAndTenant(t *testing.T) {
	m := metrics.New()
	start := time.Unix(1000, 0)
	const content = `{"choices":[{"delta":{"content":"hello"}}]}`
	for _, pair := range []struct {
		model  string
		tenant int
	}{{"client-model-a", 1}, {"client-model-b", 1}, {"client-model-a", 2}} {
		g := m.NewGenerationForTenant(start, pair.model, pair.tenant, true)
		a := g.BeginAttempt(7, 1, true, start)
		a.SetStatus(http.StatusOK)
		g.ObserveProviderContent(start.Add(time.Second))
		g.ObserveProviderContent(start.Add(2 * time.Second))
		g.ObserveSitePayload(content, start.Add(2*time.Second))
		g.ObserveSitePayload(content, start.Add(3*time.Second))
		a.Finish(10, "success")
		g.SetOutcome("success")
		g.FinishSite(http.StatusOK)
	}
	families := scrapeMetrics(t, m.Handler())
	for _, name := range []string{"new_api_generation_requests_total", "new_api_generation_started_total", "new_api_generation_requests_in_flight", "new_api_generation_output_tokens_total", "new_api_generation_time_to_first_token_seconds", "new_api_generation_time_per_output_token_seconds", "new_api_generation_tokens_per_second", "new_api_generation_samples_total"} {
		require.Contains(t, families, name)
		require.Len(t, families[name].Metric, 6)
		seen := map[string]bool{}
		for _, metric := range families[name].Metric {
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			seen[labels["model"]+":"+labels["tenant_id"]+":"+labels["scope"]] = true
		}
		assert.Len(t, seen, 6, "model and tenant must remain independent on both sides")
		assert.True(t, seen["client-model-b:1:site"])
		assert.True(t, seen["client-model-a:2:provider"])
	}
}

func TestGenerationDimensionBudgetPreservesTotals(t *testing.T) {
	t.Setenv("METRICS_MAX_GENERATION_LABELSETS", "1")
	m := metrics.New()
	for _, pair := range []struct {
		model  string
		tenant int
	}{{"first", 1}, {"overflow", 2}, {"another", 3}, {"first", 1}} {
		g := m.NewGenerationForTenant(time.Unix(1000, 0), pair.model, pair.tenant, false)
		g.SetOutcome("success")
		g.FinishSite(http.StatusOK)
	}
	families := scrapeMetrics(t, m.Handler())
	require.Len(t, families["new_api_generation_requests_total"].Metric, 2)
	totals := map[string]float64{}
	for _, metric := range families["new_api_generation_requests_total"].Metric {
		labels := map[string]string{}
		for _, label := range metric.Label {
			labels[label.GetName()] = label.GetValue()
		}
		totals[labels["model"]+":"+labels["tenant_id"]] = metric.GetCounter().GetValue()
	}
	assert.Equal(t, map[string]float64{"first:1": 2, "__other__:__other__": 2}, totals)
	assert.Equal(t, float64(2), families["new_api_generation_dimension_overflow_total"].Metric[0].GetCounter().GetValue())
	assert.Equal(t, float64(1), families["new_api_generation_dimension_labelsets"].Metric[0].GetGauge().GetValue())
	for _, metric := range families["new_api_generation_requests_in_flight"].Metric {
		assert.Zero(t, metric.GetGauge().GetValue(), "overflow must release the same series it acquired")
	}
}

func TestGenerationTrafficLifecycle(t *testing.T) {
	m := metrics.New()
	start := time.Unix(1000, 0)
	g := m.NewGenerationForTenant(start, "model-a", 42, true)
	other := m.NewGenerationForTenant(start, "model-b", 43, false)
	a := g.BeginAttempt(7, 1, true, start)
	families := scrapeMetrics(t, m.Handler())
	require.Len(t, families["new_api_generation_requests_in_flight"].Metric, 3)
	for _, metric := range families["new_api_generation_requests_in_flight"].Metric {
		assert.Equal(t, float64(1), metric.GetGauge().GetValue())
	}
	a.SetStatus(502)
	a.Finish(0, "failure")
	families = scrapeMetrics(t, m.Handler())
	assert.Equal(t, float64(1), generationMetric(t, families, "new_api_generation_requests_in_flight", "site", "0").GetGauge().GetValue())
	assert.Zero(t, generationMetric(t, families, "new_api_generation_requests_in_flight", "provider", "7").GetGauge().GetValue())
	b := g.BeginAttempt(8, 1, true, start)
	b.SetStatus(200)
	g.SetPromptCacheUsage(200, 100, "reported")
	b.Finish(7, "success")
	b.Finish(7, "success")
	g.FinishSite(200)
	g.FinishSite(200)
	other.FinishSite(429)
	require.Nil(t, g.BeginAttempt(9, 1, true, start), "finished requests cannot acquire new concurrent slots")
	families = scrapeMetrics(t, m.Handler())
	for _, metric := range families["new_api_generation_requests_in_flight"].Metric {
		assert.Zero(t, metric.GetGauge().GetValue())
	}
	assert.Equal(t, float64(1), generationMetric(t, families, "new_api_generation_started_total", "provider", "7").GetCounter().GetValue())
	assert.Equal(t, float64(1), generationMetric(t, families, "new_api_generation_started_total", "provider", "8").GetCounter().GetValue())
	for _, scope := range []string{"site", "provider"} {
		var total float64
		for _, metric := range families["new_api_generation_tokens_total"].Metric {
			for _, label := range metric.Label {
				if label.GetName() == "scope" && label.GetValue() == scope {
					total += metric.GetCounter().GetValue()
				}
			}
		}
		assert.Equal(t, float64(207), total, "tokens must count once per finished scope")
	}
}

func TestGenerationConcurrencyReleasedOnRequestTermination(t *testing.T) {
	for _, termination := range []string{"success", "client_cancel", "panic", "local_rejection"} {
		t.Run(termination, func(t *testing.T) {
			m := metrics.New()
			router := gin.New()
			router.Use(m.Middleware(), gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
				c.AbortWithStatus(500)
			}))
			router.POST("/generate", func(c *gin.Context) {
				g := m.NewGenerationForTenant(time.Now(), "stream-model", 42, true)
				c.Set(metrics.GenerationContextKey, g)
				if termination == "local_rejection" {
					c.AbortWithStatus(429)
					return
				}
				g.BeginAttempt(7, 1, true, time.Now())
				families := scrapeMetrics(t, m.Handler())
				assert.Equal(t, float64(1), generationMetric(t, families, "new_api_generation_requests_in_flight", "site", "0").GetGauge().GetValue())
				assert.Equal(t, float64(1), generationMetric(t, families, "new_api_generation_requests_in_flight", "provider", "7").GetGauge().GetValue())
				switch termination {
				case "client_cancel":
					assert.ErrorIs(t, c.Request.Context().Err(), context.Canceled)
					c.Status(499)
				case "panic":
					panic("terminated")
				default:
					c.Header("Content-Type", "text/event-stream")
					_, err := c.Writer.WriteString("data: hello\n\n")
					require.NoError(t, err)
					c.Writer.Flush()
				}
			})
			request := httptest.NewRequest(http.MethodPost, "/generate", nil)
			if termination == "client_cancel" {
				ctx, cancel := context.WithCancel(request.Context())
				cancel()
				request = request.WithContext(ctx)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			status := map[string]int{"success": 200, "client_cancel": 499, "panic": 500, "local_rejection": 429}[termination]
			assert.Equal(t, status, response.Code)
			families := scrapeMetrics(t, m.Handler())
			for _, metric := range families["new_api_generation_requests_in_flight"].Metric {
				assert.Zero(t, metric.GetGauge().GetValue())
			}
			metric := generationMetric(t, families, "new_api_generation_requests_total", "site", "0")
			for _, label := range metric.Label {
				if label.GetName() == "status" {
					assert.Equal(t, strconv.Itoa(status), label.GetValue())
				}
			}
		})
	}
}

func TestLocalRejectionRetainsGenerationTenantLabels(t *testing.T) {
	monitor := metrics.Enable()
	router := gin.New()
	router.Use(monitor.Middleware())
	router.POST("/local-limit", func(c *gin.Context) {
		info := &relaycommon.RelayInfo{UserId: 4242, OriginModelName: "local-rejection-model", StartTime: time.Now(), IsStream: true}
		info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: "private-upstream-model"}
		info.InitGenerationMetrics(c)
		failure := types.NewErrorWithStatusCode(errors.New("limited"), types.ErrorCode("gateway_rate_limit"), http.StatusTooManyRequests)
		perfmetrics.RecordRelayResult(c.Request.Context(), info, failure)
		c.Status(http.StatusTooManyRequests)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/local-limit", nil))
	require.Equal(t, http.StatusTooManyRequests, response.Code)
	families := scrapeMetrics(t, monitor.Handler())
	found := false
	for _, metric := range families["new_api_generation_requests_total"].Metric {
		labels := map[string]string{}
		for _, label := range metric.Label {
			labels[label.GetName()] = label.GetValue()
		}
		if labels["model"] != "local-rejection-model" {
			continue
		}
		found = true
		assert.Equal(t, "4242", labels["tenant_id"])
		assert.Equal(t, "429", labels["status"])
		assert.Equal(t, "site", labels["scope"])
		assert.Equal(t, "ignored", labels["outcome"])
	}
	assert.True(t, found, "local rejections before provider I/O must still be attributed to the tenant")
	assert.NotContains(t, fmt.Sprint(families["new_api_generation_requests_total"]), "private-upstream-model")
}

func TestPromptCacheUsageNormalizesProviderSemantics(t *testing.T) {
	for _, tc := range []struct {
		name          string
		usage         *dto.Usage
		format        types.RelayFormat
		channel       int
		input, cached float64
		source        string
	}{
		{name: "openai", usage: &dto.Usage{PromptTokens: 100, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 60}}, input: 100, cached: 60, source: "reported"},
		{name: "responses", usage: &dto.Usage{BillingUsage: dto.NewOpenAIResponsesBillingUsage(&dto.Usage{InputTokens: 100, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 60}})}, input: 100, cached: 60, source: "reported"},
		{name: "claude snapshot", usage: &dto.Usage{PromptTokens: 999, BillingUsage: dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{InputTokens: 20, CacheReadInputTokens: 60, CacheCreationInputTokens: 20})}, input: 100, cached: 60, source: "reported"},
		{name: "claude native", usage: &dto.Usage{PromptTokens: 20, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 60, CachedCreationTokens: 20}}, format: types.RelayFormatClaude, input: 100, cached: 60, source: "reported"},
		{name: "openrouter inclusive", usage: &dto.Usage{PromptTokens: 100, UsageSemantic: "anthropic", PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 60, CachedCreationTokens: 20}}, channel: constant.ChannelTypeOpenRouter, input: 100, cached: 60, source: "reported"},
		{name: "gemini", usage: &dto.Usage{BillingUsage: dto.NewGeminiChatBillingUsage(&dto.GeminiUsageMetadata{PromptTokenCount: 100, CachedContentTokenCount: 60})}, input: 100, cached: 60, source: "reported"},
		{name: "cache miss", usage: &dto.Usage{PromptTokens: 100}, input: 100, source: "reported"},
		{name: "missing", source: "missing"},
		{name: "invalid", usage: &dto.Usage{PromptTokens: 10, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 60}}, source: "invalid"},
		{name: "negative", usage: &dto.Usage{PromptTokens: -1}, source: "invalid"},
		{name: "estimated", usage: &dto.Usage{BillingUsage: &dto.BillingUsage{Estimated: true, Source: dto.BillingUsageSourceOAIChat, OpenAIUsage: &dto.Usage{PromptTokens: 100}}}, input: 100, source: "estimated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := metrics.New()
			g := m.NewGenerationForTenant(time.Unix(1000, 0), "requested-model", 42, false)
			a := g.BeginAttempt(7, 1, false, time.Unix(1000, 0))
			info := &relaycommon.RelayInfo{GenerationMetrics: g, RelayFormat: tc.format, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: tc.channel}}
			info.ReportPromptCacheUsage(tc.usage)
			info.ReportPromptCacheUsage(tc.usage) // Settlement/adaptor notifications must count once.
			a.Finish(5, "success")
			a.Finish(5, "success")
			g.FinishSite(200)
			g.FinishSite(200)
			families := scrapeMetrics(t, m.Handler())
			for _, scope := range []string{"site", "provider"} {
				channel := "0"
				if scope == "provider" {
					channel = "7"
				}
				sample := generationMetric(t, families, "new_api_generation_cache_usage_requests_total", scope, channel)
				assert.Equal(t, float64(1), sample.GetCounter().GetValue())
				labels := map[string]string{}
				for _, l := range sample.Label {
					labels[l.GetName()] = l.GetValue()
				}
				assert.Equal(t, tc.source, labels["source"])
				assert.Equal(t, "42", labels["tenant_id"])
				assert.Equal(t, "requested-model", labels["model"])
				if tc.source == "reported" {
					assert.Equal(t, tc.input, generationMetric(t, families, "new_api_generation_input_tokens_total", scope, channel).GetCounter().GetValue())
					assert.Equal(t, tc.cached, generationMetric(t, families, "new_api_generation_cached_input_tokens_total", scope, channel).GetCounter().GetValue())
				} else {
					assert.NotContains(t, families, "new_api_generation_input_tokens_total")
				}
				var inputTotal, outputTotal float64
				for _, metric := range families["new_api_generation_tokens_total"].Metric {
					labels := map[string]string{}
					for _, label := range metric.Label {
						labels[label.GetName()] = label.GetValue()
					}
					if labels["scope"] != scope {
						continue
					}
					if labels["direction"] == "input" {
						inputTotal += metric.GetCounter().GetValue()
					} else {
						outputTotal += metric.GetCounter().GetValue()
					}
				}
				assert.Equal(t, float64(5), outputTotal)
				if tc.source == "reported" || tc.source == "estimated" {
					assert.Equal(t, tc.input, inputTotal)
				} else {
					assert.Zero(t, inputTotal)
				}
			}
		})
	}
}

func TestPromptCacheRetriesKeepProviderAttemptsAndFinalSiteUsage(t *testing.T) {
	m := metrics.New()
	g := m.NewGenerationForTenant(time.Unix(1000, 0), "model", 1, false)
	a := g.BeginAttempt(7, 1, false, time.Unix(1000, 0))
	g.SetPromptCacheUsage(100, 80, "reported")
	a.Finish(1, "failure")
	b := g.BeginAttempt(8, 1, false, time.Unix(1001, 0))
	g.SetPromptCacheUsage(200, 20, "reported")
	b.Finish(1, "success")
	g.FinishSite(200)
	families := scrapeMetrics(t, m.Handler())
	assert.Equal(t, float64(80), generationMetric(t, families, "new_api_generation_cached_input_tokens_total", "provider", "7").GetCounter().GetValue())
	assert.Equal(t, float64(20), generationMetric(t, families, "new_api_generation_cached_input_tokens_total", "provider", "8").GetCounter().GetValue())
	assert.Equal(t, float64(20), generationMetric(t, families, "new_api_generation_cached_input_tokens_total", "site", "0").GetCounter().GetValue())
}
