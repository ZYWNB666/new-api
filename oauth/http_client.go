package oauth

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	oauthProxyURLEnv       = "OAUTH_PROXY_URL"
	oauthProxyProvidersEnv = "OAUTH_PROXY_PROVIDERS"
)

func oauthProviderUsesProxy(providerName string) bool {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		return false
	}
	for configuredName := range strings.SplitSeq(os.Getenv(oauthProxyProvidersEnv), ",") {
		if strings.EqualFold(strings.TrimSpace(configuredName), providerName) {
			return true
		}
	}
	return false
}

func newOAuthHTTPClient(providerName string, timeout time.Duration) (*http.Client, error) {
	client := &http.Client{Timeout: timeout}
	if !oauthProviderUsesProxy(providerName) {
		return client, nil
	}

	proxyURL, err := common.ParseProxyURLStrict(os.Getenv(oauthProxyURLEnv))
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", oauthProxyURLEnv, err)
	}
	if proxyURL == nil {
		return nil, fmt.Errorf("%s is required for providers listed in %s", oauthProxyURLEnv, oauthProxyProvidersEnv)
	}

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || defaultTransport == nil {
		return nil, fmt.Errorf("default HTTP transport does not support %s", oauthProxyURLEnv)
	}
	transport := defaultTransport.Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	client.Transport = transport
	return client, nil
}

type oauthProxyErrorTransport struct {
	err error
}

func (t oauthProxyErrorTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, t.err
}

func configuredOAuthHTTPClient(providerName string, timeout time.Duration) *http.Client {
	client, err := newOAuthHTTPClient(providerName, timeout)
	if err == nil {
		return client
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: oauthProxyErrorTransport{err: err},
	}
}
