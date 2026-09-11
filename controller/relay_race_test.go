package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	hostdto "github.com/QuantumNous/new-api/dto"
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

func TestSelectRaceGroupChannelsStrategies(t *testing.T) {
	reverse := func(ids []int) {
		for left, right := 0, len(ids)-1; left < right; left, right = left+1, right-1 {
			ids[left], ids[right] = ids[right], ids[left]
		}
	}

	tests := []struct {
		name        string
		group       hostdto.ChannelRaceGroup
		want        []int
		wantShuffle bool
	}{
		{
			name:        "legacy group defaults to random polling",
			group:       hostdto.ChannelRaceGroup{ChannelIds: []int{1, 2, 3}},
			want:        []int{3, 2, 1},
			wantShuffle: true,
		},
		{
			name:  "ordered polling preserves configured order",
			group: hostdto.ChannelRaceGroup{ChannelIds: []int{1, 2, 3}, Mode: hostdto.ChannelRaceGroupModeOrder},
			want:  []int{1, 2, 3},
		},
		{
			name:        "random n samples the requested candidate count",
			group:       hostdto.ChannelRaceGroup{ChannelIds: []int{1, 2, 3, 4}, Mode: hostdto.ChannelRaceGroupModeRandomN, RandomCount: 2},
			want:        []int{4, 3},
			wantShuffle: true,
		},
		{
			name:        "random n defaults to one candidate",
			group:       hostdto.ChannelRaceGroup{ChannelIds: []int{1, 2, 3}, Mode: hostdto.ChannelRaceGroupModeRandomN},
			want:        []int{3},
			wantShuffle: true,
		},
		{
			name:        "random n above group size uses every candidate",
			group:       hostdto.ChannelRaceGroup{ChannelIds: []int{1, 2, 3}, Mode: hostdto.ChannelRaceGroupModeRandomN, RandomCount: 10},
			want:        []int{3, 2, 1},
			wantShuffle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuffled := false
			shuffle := func(ids []int) {
				shuffled = true
				reverse(ids)
			}

			assert.Equal(t, tt.want, selectRaceGroupChannels(tt.group, shuffle))
			assert.Equal(t, tt.wantShuffle, shuffled)
		})
	}
}

func TestValidateTokenChannelRulesRaceTimeout(t *testing.T) {
	require.NoError(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"race"}}`))
	require.NoError(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"race","race_timeout":25}}`))
	assert.Error(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"race","race_timeout":301}}`))
	assert.Error(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"unsupported"}}`))
	require.NoError(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"race","channels":[{"ids":[1,2],"race_mode":"order"},{"ids":[3,4],"race_mode":"random_n","race_count":1}]}}`))
	require.NoError(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"order","channels":[{"ids":[1,2],"race_mode":"order"}]}}`))
	require.NoError(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"random","channels":[{"ids":[1,2],"race_mode":"random_n","race_count":1}]}}`))
	assert.Error(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"race","channels":[{"ids":[1],"race_mode":"unsupported"}]}}`))
	assert.Error(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"order","channels":[{"ids":[1],"race_mode":"unsupported"}]}}`))
	assert.Error(t, validateTokenChannelRules(`{"gpt-4o":{"random_type":"race","channels":[{"ids":[1],"race_mode":"random_n","race_count":-1}]}}`))
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
