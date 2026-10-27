package channel

import (
	"errors"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/pkg/metrics"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// MeteredAdaptor observes the common adaptor boundary, including SDK-backed
// providers. Embedding preserves all request conversion and configuration methods.
type MeteredAdaptor struct {
	Adaptor
	attempt *metrics.ProviderAttempt
}

func (a *MeteredAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (response any, err error) {
	if err = info.AcquireProviderLimit(c, info.ChannelId); err != nil {
		return nil, err
	}
	info.MarkGatewaySent()
	a.attempt = info.BeginProviderMetrics(c)
	returned := false
	defer func() {
		if !returned {
			a.attempt.Finish(0, "failure")
		}
	}()
	response, err = a.Adaptor.DoRequest(c, info, body)
	returned = true
	if resp, ok := response.(*http.Response); ok && resp != nil {
		a.attempt.SetStatus(resp.StatusCode)
		a.attempt.SetRetryAfter(resp.Header.Get("Retry-After"))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			a.attempt.Finish(0, "failure")
		}
	}
	if err != nil {
		var statusError interface{ HTTPStatusCode() int }
		if errors.As(err, &statusError) {
			a.attempt.SetStatus(statusError.HTTPStatusCode())
		}
		apiErr := types.NewError(err, types.ErrorCodeDoRequestFailed)
		a.attempt.Finish(0, string(perfmetrics.ClassifyRelayOutcome(c.Request.Context(), info, apiErr)))
	}
	return response, err
}

func (a *MeteredAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, apiErr *types.NewAPIError) {
	returned := false
	defer func() {
		if !returned {
			a.attempt.Finish(0, "failure")
		}
	}()
	usage, apiErr = a.Adaptor.DoResponse(c, resp, info)
	returned = true
	if value, ok := usage.(*dto.Usage); ok && value != nil {
		info.ReportGatewayUsage(value.PromptTokens, value.CompletionTokens)
	}
	if value, ok := usage.(*dto.RealtimeUsage); ok && value != nil {
		info.ReportGatewayUsage(value.InputTokens, value.OutputTokens)
	}
	if a.attempt == nil {
		return usage, apiErr
	}
	// SDK adaptors execute their network call inside DoResponse and do not
	// expose an HTTP response. Preserve their native status before host remapping.
	if resp == nil {
		if apiErr != nil {
			var statusError interface{ HTTPStatusCode() int }
			if errors.As(apiErr, &statusError) {
				a.attempt.SetStatus(statusError.HTTPStatusCode())
			} else {
				a.attempt.SetStatus(apiErr.StatusCode)
			}
		} else {
			a.attempt.SetStatus(http.StatusOK)
		}
	}
	var tokens int64
	info.ReportPromptCacheUsage(nil)
	if value, ok := usage.(*dto.Usage); ok && value != nil {
		tokens = int64(value.CompletionTokens)
		info.ReportPromptCacheUsage(value)
		details := value.CompletionTokenDetails
		incompatibleOutput := details.ReasoningTokens > 0 || details.AudioTokens > 0 || details.ImageTokens > 0
		if output := value.OutputTokensDetails; output != nil {
			incompatibleOutput = incompatibleOutput || output.ReasoningTokens > 0 || output.AudioTokens > 0 || output.ImageTokens > 0
		}
		if (value.BillingUsage == nil || !value.BillingUsage.Estimated) && !incompatibleOutput {
			a.attempt.SetRoutingOutputTokens(int64(value.CompletionTokens))
		}
	}
	if stream := info.StreamStatus.OutcomeSnapshot(); stream.ErrorStatus != 0 {
		info.GenerationMetrics.SetProviderStatus(stream.ErrorStatus)
	}
	a.attempt.Finish(tokens, string(perfmetrics.ClassifyRelayOutcome(c.Request.Context(), info, apiErr)))
	return usage, apiErr
}
