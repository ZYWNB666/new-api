package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(),
		middleware.Cache(),
		func(c *gin.Context) {
			if isAPIRequest(c.Request) {
				controller.RelayNotFound(c)
				c.Abort()
			}
		},
		static.Serve("/", frontendFS),
		func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/assets") || strings.HasPrefix(c.Request.URL.Path, "/static") {
				controller.RelayNotFound(c)
				return
			}
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.IndexPage)
		},
	)
}

// isAPIRequest keeps API probes and failed API calls out of the SPA fallback.
// Plugin routes run first, and ordinary browser navigation remains available.
func isAPIRequest(request *http.Request) bool {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return true
	}
	segment, _, _ := strings.Cut(strings.TrimPrefix(strings.ToLower(request.URL.Path), "/"), "/")
	if len(segment) > 1 && segment[0] == 'v' && segment[1] >= '0' && segment[1] <= '9' {
		return true
	}
	switch segment {
	case "api", "messages", "responses", "chat", "completions", "embeddings":
		return true
	}
	return request.Header.Get("Authorization") != "" ||
		request.Header.Get("X-Api-Key") != "" ||
		request.Header.Get("X-Goog-Api-Key") != "" ||
		strings.Contains(strings.ToLower(request.Header.Get("Accept")), "application/json") ||
		strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/json")
}
