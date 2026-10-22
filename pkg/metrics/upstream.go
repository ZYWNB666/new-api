package metrics

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"time"
)

// DoUpstream observes one shared upstream HTTP call without reading or wrapping its body.
// Traces compose with existing hooks, and the cached client and transport remain unchanged.
func (m *Metrics) DoUpstream(client *http.Client, req *http.Request) (*http.Response, error) {
	return m.DoProviderUpstream(client, req, 0, 0)
}

func (m *Metrics) DoProviderUpstream(client *http.Client, req *http.Request, channelID, channelType int) (*http.Response, error) {
	started := time.Now()
	m.upstreamInFlight.Inc()
	defer m.upstreamInFlight.Dec()
	var mu sync.Mutex
	var acquiring time.Time
	trace := &httptrace.ClientTrace{
		GetConn: func(_ string) {
			mu.Lock()
			acquiring = time.Now()
			mu.Unlock()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			mu.Lock()
			if !acquiring.IsZero() {
				m.connectionAcquire.Observe(time.Since(acquiring).Seconds())
			}
			mu.Unlock()
			m.connections.WithLabelValues(strconv.FormatBool(info.Reused)).Inc()
		},
	}
	tracedRequest := req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := client.Do(tracedRequest)
	status, errorKind := "none", "none"
	if resp != nil {
		status = strconv.Itoa(resp.StatusCode)
	}
	if err != nil {
		var networkError net.Error
		switch {
		case errors.Is(err, context.Canceled):
			errorKind = "canceled"
		case errors.Is(err, context.DeadlineExceeded):
			errorKind = "timeout"
		case errors.As(err, &networkError) && networkError.Timeout():
			errorKind = "timeout"
		default:
			errorKind = "transport"
		}
	}
	m.upstreamRequests.WithLabelValues(status, errorKind, strconv.Itoa(channelID), strconv.Itoa(channelType)).Inc()
	m.upstreamDuration.WithLabelValues(errorKind).Observe(time.Since(started).Seconds())
	return resp, err
}

// ProviderHTTPClient implements SDK HTTP client interfaces. SDK-internal retries
// each call Do, so their native wire statuses are observed separately too.
type ProviderHTTPClient struct {
	Client                 *http.Client
	Monitor                *Metrics
	Generation             *Generation
	ChannelID, ChannelType int
}

func (c *ProviderHTTPClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.Monitor.DoProviderUpstream(c.Client, req, c.ChannelID, c.ChannelType)
	if resp != nil {
		c.Generation.SetProviderStatus(resp.StatusCode)
	}
	return resp, err
}
