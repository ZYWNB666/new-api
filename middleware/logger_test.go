package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
