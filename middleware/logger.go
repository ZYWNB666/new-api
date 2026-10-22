package middleware

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

const RouteTagKey = "route_tag"

func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RouteTagKey, tag)
		if tag == "relay" && common.GetEnvOrDefaultBool("LOG_REQUEST_HEADERS", false) {
			headers := map[string][]string{"Host": {c.Request.Host[:min(len(c.Request.Host), 512)]}}
			for _, name := range slices.Sorted(maps.Keys(c.Request.Header)) {
				if len(headers) >= 64 {
					headers["_truncated"] = []string{"too many headers"}
					break
				}
				lower := strings.ToLower(name)
				sensitive := lower == "referer" || lower == "sec-websocket-protocol"
				for _, part := range []string{"auth", "cookie", "key", "token", "secret", "password", "credential", "signature", "url", "uri"} {
					if strings.Contains(lower, part) {
						sensitive = true
						break
					}
				}
				if sensitive {
					headers[name] = []string{"[REDACTED]"}
					continue
				}
				for i, value := range c.Request.Header[name] {
					if i == 4 {
						headers[name] = append(headers[name], "[TRUNCATED]")
						break
					}
					if len(value) > 512 {
						value = value[:512] + "[TRUNCATED]"
					}
					headers[name] = append(headers[name], value)
				}
			}
			if encoded, err := common.Marshal(headers); err == nil {
				logger.LogInfo(c.Request.Context(), "client request headers: "+string(encoded))
			}
		}
		c.Next()
	}
}

func SetUpLogger(server *gin.Engine) {
	server.Use(redactTaskArtifactAccessQuery())
	server.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		path := param.Path
		// OAuth callbacks carry one-time codes and state in the query string.
		// Redact the log value only; the handler still needs the original query.
		if strings.HasPrefix(path, "/api/oauth/") || strings.HasPrefix(path, "/oauth/") {
			path, _, _ = strings.Cut(path, "?")
		}
		return fmt.Sprintf("[GIN] %s | %s | %s | %3d | %13v | %15s | %7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			tag,
			requestID,
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			path,
		)
	}))
}
