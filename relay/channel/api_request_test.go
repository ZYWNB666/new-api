package channel

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type firstChunkTransport func(*http.Request) (*http.Response, error)

func (f firstChunkTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// firstChunkFixtureBody models a response that blocks until a body byte, EOF,
// or request cancellation. synctest advances virtual time without real sleeps.
type firstChunkFixtureBody struct {
	ctx        context.Context
	delay      time.Duration
	reader     *strings.Reader
	waited     bool
	closed     bool
	beforeRead func()
}

func (b *firstChunkFixtureBody) Read(p []byte) (int, error) {
	if !b.waited {
		if b.beforeRead != nil {
			b.beforeRead()
		}
		b.waited = true
		select {
		case <-b.ctx.Done():
			return 0, b.ctx.Err()
		case <-time.After(b.delay):
		}
	}
	return b.reader.Read(p)
}

func TestDoRequestProviderResponseHeaders(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	service.InitHttpClient()
	client := service.GetHttpClient()
	previousTransport, previousTimeout := client.Transport, common.RelayTTFTTimeout
	t.Cleanup(func() { client.Transport = previousTransport; common.RelayTTFTTimeout = previousTimeout })
	common.RelayTTFTTimeout = 1
	for _, tc := range []struct {
		name        string
		enabled     string
		status      int
		timeout     bool
		manyHeaders bool
	}{
		{name: "success", enabled: "true", status: 200},
		{name: "upstream rejection", enabled: "true", status: 429},
		{name: "headers survive first chunk timeout", enabled: "true", status: 200, timeout: true},
		{name: "bounded header count", enabled: "true", status: 200, manyHeaders: true},
		{name: "disabled", enabled: "false", status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LOG_REQUEST_HEADERS", tc.enabled)
			synctest.Test(t, func(t *testing.T) {
				var output bytes.Buffer
				previousWriter := gin.DefaultWriter
				gin.DefaultWriter = &output
				t.Cleanup(func() { gin.DefaultWriter = previousWriter })
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(context.WithValue(context.Background(), common.RequestIdKey, "supplier-headers-test"))
				headers := http.Header{
					"Content-Type": {"text/event-stream"}, "X-Request-Id": {"provider-trace"},
					"Set-Cookie": {"session=do-not-log-cookie"}, "Authorization": {"do-not-log-auth"},
					"X-Api-Key": {"do-not-log-key"}, "X-Access-Token": {"do-not-log-token"},
					"Location":      {"https://provider.test/?signature=do-not-log-location"},
					"Link":          {"https://provider.test/?token=do-not-log-link"},
					"X-Debug-Value": {"first\nsecond"}, "X-Long-Value": {strings.Repeat("x", 1024)},
					"X-Multiple": {"a", "b", "c", "d", "do-not-log-extra-value"},
				}
				if tc.manyHeaders {
					for i := range 75 {
						headers[fmt.Sprintf("X-%03d", i)] = []string{"diagnostic"}
					}
				}
				original := headers.Clone()
				client.Transport = firstChunkTransport(func(r *http.Request) (*http.Response, error) {
					if trace := httptrace.ContextClientTrace(r.Context()); trace != nil && trace.WroteRequest != nil {
						trace.WroteRequest(httptrace.WroteRequestInfo{})
					}
					delay := time.Duration(0)
					if tc.timeout {
						delay = 3 * time.Second
					}
					return &http.Response{StatusCode: tc.status, Header: headers, Body: &firstChunkFixtureBody{
						ctx: r.Context(), delay: delay, reader: strings.NewReader("body"),
						beforeRead: func() {
							if tc.enabled == "true" {
								assert.Contains(t, output.String(), "provider response headers:", "response headers must be logged before reading or waiting for the body")
							}
						},
					}}, nil
				})
				info := &relaycommon.RelayInfo{IsStream: true, RelayMode: relayconstant.RelayModeChatCompletions, RetryIndex: 1, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 5}}
				req, err := http.NewRequest(http.MethodPost, "http://provider.test", strings.NewReader("{}"))
				require.NoError(t, err)
				response, err := doRequest(c, req, info)
				if tc.timeout {
					var apiError *types.NewAPIError
					require.ErrorAs(t, err, &apiError)
					assert.Equal(t, types.ErrorCodeUpstreamFirstChunkTimeout, apiError.GetErrorCode())
				} else {
					require.NoError(t, err)
					data, readErr := io.ReadAll(response.Body)
					require.NoError(t, readErr)
					assert.Equal(t, "body", string(data))
					require.NoError(t, response.Body.Close())
				}
				assert.Equal(t, original, headers, "logging must not change the provider response")
				var line string
				for entry := range strings.SplitSeq(output.String(), "\n") {
					if strings.Contains(entry, "provider response headers:") {
						line += entry + "\n"
					}
				}
				if tc.enabled != "true" {
					assert.Empty(t, line)
					return
				}
				assert.Contains(t, line, "| supplier-headers-test |")
				assert.Contains(t, line, fmt.Sprintf("channel_id=5 attempt=2 status=%d", tc.status))
				assert.NotContains(t, line, "do-not-log-")
				assert.Equal(t, 1, strings.Count(line, "\n"), "response headers cannot inject log lines")
				if tc.manyHeaders {
					assert.Contains(t, line, `"_truncated":["too many headers"]`)
				} else {
					assert.Contains(t, line, `"X-Request-Id":["provider-trace"]`)
					assert.Contains(t, line, `"X-Debug-Value":["first\nsecond"]`)
					assert.Contains(t, line, `"X-Multiple":["a","b","c","d","[TRUNCATED]"]`)
					assert.Contains(t, line, "[REDACTED]")
					assert.Contains(t, line, "[TRUNCATED]")
				}
			})
		})
	}
}

