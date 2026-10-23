package common

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/pkg/gatewaylimit"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const gatewayLeaseKey = "gateway_limit_leases"
const gatewayErrorKey = "gateway_limit_error"

// GatewayLimitCleanup runs after the response, including synchronous task presenters.
func GatewayLimitCleanup() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if value, ok := c.Get(gatewayLeaseKey); ok {
				for _, lease := range value.([]*gatewaylimit.Lease) {
					lease.Finish()
				}
			}
		}()
		c.Next()
	}
}

func registerGatewayLease(c *gin.Context, lease *gatewaylimit.Lease) {
	if lease == nil {
		return
	}
	var leases []*gatewaylimit.Lease
	if value, ok := c.Get(gatewayLeaseKey); ok {
		leases = value.([]*gatewaylimit.Lease)
	}
	c.Set(gatewayLeaseKey, append(leases, lease))
}

func GatewayLimitError(c *gin.Context, err error) *types.NewAPIError {
	code := http.StatusServiceUnavailable
	var rejection *gatewaylimit.Rejection
	if errors.As(err, &rejection) {
		code = http.StatusTooManyRequests
		c.Header("Retry-After", strconv.Itoa(rejection.RetryAfter))
	}
	if code == http.StatusServiceUnavailable {
		err = gatewaylimit.ErrUnavailable
	}
	return types.NewErrorWithStatusCode(err, types.ErrorCode("gateway_rate_limit"), code, types.ErrOptionWithSkipRetry())
}

func (info *RelayInfo) AcquireRequestLimit(c *gin.Context, estimate int64) *types.NewAPIError {
	if info.GatewayRequestLimit != nil {
		return nil
	}
	rules := gatewaylimit.Current().RequestRules(info.UserId, info.TokenId, info.OriginModelName)
	lease, err := gatewaylimit.Default.Acquire(c.Request.Context(), rules, estimate)
	if err != nil {
		return GatewayLimitError(c, err)
	}
	info.GatewayRequestLimit = lease
	registerGatewayLease(c, lease)
	return nil
}

func (info *RelayInfo) AcquireProviderLimit(c *gin.Context, channelID int) error {
	if info.GatewayProviderLimit != nil {
		return nil
	}
	rules := gatewaylimit.Current().ProviderRules(channelID)
	lease, err := gatewaylimit.Default.Acquire(c.Request.Context(), rules, info.GatewayTokenEstimate)
	if err != nil {
		c.Set(gatewayErrorKey, err)
		return err
	}
	info.GatewayProviderLimit = lease
	registerGatewayLease(c, lease)
	return nil
}

func GatewayProviderError(c *gin.Context) error {
	value, ok := c.Get(gatewayErrorKey)
	if !ok || value == nil {
		return nil
	}
	c.Set(gatewayErrorKey, nil)
	return value.(error)
}

func (info *RelayInfo) FinishProviderLimit() {
	info.GatewayProviderLimit.Finish()
	info.GatewayProviderLimit = nil
}
func (info *RelayInfo) FinishGatewayLimits() {
	info.FinishProviderLimit()
	info.GatewayRequestLimit.Finish()
}
func (info *RelayInfo) MarkGatewaySent() {
	info.GatewayRequestLimit.MarkSent()
	info.GatewayProviderLimit.MarkSent()
}
func (info *RelayInfo) ReportGatewayUsage(prompt, completion int) {
	if prompt < 0 || completion < 0 {
		return
	}
	total := int64(prompt) + int64(completion)
	info.GatewayRequestLimit.ReportUsage(total)
	info.GatewayProviderLimit.ReportUsage(total)
}
