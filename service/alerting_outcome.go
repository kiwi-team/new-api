package service

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// BeginAlertTaskAttempt is called after resolving an original task's channel,
// immediately before a new submission. Polling and downloads do not call it.
func BeginAlertTaskAttempt(c *gin.Context, channelID int, routingModel string) (*common.AlertSpan, error) {
	if err := model.CheckChannelModelAvailability(c.Request.Context(), channelID, routingModel); err != nil {
		return nil, err
	}
	if !common.AlertingEnabled {
		return nil, nil
	}
	channel, err := model.GetChannelById(channelID, false)
	if err != nil {
		return nil, err
	}
	key, _, version := model.ResolveChannelModelIdentity(channel, routingModel)
	_, span := common.BeginAlertSpan(c.Request.Context(), common.AlertHealthEvent{Layer: "channel_attempt", UserID: c.GetInt("id"), TokenID: c.GetInt("token_id"), ChannelID: channelID, ChannelName: channel.Name, Model: key, ConfigVersion: version, RequestID: c.GetString(common.RequestIdKey), Protocol: "midjourney", SampleClass: "production", AttributionCovered: true})
	return span, nil
}

// AlertRelayOutcome preserves the existing business/cancellation classifier,
// while retaining timeout as a failure subset for dedicated alert metrics.
func AlertRelayOutcome(ctx context.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) string {
	outcome := perfmetrics.ClassifyRelayOutcome(ctx, info, apiErr)
	if outcome != perfmetrics.OutcomeFailure {
		return string(outcome)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	if info != nil && info.StreamStatus.OutcomeSnapshot().EndReason == relaycommon.StreamEndReasonTimeout {
		return "timeout"
	}
	if apiErr != nil {
		if apiErr.StatusCode == http.StatusRequestTimeout || apiErr.StatusCode == http.StatusGatewayTimeout {
			return "timeout"
		}
		var networkError net.Error
		if errors.As(apiErr.Err, &networkError) && networkError.Timeout() {
			return "timeout"
		}
	}
	return "failure"
}
