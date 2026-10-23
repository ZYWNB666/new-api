package service

import (
	"errors"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/gatewaylimit"
	"github.com/gin-gonic/gin"
)

// RerouteGatewayLimit skips an exhausted supplier pool without consuming an
// upstream retry or overriding a channel pin or strict session affinity.
func RerouteGatewayLimit(c *gin.Context, retry *RetryParam, err error, probes int) bool {
	var rejected *gatewaylimit.Rejection
	if !errors.As(err, &rejected) || rejected.Rule.Scope != "provider" || probes >= 32 {
		return false
	}
	constraints := GetChannelConstraints(c)
	if _, pinned, _ := constraints.ResolvedPin(); pinned || RequestPolicy(c).SessionMode == "strict" {
		return false
	}
	constraints.AddFilter(dto.ChannelFilter{Kind: dto.FilterGatewayCapacity, ExcludedChannelIDs: rejected.Rule.ChannelIDs})
	retry.ResetRetryNextTry()
	return true
}
