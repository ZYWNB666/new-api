package middleware

import (
	"bufio"
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowCredentials = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"*"}
	config.ExposeHeaders = []string{common.RequestIdKey, common.VersionHeaderKey}
	return cors.New(config)
}

func Version() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header(common.VersionHeaderKey, common.Version)
		c.Writer = &publicResponseWriter{ResponseWriter: c.Writer}
		defer normalizeResponseHeaders(c.Writer.Header())
		c.Next()
	}
}

// Keep Gin's status, Written and streaming behavior: only normalize metadata
// immediately before committing, without buffering or committing a response early.
type publicResponseWriter struct {
	gin.ResponseWriter
}

func (w *publicResponseWriter) prepareHeaders() {
	if !w.Written() {
		normalizeResponseHeaders(w.Header())
	}
}

func (w *publicResponseWriter) Write(data []byte) (int, error) {
	w.prepareHeaders()
	return w.ResponseWriter.Write(data)
}

func (w *publicResponseWriter) WriteString(data string) (int, error) {
	w.prepareHeaders()
	return w.ResponseWriter.WriteString(data)
}

func (w *publicResponseWriter) WriteHeaderNow() {
	w.prepareHeaders()
	w.ResponseWriter.WriteHeaderNow()
}

func (w *publicResponseWriter) Flush() {
	w.prepareHeaders()
	w.ResponseWriter.Flush()
}

func (w *publicResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.prepareHeaders()
	return w.ResponseWriter.Hijack()
}

func normalizeResponseHeaders(header http.Header) {
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		name := key
		prefix := ""
		if trailer, ok := strings.CutPrefix(key, http.TrailerPrefix); ok {
			name, prefix = trailer, http.TrailerPrefix
		}
		renamed := prefix + common.PublicResponseHeaderName(name)
		if renamed == key {
			continue
		}
		// An explicitly supplied public name wins, especially our local ID/version.
		if _, exists := header[renamed]; !exists {
			header[renamed] = header[key]
		}
		delete(header, key)
	}
	// These values contain response header names rather than application data.
	for _, key := range []string{"Access-Control-Expose-Headers", "Trailer"} {
		values, exists := header[key]
		if !exists {
			continue
		}
		var names []string
		for _, value := range values {
			for name := range strings.SplitSeq(value, ",") {
				name = common.PublicResponseHeaderName(strings.TrimSpace(name))
				if name != "" && !slices.ContainsFunc(names, func(existing string) bool { return strings.EqualFold(existing, name) }) {
					names = append(names, name)
				}
			}
		}
		header[key] = []string{strings.Join(names, ", ")}
	}
}
