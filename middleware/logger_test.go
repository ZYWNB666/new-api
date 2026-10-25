package middleware

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicResponseHeaderNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"body", "string", "status only", "explicit commit", "flush"} {
		t.Run(mode, func(t *testing.T) {
			router := gin.New()
			router.Use(RequestId(), Version())
			router.GET("/", func(c *gin.Context) {
				c.Header("X-New-Api-Other-Ratios", `{"seconds":5}`)
				c.Writer.Header()["x-oNeApI-trace"] = []string{"trace-oneapi-value"}
				c.Header("X-Newapi-Extension", "extension")
				c.Header("X-One_Api-Metadata", "metadata")
				c.Header("X-New-Api-Version", "supplier-version")
				c.Header("X-Oneapi-Request-Id", "supplier-id")
				c.Header("Access-Control-Expose-Headers", "X-New-Api-Other-Ratios, X-Oneapi-Trace, X-Nawei-Trace")
				c.Header("Set-Cookie", "newapi-session=opaque-oneapi; Path=/; HttpOnly")
				c.Header("Location", "https://newapi.example/file?signature=oneapi")
				assert.False(t, c.Writer.Written(), "normalization must not commit before a retry can be considered")
				c.Status(http.StatusAccepted)
				assert.False(t, c.Writer.Written(), "Gin WriteHeader only stages the status")
				switch mode {
				case "body":
					_, _ = c.Writer.Write([]byte("ok"))
				case "string":
					_, _ = c.Writer.WriteString("ok")
				case "explicit commit":
					c.Writer.WriteHeaderNow()
				case "flush":
					c.Writer.Flush()
				}
				assert.Equal(t, mode != "status only", c.Writer.Written())
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			response := recorder.Result()
			defer response.Body.Close()
			assert.Equal(t, http.StatusAccepted, response.StatusCode)
			assert.Equal(t, `{"seconds":5}`, response.Header.Get("X-Nawei-Other-Ratios"))
			assert.Equal(t, "trace-oneapi-value", response.Header.Get("X-Nawei-Trace"))
			assert.Equal(t, "extension", response.Header.Get("X-Nawei-Extension"))
			assert.Equal(t, "metadata", response.Header.Get("X-Nawei-Metadata"))
			assert.Equal(t, common.Version, response.Header.Get(common.VersionHeaderKey))
			assert.NotEqual(t, "supplier-id", response.Header.Get(common.RequestIdKey))
			assert.Equal(t, "X-Nawei-Other-Ratios, X-Nawei-Trace", response.Header.Get("Access-Control-Expose-Headers"))
			assert.Equal(t, "newapi-session=opaque-oneapi; Path=/; HttpOnly", response.Header.Get("Set-Cookie"))
			assert.Equal(t, "https://newapi.example/file?signature=oneapi", response.Header.Get("Location"))
			for key := range response.Header {
				assert.Equal(t, key, common.PublicResponseHeaderName(key), "legacy response header: %s", key)
			}
		})
	}
}

func TestPublicResponseStreamingAndWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestId(), Version())
	finishStream := make(chan struct{})
	router.GET("/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("X-Oneapi-Trace", "stream-trace")
		c.Header("Trailer", "X-Newapi-End")
		_, _ = c.Writer.WriteString("data: first\n\n")
		c.Writer.Flush()
		select {
		case <-finishStream:
		case <-c.Request.Context().Done():
			return
		}
		_, _ = c.Writer.WriteString("data: second\n\n")
		c.Header("X-Newapi-End", "complete")
		c.Writer.Header().Set(http.TrailerPrefix+"X-Oneapi-Late", "late")
	})
	router.GET("/ws", func(c *gin.Context) {
		c.Header("X-Newapi-Trace", "ws-trace")
		conn, err := (&websocket.Upgrader{}).Upgrade(c.Writer, c.Request, c.Writer.Header())
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("ok"))
	})
	server := httptest.NewServer(router)
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(server.URL + "/stream")
	if !assert.NoError(t, err) {
		close(finishStream)
		return
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	first, err := reader.ReadString('\n')
	close(finishStream)
	require.NoError(t, err)
	assert.Equal(t, "data: first\n", first, "the first event must arrive before the handler completes")
	assert.Equal(t, "stream-trace", response.Header.Get("X-Nawei-Trace"))
	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "\ndata: second\n\n", string(rest))
	assert.Equal(t, "complete", response.Trailer.Get("X-Nawei-End"))
	assert.Equal(t, "late", response.Trailer.Get("X-Nawei-Late"))
	assert.Empty(t, response.Trailer.Get("X-Newapi-End"))
	assert.Empty(t, response.Trailer.Get("X-Oneapi-Late"))
	conn, handshake, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	assert.Equal(t, "ws-trace", handshake.Header.Get("X-Nawei-Trace"))
	assert.Empty(t, handshake.Header.Get("X-Newapi-Trace"))
	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "ok", string(message))
}

func TestNaweiResponseHeaderContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, upstreamHeader, upstreamID string
		stream                           bool
		alsoLegacy                       bool
		status                           int
	}{
		{name: "plain response", status: 200},
		{name: "stream response", stream: true, status: 200},
		{name: "error response", status: 429},
		{name: "legacy supplier ID", upstreamHeader: "X-Oneapi-Request-Id", upstreamID: "legacy-provider-id", status: 200},
		{name: "renamed supplier ID", upstreamHeader: "X-Nawei-Request-Id", upstreamID: "new-provider-id", status: 200},
		{name: "new supplier ID takes precedence", upstreamHeader: "X-Nawei-Request-Id", upstreamID: "new-provider-id", alsoLegacy: true, status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(RequestId(), Version(), CORS())
			localID := ""
			router.GET("/headers", func(c *gin.Context) {
				localID = c.GetString(common.RequestIdKey)
				assert.Equal(t, localID, c.Request.Context().Value(common.RequestIdKey))
				if tc.stream {
					c.Header("Content-Type", "text/event-stream")
					c.String(tc.status, "data: [DONE]\n\n")
					c.Writer.Flush()
					return
				}
				header := http.Header{"X-New-Api-Version": {"supplier-version"}, "X-Nawei-Version": {"supplier-version"}, "Access-Control-Expose-Headers": {"X-Provider-Trace, X-Oneapi-Request-Id, X-New-Api-Version"}}
				if tc.upstreamHeader != "" {
					header.Set(tc.upstreamHeader, tc.upstreamID)
				}
				if tc.alsoLegacy {
					header.Set("X-Oneapi-Request-Id", "legacy-provider-id")
				}
				service.IOCopyBytesGracefully(c, &http.Response{StatusCode: tc.status, Header: header}, []byte("ok"))
				if tc.upstreamID != "" {
					assert.Equal(t, tc.upstreamID, c.GetString(common.UpstreamRequestIdKey))
				}
			})
			request := httptest.NewRequest(http.MethodGet, "/headers", nil)
			request.Header.Set("Origin", "https://client.example")
			request.Header.Set("X-Nawei-Request-Id", "untrusted-client-id")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code)
			require.NotEmpty(t, localID)
			assert.NotEqual(t, "untrusted-client-id", localID)
			assert.Equal(t, localID, response.Header().Get("X-Nawei-Request-Id"))
			assert.Equal(t, common.Version, response.Header().Get("X-Nawei-Version"))
			assert.Empty(t, response.Header().Get("X-Oneapi-Request-Id"))
			assert.Empty(t, response.Header().Get("X-New-Api-Version"))
			exposed := strings.ToLower(strings.Join(response.Header().Values("Access-Control-Expose-Headers"), ","))
			assert.Contains(t, exposed, "x-nawei-request-id")
			assert.Contains(t, exposed, "x-nawei-version")
			assert.NotContains(t, exposed, "x-oneapi-request-id")
			assert.NotContains(t, exposed, "x-new-api-version")
			if !tc.stream {
				assert.Contains(t, exposed, "x-provider-trace")
			}
		})
	}
}

func TestRelayClientRequestHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, enabled, tag string
		logged             bool
	}{
		{"enabled relay", "true", "relay", true},
		{"disabled relay", "false", "relay", false},
		{"management excluded", "true", "api", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LOG_REQUEST_HEADERS", tc.enabled)
			previous := gin.DefaultWriter
			var output bytes.Buffer
			gin.DefaultWriter = &output
			t.Cleanup(func() { gin.DefaultWriter = previous })
			router := gin.New()
			router.Use(RequestId(), RouteTag(tc.tag))
			original := http.Header{
				"Authorization": {"Bearer do-not-log-authorization"}, "Cookie": {"session=do-not-log-cookie"},
				"X-Api-Key": {"do-not-log-api-key"}, "X-Goog-Api-Key": {"do-not-log-google-key"},
				"X-Custom-Token": {"do-not-log-token"}, "X-Signature": {"do-not-log-signature"},
				"Referer":                {"https://client.test/?access=do-not-log-referer"},
				"Sec-Websocket-Protocol": {"do-not-log-websocket-token"},
				"User-Agent":             {"client-sdk"}, "Content-Type": {"application/json"},
				"Anthropic-Version": {"2023-06-01"}, "X-Debug-Value": {"line1\nline2"},
				"X-Long-Value": {strings.Repeat("x", 1024)}, "X-Multiple": {"a", "b"},
			}
			router.POST("/v1/chat/completions", func(c *gin.Context) {
				assert.Equal(t, original, c.Request.Header, "logging must not mutate authentication or forwarded headers")
				if tc.logged {
					assert.Contains(t, output.String(), "client request headers:", "headers should be recorded before request handling")
				}
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodPost, "http://gateway.test/v1/chat/completions?token=do-not-log-query", nil)
			request.Header = original.Clone()
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusNoContent, recorder.Code)
			if !tc.logged {
				assert.Empty(t, output.String())
				return
			}
			line := output.String()
			assert.Contains(t, line, "client request headers:")
			assert.NotContains(t, line, "do-not-log-")
			assert.Contains(t, line, `"Host":["gateway.test"]`)
			assert.Contains(t, line, `"User-Agent":["client-sdk"]`)
			assert.Contains(t, line, `"Anthropic-Version":["2023-06-01"]`)
			assert.Contains(t, line, `"X-Multiple":["a","b"]`)
			assert.Contains(t, line, `"X-Debug-Value":["line1\nline2"]`)
			assert.Contains(t, line, "[REDACTED]")
			assert.Contains(t, line, "[TRUNCATED]")
			assert.Equal(t, 1, strings.Count(line, "\n"), "untrusted headers cannot inject log lines")
			assert.NotContains(t, line, "| SYSTEM |", "request ID should correlate the header log")
		})
	}
}
