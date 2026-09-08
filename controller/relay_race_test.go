package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRaceResponseWriterWaitsForMeaningfulSSEData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	parent, _ := gin.CreateTestContext(recorder)
	coordinator := &raceCoordinator{winnerCh: make(chan raceWinner, 1)}
	child := parent.Copy()
	done := make(chan struct{})
	writer := newRaceResponseWriter(parent.Writer, coordinator, &raceLogState{}, "req-1", raceWinner{index: 0, channelId: 11, done: done}, child)
	writer.Header().Set("Content-Type", "text/event-stream")

	_, err := writer.WriteString(": PING\n\n")
	require.NoError(t, err)
	assert.Empty(t, recorder.Body.String())
	select {
	case <-coordinator.winnerCh:
		t.Fatal("SSE comment must not select a winner")
	default:
	}

	_, err = writer.WriteString("event: message\ndata: {\"ok\":true}\n\n")
	require.NoError(t, err)
	winner := <-coordinator.winnerCh
	assert.Equal(t, 11, winner.channelId)
	assert.Equal(t, "winner", child.GetString(common.KeyChannelRaceResult))
	assert.Contains(t, recorder.Body.String(), `data: {"ok":true}`)
}

func TestRaceResponseWriterOnlyForwardsOneWinner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	parent, _ := gin.CreateTestContext(recorder)
	coordinator := &raceCoordinator{winnerCh: make(chan raceWinner, 1)}
	state := &raceLogState{}

	firstContext := parent.Copy()
	first := newRaceResponseWriter(parent.Writer, coordinator, state, "req-1", raceWinner{index: 0, channelId: 11, done: make(chan struct{})}, firstContext)
	secondContext := parent.Copy()
	second := newRaceResponseWriter(parent.Writer, coordinator, state, "req-1", raceWinner{index: 1, channelId: 12, done: make(chan struct{})}, secondContext)

	_, err := first.WriteString(`{"channel":11}`)
	require.NoError(t, err)
	_, err = second.WriteString(`{"channel":12}`)
	require.NoError(t, err)

	assert.JSONEq(t, `{"channel":11}`, recorder.Body.String())
	assert.Equal(t, "winner", firstContext.GetString(common.KeyChannelRaceResult))
	assert.Equal(t, "loser", secondContext.GetString(common.KeyChannelRaceResult))
}

func TestRaceCoordinatorWaitsForSlowStartedAttemptBeforeFailing(t *testing.T) {
	coordinator := &raceCoordinator{winnerCh: make(chan raceWinner, 1)}

	// The final candidate can fail while an earlier timed-out candidate is still
	// running. That is not terminal failure because the earlier request may yet
	// return valid data.
	assert.False(t, coordinator.markAttemptDone(2))
	winner := raceWinner{index: 0, channelId: 48, done: make(chan struct{})}
	assert.True(t, coordinator.tryWin(winner))
	assert.Equal(t, 48, (<-coordinator.winnerCh).channelId)
	assert.False(t, coordinator.markAttemptDone(2))
}

func TestRaceCoordinatorFailsOnlyAfterEveryAttemptFinishes(t *testing.T) {
	coordinator := &raceCoordinator{winnerCh: make(chan raceWinner, 1)}

	assert.False(t, coordinator.markAttemptDone(3))
	assert.False(t, coordinator.markAttemptDone(3))
	assert.True(t, coordinator.markAttemptDone(3))
	assert.False(t, coordinator.tryWin(raceWinner{index: 0, channelId: 48}))
}

func TestValidateTokenChannelRulesRaceTimeout(t *testing.T) {
	require.NoError(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"race"}}`))
	require.NoError(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"race","race_timeout":25}}`))
	assert.Error(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"race","race_timeout":301}}`))
	assert.Error(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"unsupported"}}`))
}

func TestRaceProtocolAllowlist(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	context.Request.Header.Set("Content-Type", "application/json")
	assert.True(t, isRaceSupported(context, types.RelayFormatOpenAI))

	context.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	context.Request.Header.Set("Content-Type", "application/json")
	assert.False(t, isRaceSupported(context, types.RelayFormatOpenAIImage))

	context.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	context.Request.Header.Set("Content-Type", "application/json")
	assert.False(t, isRaceSupported(context, types.RelayFormatOpenAIRealtime))

	context.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	context.Request.Header.Set("Content-Type", "multipart/form-data")
	assert.False(t, isRaceSupported(context, types.RelayFormatOpenAI))
}
