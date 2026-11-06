package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// RealtimeAlertTracker measures response operations, never connection lifetime.
// Metadata is captured before provider reader goroutines begin.
type RealtimeAlertTracker struct {
	mu                  sync.Mutex
	metadata            common.AlertHealthEvent
	requestedModel      string
	request             *common.AlertSpan
	attempt             *common.AlertSpan
	outputContext       context.Context
	inputAdmitted       bool
	nativeStartExpected bool
	unverified          bool
}

func NewRealtimeAlertTracker(metadata common.AlertHealthEvent) *RealtimeAlertTracker {
	metadata.Protocol, metadata.Stream, metadata.OutputCovered = "realtime", true, true
	return &RealtimeAlertTracker{metadata: metadata, requestedModel: metadata.Model}
}

func (t *RealtimeAlertTracker) Admit(ctx context.Context, kind string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	inference := kind == "response.create" || kind == "input_audio_buffer.commit" || kind == "input_audio_buffer.append" && !t.inputAdmitted
	if !inference {
		return nil
	}
	if err := model.CheckChannelModelAvailability(ctx, t.metadata.ChannelID, t.metadata.Model); err != nil {
		return errors.New("channel model is unavailable")
	}
	held, err := model.AlertTokenHeld(ctx, t.metadata.TokenID)
	if err != nil {
		return err
	}
	if held {
		return errors.New("API key is unavailable")
	}
	if common.AlertingControlsEnabled || common.AlertingEnabled {
		var channel model.Channel
		if err := model.DB.WithContext(ctx).Select("id", "name", "type", "models", "model_mapping", "group", "settings").First(&channel, t.metadata.ChannelID).Error; err != nil {
			return err
		}
		key, _, version := model.ResolveChannelModelIdentity(&channel, t.requestedModel)
		if t.metadata.ConfigVersion != "" && t.metadata.ConfigVersion != version {
			return errors.New("channel configuration changed; reconnect before starting a new response")
		}
		t.metadata.Model, t.metadata.ChannelName, t.metadata.ConfigVersion = key, channel.Name, version
	}
	t.inputAdmitted = true
	if kind == "response.create" || kind == "input_audio_buffer.commit" {
		t.nativeStartExpected = true
		if t.attempt != nil {
			t.unverified = true
			t.attempt.MarkAlertCoverageUnverified(ctx)
			t.request.MarkAlertCoverageUnverified(ctx)
		}
		t.begin(ctx)
	}
	return nil
}

func (t *RealtimeAlertTracker) begin(ctx context.Context) {
	if t.attempt != nil {
		return
	}
	if t.unverified {
		t.metadata.SampleClass, t.metadata.OutputCovered = "unsupported", false
	}
	request := t.metadata
	request.Layer = "request"
	parent, span := common.BeginAlertSpan(ctx, request)
	t.request = span
	attempt := t.metadata
	attempt.Layer = "channel_attempt"
	t.outputContext, t.attempt = common.BeginAlertSpan(parent, attempt)
}

func (t *RealtimeAlertTracker) Observe(ctx context.Context, kind, status string, hasOutput bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if kind == "response.created" {
		t.begin(ctx)
	}
	// Delta-bearing protocol events represent decoded model output. Session
	// acknowledgements, input transcription and empty keepalives do not.
	if hasOutput && strings.HasPrefix(kind, "response.") && strings.HasSuffix(kind, ".delta") {
		common.ObserveAlertOutput(t.outputContext)
	}
	if kind == "response.done" {
		outcome := "success"
		if status == "failed" {
			outcome = "failure"
		}
		if status == "cancelled" {
			outcome = "ignored"
		}
		t.finish(ctx, outcome)
	}
}

func (t *RealtimeAlertTracker) finish(ctx context.Context, outcome string) {
	t.attempt.End(ctx, outcome)
	t.request.End(ctx, outcome)
	t.attempt, t.request, t.inputAdmitted = nil, nil, false
	t.nativeStartExpected = false
}

func (t *RealtimeAlertTracker) Close(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finish(ctx, "ignored")
}

// NativeResponseStart is used after a provider's VAD boundary. Overlapping
// responses without stable correlation remain visible as unverified coverage.
func (t *RealtimeAlertTracker) NativeResponseStart(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.nativeStartExpected {
		// The provider acknowledges the client operation already recorded at
		// commit; this acknowledgement is not a second overlapping request.
		t.nativeStartExpected = false
		t.begin(ctx)
		return
	}
	// Server VAD without a correlated client boundary does not establish a
	// trustworthy request start. Preserve evidence without claiming TTFT cover.
	t.unverified = true
	if t.attempt != nil {
		t.unverified = true
		t.attempt.MarkAlertCoverageUnverified(ctx)
		t.request.MarkAlertCoverageUnverified(ctx)
	}
	t.begin(ctx)
}