func (b *firstChunkFixtureBody) Close() error {
	b.closed = true
	return nil
}

func TestUpstreamFirstChunkTimeout(t *testing.T) {
	for _, tc := range []struct {
		name         string
		noHeaders    bool
		payload      string
		statusCode   int
		delay        time.Duration
		cancelClient bool
		networkError bool
		wantTimeout  bool
	}{
		{name: "no response headers", noHeaders: true, wantTimeout: true},
		{name: "headers without body", delay: 3 * time.Second, wantTimeout: true},
		{name: "upstream heartbeat is a chunk", payload: ": ping\n\n", delay: time.Second},
		{name: "blank line is a chunk", payload: "\n", delay: time.Second},
		{name: "role only is a chunk", payload: "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n", delay: time.Second},
		{name: "usage is a chunk", payload: "data: {\"usage\":{}}\n\n", delay: time.Second},
		{name: "done is a chunk", payload: "data: [DONE]\n\n", delay: time.Second},
		{name: "normal empty body", delay: time.Second},
		{name: "HTTP error body retains its status", statusCode: 429, payload: `{"error":"rate limited"}`, delay: time.Second},
		{name: "client cancellation", delay: 3 * time.Second, cancelClient: true},
		{name: "other network timeout", networkError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				clientCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tc.cancelClient {
					time.AfterFunc(time.Second, cancel)
				}
				req := httptest.NewRequest(http.MethodPost, "http://provider.test/v1/chat/completions", nil)
				var upstreamCtx context.Context
				var body *firstChunkFixtureBody
				send := func(r *http.Request) (*http.Response, error) {
					upstreamCtx = r.Context()
					if trace := httptrace.ContextClientTrace(r.Context()); trace != nil && trace.WroteRequest != nil {
						trace.WroteRequest(httptrace.WroteRequestInfo{})
					}
					if tc.noHeaders {
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					if tc.networkError {
						time.Sleep(time.Second)
						return nil, context.DeadlineExceeded
					}
					body = &firstChunkFixtureBody{ctx: r.Context(), delay: tc.delay, reader: strings.NewReader(tc.payload)}
					statusCode := tc.statusCode
					if statusCode == 0 {
						statusCode = http.StatusOK
					}
					return &http.Response{StatusCode: statusCode, Body: body}, nil
				}
				resp, err := awaitFirstChunk(clientCtx, req, 2*time.Second, send)
				if tc.wantTimeout {
					var apiError *types.NewAPIError
					require.ErrorAs(t, err, &apiError)
					assert.Equal(t, 499, apiError.StatusCode)
					assert.Equal(t, types.ErrorCodeUpstreamFirstChunkTimeout, apiError.GetErrorCode())
					assert.ErrorIs(t, context.Cause(upstreamCtx), errFirstChunkTimeout)
				} else if tc.cancelClient {
					require.ErrorIs(t, err, context.Canceled)
					assert.NotErrorIs(t, err, errFirstChunkTimeout)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = req.WithContext(clientCtx)
					decision := service.DecideRelayRetry(c, types.NewError(err, types.ErrorCodeDoRequestFailed), 1)
					assert.Equal(t, "client_canceled", decision.Reason)
				} else if tc.networkError {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				} else {
					require.NoError(t, err)
					if tc.statusCode != 0 {
						assert.Equal(t, tc.statusCode, resp.StatusCode)
					}
					time.Sleep(5 * time.Second)
					assert.NoError(t, upstreamCtx.Err(), "first byte disarms the timer for long generation")
					data, readErr := io.ReadAll(resp.Body)
					require.NoError(t, readErr)
					assert.Equal(t, tc.payload, string(data), "pre-read byte is preserved")
					require.NoError(t, resp.Body.Close())
					assert.ErrorIs(t, upstreamCtx.Err(), context.Canceled)
				}
				if body != nil {
					assert.True(t, body.closed)
				}
			})
		})
	}
}

