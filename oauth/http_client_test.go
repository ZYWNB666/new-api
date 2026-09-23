package oauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOAuthHTTPClientSelectsConfiguredProviders(t *testing.T) {
	t.Setenv(oauthProxyURLEnv, "socks5://example-user:example-password@127.0.0.1:1080")
	t.Setenv(oauthProxyProvidersEnv, " linuxdo,GitHub ")

	proxiedClient, err := newOAuthHTTPClient("linuxdo", 5*time.Second)
	require.NoError(t, err)
	transport, ok := proxiedClient.Transport.(*http.Transport)
	require.True(t, ok)
	proxyURL, err := transport.Proxy(httptest.NewRequest(http.MethodGet, "https://connect.linux.do", nil))
	require.NoError(t, err)
	require.NotNil(t, proxyURL)
	assert.Equal(t, "socks5", proxyURL.Scheme)
	assert.Equal(t, "127.0.0.1:1080", proxyURL.Host)

	directClient, err := newOAuthHTTPClient("discord", 5*time.Second)
	require.NoError(t, err)
	assert.Nil(t, directClient.Transport)
}

func TestNewOAuthHTTPClientRejectsUnsafeConfigurationWithoutLeakingCredentials(t *testing.T) {
	t.Setenv(oauthProxyProvidersEnv, "linuxdo")
	t.Setenv(oauthProxyURLEnv, "socks5://sensitive-user:sensitive-password@")

	_, err := newOAuthHTTPClient("linuxdo", 5*time.Second)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sensitive-user")
	assert.NotContains(t, err.Error(), "sensitive-password")
}

func TestNewOAuthHTTPClientRequiresURLForSelectedProvider(t *testing.T) {
	t.Setenv(oauthProxyProvidersEnv, "linuxdo")
	t.Setenv(oauthProxyURLEnv, "")

	_, err := newOAuthHTTPClient("linuxdo", 5*time.Second)
	require.EqualError(t, err, "OAUTH_PROXY_URL is required for providers listed in OAUTH_PROXY_PROVIDERS")
}

func TestLinuxDOProviderRoutesTokenAndUserInfoThroughOAuthProxy(t *testing.T) {
	var requestCount atomic.Int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.Header.Get("Proxy-Authorization") == "" {
			http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = io.WriteString(w, `{"access_token":"test-access-token"}`)
		case "/api/user":
			_, _ = io.WriteString(w, `{"id":7,"username":"proxy-user","name":"Proxy User","active":true,"trust_level":2}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer proxyServer.Close()

	parsedProxyURL, err := url.Parse(proxyServer.URL)
	require.NoError(t, err)
	parsedProxyURL.User = url.UserPassword("example-user", "example-password")
	t.Setenv(oauthProxyURLEnv, parsedProxyURL.String())
	t.Setenv(oauthProxyProvidersEnv, "linuxdo")
	t.Setenv("LINUX_DO_TOKEN_ENDPOINT", "http://linuxdo.test/oauth2/token")
	t.Setenv("LINUX_DO_USER_ENDPOINT", "http://linuxdo.test/api/user")

	originalClientID := common.LinuxDOClientId
	originalClientSecret := common.LinuxDOClientSecret
	originalTrustLevel := common.LinuxDOMinimumTrustLevel
	defer func() {
		common.LinuxDOClientId = originalClientID
		common.LinuxDOClientSecret = originalClientSecret
		common.LinuxDOMinimumTrustLevel = originalTrustLevel
	}()
	common.LinuxDOClientId = "test-client"
	common.LinuxDOClientSecret = "test-secret"
	common.LinuxDOMinimumTrustLevel = 1

	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest(http.MethodGet, "https://console.example/api/oauth/linuxdo", nil)
	provider := &LinuxDOProvider{}
	token, err := provider.ExchangeToken(context.Background(), "test-code", ginContext)
	require.NoError(t, err)
	require.NotNil(t, token)

	user, err := provider.GetUserInfo(context.Background(), token)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "7", user.ProviderUserID)
	assert.Equal(t, int32(2), requestCount.Load())

	assert.False(t, strings.Contains(parsedProxyURL.Redacted(), "example-password"))
}
