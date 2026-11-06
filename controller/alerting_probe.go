package controller

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/alerting"
)

func init() {
	// The controller owns the provider-test pipeline; injecting it keeps the
	// evaluation domain independent of HTTP presenters and provider adaptors.
	service.AlertEngine.Probe = probeAlertChannel
}

func probeAlertChannel(ctx context.Context, hold model.AlertDisableHold) alerting.ProbeResult {
	channel, err := model.GetChannelById(hold.ChannelID, true)
	if err != nil {
		return alerting.ProbeResult{}
	}
	key, _, version := model.ResolveChannelModelIdentity(channel, hold.ModelKey)
	if version != hold.ConfigVersion {
		return alerting.ProbeResult{ConfigVersion: version}
	}
	var root model.User
	if err = model.DB.WithContext(ctx).Select("id").Where("role = ? AND status = ?", common.RoleRootUser, common.UserStatusEnabled).First(&root).Error; err != nil {
		return alerting.ProbeResult{}
	}
	ctx = common.WithAlertRecoveryProbe(ctx)
	var recovery model.AlertRecovery
	if common.UnmarshalJsonStr(hold.Recovery, &recovery) != nil {
		return alerting.ProbeResult{}
	}
	endpoint := map[string]string{"openai": string(constant.EndpointTypeOpenAI), "claude": string(constant.EndpointTypeAnthropic), "gemini": string(constant.EndpointTypeGemini), "openai_responses": string(constant.EndpointTypeOpenAIResponse)}[recovery.Protocol]
	if endpoint == "" {
		return alerting.ProbeResult{}
	}
	ctx, span := common.BeginAlertSpan(ctx, common.AlertHealthEvent{Layer: "channel_attempt", ChannelID: channel.Id, ChannelName: channel.Name, Model: key, ConfigVersion: version, SampleClass: "recovery_probe", OutputCovered: true, Stream: true, Protocol: recovery.Protocol})
	started := time.Now()
	result := testChannel(ctx, channel, root.Id, key, endpoint, true)
	passed := result.localErr == nil && result.newAPIError == nil
	outcome := "failure"
	if passed {
		outcome = "success"
	}
	span.End(ctx, outcome)
	snapshot := span.Snapshot()
	duration := time.Since(started).Milliseconds()
	return alerting.ProbeResult{Success: passed, DurationMS: &duration, TTFTMS: snapshot.TTFTMS, ConfigVersion: version}
}