func TestFirstChunkTimerIncludesConnectionAndUpload(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://provider.test", nil)
		started := time.Now()
		resp, err := awaitFirstChunk(context.Background(), req, time.Second, func(r *http.Request) (*http.Response, error) {
			// No WroteRequest event: the transport is still connecting or uploading.
			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-time.After(3 * time.Second):
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("late"))}, nil
			}
		})
		if resp != nil {
			defer resp.Body.Close()
		}
		var apiError *types.NewAPIError
		require.ErrorAs(t, err, &apiError)
		assert.Equal(t, types.ErrorCodeUpstreamFirstChunkTimeout, apiError.GetErrorCode())
		assert.Equal(t, time.Second, time.Since(started))
	})
}

func TestFirstChunkTimeoutCancelsHTTPTransport(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers bool
		http2   bool
	}{
		{name: "HTTP1 waiting for headers"},
		{name: "HTTP1 waiting for body", headers: true},
		{name: "HTTP2 waiting for headers", http2: true},
		{name: "HTTP2 waiting for body", headers: true, http2: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			canceled := make(chan struct{})
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if tc.headers {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
				close(canceled)
			}))
			server.EnableHTTP2 = tc.http2
			if tc.http2 {
				server.StartTLS()
			} else {
				server.Start()
			}
			defer server.Close()
			req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("request"))
			require.NoError(t, err)
			resp, err := awaitFirstChunk(context.Background(), req, 50*time.Millisecond, server.Client().Do)
			var apiError *types.NewAPIError
			require.ErrorAs(t, err, &apiError)
			assert.Equal(t, types.ErrorCodeUpstreamFirstChunkTimeout, apiError.GetErrorCode())
			assert.Nil(t, resp)
			select {
			case <-canceled:
			case <-time.After(5 * time.Second):
				t.Fatal("server did not observe request cancellation")
			}
		})
	}
}

func TestFirstChunkTimeoutCancelsTLSHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		// Consume ClientHello without answering: TLS must be canceled by TTFT.
		_, _ = io.Copy(io.Discard, conn)
	}()
	transport := &http.Transport{TLSHandshakeTimeout: 5 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "https://"+listener.Addr().String(), nil)
	require.NoError(t, err)
	response, err := awaitFirstChunk(context.Background(), req, 100*time.Millisecond, client.Do)
	if response != nil {
		defer response.Body.Close()
	}
	var apiError *types.NewAPIError
	require.ErrorAs(t, err, &apiError)
	assert.Equal(t, types.ErrorCodeUpstreamFirstChunkTimeout, apiError.GetErrorCode())
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("TLS connection was not closed after first chunk timeout")
	}
}

type firstChunkSlowLogWriter struct{ io.Writer }

func (w firstChunkSlowLogWriter) Write(data []byte) (int, error) {
	time.Sleep(3 * time.Second) // Virtual time inside synctest.
	return w.Writer.Write(data)
}

