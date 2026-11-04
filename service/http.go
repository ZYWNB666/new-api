package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/http/httpguts"
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

// CaptureUpstreamRequestID selects a response correlation ID, never a client
// header. Known names take precedence; vendor X-*-Request-Id / Requestid names
// are a fallback. Ties are deterministic regardless of Go's map iteration.
// Calling with nil clears the previous attempt before starting another one.
func CaptureUpstreamRequestID(c *gin.Context, headers http.Header, rules ...string) {
	if c == nil {
		return
	}
	rule := ""
	if len(rules) > 0 {
		rule = rules[0]
	}
	id, name := selectRequestIDHeader(headers, rule)
	c.Set(common.UpstreamRequestIdKey, id)
	c.Set(common.UpstreamRequestIdHeaderKey, name)
}

// CaptureClientRequestID runs only after the user has been authenticated. A
// blank user policy disables collection; it never falls back to client headers.
func CaptureClientRequestID(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	setting, _ := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
	var id, name string
	if strings.TrimSpace(setting.ClientRequestIDHeader) != "" {
		id, name = selectRequestIDHeader(c.Request.Header, setting.ClientRequestIDHeader)
	}
	c.Set(common.ClientRequestIdKey, id)
	c.Set(common.ClientRequestIdHeaderKey, name)
}

func selectRequestIDHeader(headers http.Header, rule string) (string, string) {
	configured := strings.TrimSpace(rule) != ""
	matcher, err := dto.CompileRequestIDHeaderRule(rule)
	if err != nil {
		return "", ""
	}
	bestPriority := 1000
	var selectedID, selectedHeader, selectedLower string
	for name, values := range headers {
		if !httpguts.ValidHeaderFieldName(name) || logger.IsSensitiveHTTPHeader(name) {
			continue
		}
		lower := strings.ToLower(name)
		priority := 100
		if configured {
			if matcher == nil || !matcher.MatchString(name) {
				continue
			}
		} else {
			switch lower {
			case strings.ToLower(common.RequestIdKey):
				priority = 0
			case strings.ToLower(common.LegacyUpstreamRequestIdKey):
				priority = 1
			case "x-upstream-request-id":
				priority = 2
			case "x-request-id", "x-requestid":
				priority = 3
			case "request-id", "requestid":
				priority = 4
			default:
				prefix, ok := strings.CutSuffix(lower, "-request-id")
				if !ok {
					prefix, ok = strings.CutSuffix(lower, "-requestid")
				}
				if !ok || !strings.HasPrefix(prefix, "x-") || len(prefix) <= 2 {
					continue
				}
			}
		}
		if priority > bestPriority || (priority == bestPriority && (lower > selectedLower || (lower == selectedLower && name >= selectedHeader))) {
			continue
		}
		for _, value := range values {
			value = strings.TrimSpace(value)
			// The persisted upstream_request_id column is varchar(128). Do not
			// truncate correlation IDs or accept whitespace/control characters.
			if value == "" || len(value) > 128 || strings.IndexFunc(value, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
				continue
			}
			selectedID, selectedHeader, selectedLower = value, name, lower
			bestPriority = priority
			break
		}
	}
	return selectedID, http.CanonicalHeaderKey(selectedHeader)
}

// ShouldCopyUpstreamHeader checks whether a given upstream response header
// should be copied to the client response. It returns false for Content-Length
// (managed separately) and gateway version/request ID headers. The local ID
// and version stay authoritative. CaptureUpstreamRequestID owns upstream ID
// selection; copying headers must not change that choice based on map order.
func ShouldCopyUpstreamHeader(c *gin.Context, k string, v []string) bool {
	publicName := common.PublicResponseHeaderName(k)
	if strings.EqualFold(k, "Content-Length") {
		return false
	}
	if strings.EqualFold(k, "Access-Control-Expose-Headers") && c != nil {
		// Keep the locally exposed correlation/version headers when a provider
		// supplies its own list of headers readable by browser clients.
		for _, value := range v {
			var names []string
			for name := range strings.SplitSeq(value, ",") {
				name = strings.TrimSpace(name)
				name = common.PublicResponseHeaderName(name)
				if name == "" {
					continue
				}
				names = append(names, name)
			}
			if len(names) > 0 {
				c.Writer.Header().Add(k, strings.Join(names, ", "))
			}
		}
		return false
	}
	if strings.EqualFold(publicName, common.VersionHeaderKey) {
		return false
	}
	if strings.EqualFold(publicName, common.RequestIdKey) {
		return false
	}
	return true
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	body := io.NopCloser(bytes.NewBuffer(data))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		if _, captured := c.Get(common.UpstreamRequestIdHeaderKey); !captured {
			CaptureUpstreamRequestID(c, src.Header)
		}
		for k, v := range src.Header {
			if !ShouldCopyUpstreamHeader(c, k, v) {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	c.Writer.Flush()
}