func TestDoRequestPhaseTimings(t *testing.T) {
	t.Setenv("LOG_REQUEST_HEADERS", "false")
	service.InitHttpClient()
	client := service.GetHttpClient()
	previousTransport, previousTimeout := client.Transport, common.RelayTTFTTimeout
	t.Cleanup(func() { client.Transport = previousTransport; common.RelayTTFTTimeout = previousTimeout })
	common.RelayTTFTTimeout = 10
	for _, tc := range []struct {
		name, failure string
		reused        bool
		slowLog       bool
	}{
		{name: "fresh connection"}, {name: "reused connection", reused: true},
		{name: "slow log does not consume TTFT budget", reused: true, slowLog: true},
		{name: "first chunk timeout during TLS", failure: "first_chunk_timeout"},
		{name: "client cancellation during TLS", failure: "client_canceled"},
		{name: "network timeout during TLS", failure: "network_timeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var output bytes.Buffer
				previousWriter := gin.DefaultWriter
				gin.DefaultWriter = &output
				if tc.slowLog {
					gin.DefaultWriter = firstChunkSlowLogWriter{Writer: &output}
				}
				t.Cleanup(func() { gin.DefaultWriter = previousWriter })
				client.Transport = firstChunkTransport(func(r *http.Request) (*http.Response, error) {
					trace := httptrace.ContextClientTrace(r.Context())
					trace.GetConn("do-not-log-host")
					if !tc.reused {
						trace.DNSStart(httptrace.DNSStartInfo{})
						time.Sleep(100 * time.Millisecond)
						trace.DNSDone(httptrace.DNSDoneInfo{})
						trace.ConnectStart("tcp", "do-not-log-address")
						time.Sleep(200 * time.Millisecond)
						trace.ConnectDone("tcp", "do-not-log-address", nil)
						trace.TLSHandshakeStart()
						if tc.failure != "" {
							if tc.failure == "network_timeout" {
								return nil, context.DeadlineExceeded
							}
							<-r.Context().Done()
							trace.TLSHandshakeDone(tls.ConnectionState{}, r.Context().Err())
							return nil, r.Context().Err()
						}
						time.Sleep(300 * time.Millisecond)
						trace.TLSHandshakeDone(tls.ConnectionState{}, nil)
					}
					trace.GotConn(httptrace.GotConnInfo{Reused: tc.reused})
					time.Sleep(400 * time.Millisecond)
					trace.WroteRequest(httptrace.WroteRequestInfo{})
					time.Sleep(500 * time.Millisecond)
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &firstChunkFixtureBody{
						ctx: r.Context(), delay: 600 * time.Millisecond, reader: strings.NewReader("do-not-log-body"),
					}}, nil
				})
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				common.RelayTTFTTimeout = 10
				if tc.slowLog {
					common.RelayTTFTTimeout = 2
				}
				if tc.failure == "first_chunk_timeout" {
					common.RelayTTFTTimeout = 1
				}
				if tc.failure == "client_canceled" {
					ctx, cancel := context.WithCancel(c.Request.Context())
					defer cancel()
					c.Request = c.Request.WithContext(ctx)
					time.AfterFunc(time.Second, cancel)
				}
				req := httptest.NewRequest(http.MethodPost, "https://provider.test?key=do-not-log-query", nil)
				req.RequestURI = ""
				info := &relaycommon.RelayInfo{IsStream: true, RelayMode: relayconstant.RelayModeChatCompletions, RetryIndex: 2, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 7}}
				response, err := doRequest(c, req, info)
				if tc.failure != "" {
					require.Error(t, err)
					assert.Nil(t, response)
					var timingLine string
					for line := range strings.SplitSeq(output.String(), "\n") {
						if strings.Contains(line, "upstream timing:") {
							timingLine = line
						}
					}
					require.NotEmpty(t, timingLine)
					assert.NotContains(t, timingLine, "do-not-log-")
					_, raw, ok := strings.Cut(timingLine, "upstream timing: ")
					require.True(t, ok)
					var record map[string]any
					require.NoError(t, common.UnmarshalJsonStr(raw, &record))
					assert.Equal(t, tc.failure, record["outcome"])
					assert.Equal(t, "tls_handshake", record["phase"])
					assert.Equal(t, 7.0, record["channel_id"])
					assert.Equal(t, 3.0, record["attempt"])
					return
				}
				require.NoError(t, err)
				data, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				assert.Equal(t, "do-not-log-body", string(data))
				require.NoError(t, response.Body.Close())
				line := output.String()
				require.Equal(t, 1, strings.Count(line, "upstream timing:"))
				assert.NotContains(t, line, "do-not-log-")
				_, raw, ok := strings.Cut(line, "upstream timing: ")
				require.True(t, ok)
				var record struct {
					ChannelID  int                `json:"channel_id"`
					Attempt    int                `json:"attempt"`
					Outcome    string             `json:"outcome"`
					Reused     bool               `json:"connection_reused"`
					Durations  map[string]float64 `json:"durations_ms"`
					Incomplete map[string]bool    `json:"incomplete_phases"`
				}
				require.NoError(t, common.UnmarshalJsonStr(strings.TrimSpace(raw), &record))
				assert.Equal(t, 7, record.ChannelID)
				assert.Equal(t, 3, record.Attempt)
				assert.Equal(t, "first_chunk", record.Outcome)
				assert.Equal(t, tc.reused, record.Reused)
				assert.Empty(t, record.Incomplete)
				assert.Equal(t, 400.0, record.Durations["request_write"])
				assert.Equal(t, 500.0, record.Durations["response_headers"])
				assert.Equal(t, 600.0, record.Durations["first_body_wait"])
				if tc.reused {
					assert.NotContains(t, record.Durations, "tls_handshake")
					assert.NotContains(t, record.Durations, "dns")
				} else {
					assert.Equal(t, 100.0, record.Durations["dns"])
					assert.Equal(t, 200.0, record.Durations["tcp_connect"])
					assert.Equal(t, 300.0, record.Durations["tls_handshake"])
					assert.Equal(t, 600.0, record.Durations["connection_acquire"])
				}
			})
		})
	}
}

func TestFirstChunkAndTimeoutAtSameDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://provider.test", nil)
		var upstreamCtx context.Context
		resp, err := awaitFirstChunk(context.Background(), req, time.Second, func(r *http.Request) (*http.Response, error) {
			upstreamCtx = r.Context()
			if trace := httptrace.ContextClientTrace(r.Context()); trace != nil && trace.WroteRequest != nil {
				trace.WroteRequest(httptrace.WroteRequestInfo{})
			}
			return &http.Response{StatusCode: 200, Body: &firstChunkFixtureBody{ctx: r.Context(), delay: time.Second, reader: strings.NewReader("first")}}, nil
		})
		if err != nil {
			var apiError *types.NewAPIError
			require.ErrorAs(t, err, &apiError)
			assert.Equal(t, types.ErrorCodeUpstreamFirstChunkTimeout, apiError.GetErrorCode())
			assert.Nil(t, resp)
			return
		}
		time.Sleep(2 * time.Second)
		assert.NoError(t, upstreamCtx.Err(), "a completed first chunk must not be canceled by a losing timer")
		require.NoError(t, resp.Body.Close())
	})
}

func TestFirstChunkRespectsExistingClientTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := &http.Client{
			Timeout: time.Second,
			Transport: firstChunkTransport(func(r *http.Request) (*http.Response, error) {
				if trace := httptrace.ContextClientTrace(r.Context()); trace != nil && trace.WroteRequest != nil {
					trace.WroteRequest(httptrace.WroteRequestInfo{})
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &firstChunkFixtureBody{
					ctx: r.Context(), delay: 10 * time.Second, reader: strings.NewReader("late"),
				}}, nil
			}),
		}
		req, err := http.NewRequest(http.MethodPost, "http://provider.test", nil)
		require.NoError(t, err)
		resp, err := awaitFirstChunk(context.Background(), req, 3*time.Second, client.Do)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.True(t, errors.Is(err, context.DeadlineExceeded))
		assert.NotErrorIs(t, err, errFirstChunkTimeout)
	})
}

func TestDoRequestFirstChunkPolicyAndRetrySafety(t *testing.T) {
	service.InitHttpClient()
	client := service.GetHttpClient()
	previousTransport := client.Transport
	previousTimeout := common.RelayTTFTTimeout
	general := operation_setting.GetGeneralSetting()
	previousGeneral := *general
	t.Cleanup(func() {
		client.Transport = previousTransport
		common.RelayTTFTTimeout = previousTimeout
		*general = previousGeneral
	})
	common.RelayTTFTTimeout = 1
	general.PingIntervalEnabled, general.PingIntervalSeconds = true, 1
	for _, tc := range []struct {
		name       string
		stream     bool
		committed  bool
		disabled   bool
		claude     bool
		request    *dto.OpenAIResponsesRequest
		wantReason string
	}{
		{name: "retryable stream", stream: true, wantReason: "retry_status_matched"},
		{name: "Claude Messages stream", stream: true, claude: true, wantReason: "retry_status_matched"},
		{name: "committed response", stream: true, committed: true, wantReason: "response_committed"},
		{name: "provider side effects", stream: true, request: &dto.OpenAIResponsesRequest{Tools: []byte(`[{"type":"mcp"}]`)}, wantReason: "non_retryable_error"},
		{name: "non streaming unchanged"},
		{name: "disabled stream preserves heartbeat", stream: true, disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				common.RelayTTFTTimeout = 1
				if tc.disabled {
					common.RelayTTFTTimeout = 0
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				if tc.committed {
					c.Writer.WriteHeaderNow()
				}
				info := &relaycommon.RelayInfo{IsStream: tc.stream, RelayMode: relayconstant.RelayModeChatCompletions, ChannelMeta: &relaycommon.ChannelMeta{}}
				if tc.claude {
					info.RelayMode, info.RelayFormat = relayconstant.RelayModeUnknown, types.RelayFormatClaude
				}
				if tc.request != nil {
					info.Request = tc.request
					info.RelayMode = relayconstant.RelayModeResponses
				}
				client.Transport = firstChunkTransport(func(r *http.Request) (*http.Response, error) {
					if trace := httptrace.ContextClientTrace(r.Context()); trace != nil && trace.WroteRequest != nil {
						trace.WroteRequest(httptrace.WroteRequestInfo{})
					}
					if tc.stream && !tc.disabled {
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					time.Sleep(3 * time.Second)
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
				})
				req, err := http.NewRequest(http.MethodPost, "http://provider.test", strings.NewReader("{}"))
				require.NoError(t, err)
				resp, err := doRequest(c, req, info)
				if !tc.stream || tc.disabled {
					require.NoError(t, err)
					require.NoError(t, resp.Body.Close())
					assert.Equal(t, tc.disabled, c.Writer.Written())
					return
				}
				var apiError *types.NewAPIError
				require.ErrorAs(t, err, &apiError)
				assert.Equal(t, 499, apiError.StatusCode)
				assert.Equal(t, tc.committed, c.Writer.Written(), "local pings must not commit before first upstream chunk")
				assert.Empty(t, c.Writer.Header().Get("Content-Type"), "failed attempts must not leave SSE headers on the final error")
				decision := service.DecideRelayRetry(c, apiError, 1)
				assert.Equal(t, tc.wantReason, decision.Reason)
				service.RecordPolicyFailure(c, 1, apiError, decision)
				events := service.RequestPolicy(c).Events()
				require.Len(t, events, 2)
				assert.Equal(t, 499, events[0].Status)
				assert.Equal(t, "transport", events[0].ErrorSource)
				assert.Equal(t, "upstream_failure", events[0].Decision.Reason)
				if tc.wantReason == "retry_status_matched" {
					assert.Equal(t, "retry", decision.Action)
					assert.Equal(t, "attempt_budget_exhausted", service.DecideRelayRetry(c, apiError, 0).Reason)
					// A newly selected channel starts a fresh attempt and sends only its own bytes.
					client.Transport = firstChunkTransport(func(r *http.Request) (*http.Response, error) {
						if trace := httptrace.ContextClientTrace(r.Context()); trace != nil && trace.WroteRequest != nil {
							trace.WroteRequest(httptrace.WroteRequestInfo{})
						}
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("new channel")), Header: make(http.Header)}, nil
					})
					retryReq, retryErr := http.NewRequest(http.MethodPost, "http://second-provider.test", strings.NewReader("{}"))
					require.NoError(t, retryErr)
					retryResp, retryErr := doRequest(c, retryReq, info)
					require.NoError(t, retryErr)
					data, readErr := io.ReadAll(retryResp.Body)
					require.NoError(t, readErr)
					assert.Equal(t, "new channel", string(data))
					require.NoError(t, retryResp.Body.Close())
				}
			})
		})
	}
}

func TestNewTaskAPIRequestInheritsClientCancellation(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestContext, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)

	upstream, err := newTaskAPIRequest(c, "https://provider.example/tasks", nil)
	require.NoError(t, err)
	cancel()

	require.ErrorIs(t, upstream.Context().Err(), context.Canceled)
}

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

func TestToWebSocketURL(t *testing.T) {
	for input, want := range map[string]string{
		"https://api.openai.com/v1/responses":             "wss://api.openai.com/v1/responses",
		"http://127.0.0.1:3000/v1/responses":              "ws://127.0.0.1:3000/v1/responses",
		"wss://chatgpt.com/backend-api/codex/responses":   "wss://chatgpt.com/backend-api/codex/responses",
		"ws://127.0.0.1:3000/backend-api/codex/responses": "ws://127.0.0.1:3000/backend-api/codex/responses",
	} {
		assert.Equal(t, want, toWebSocketURL(input), input)
	}
}
