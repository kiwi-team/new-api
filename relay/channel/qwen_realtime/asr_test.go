package qwen_realtime

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestASRRealtimeRequestURLAndModelLock(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://workspace.cn-beijing.maas.aliyuncs.com",
			UpstreamModelName: "fun-asr-realtime",
		},
	}
	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "wss://workspace.cn-beijing.maas.aliyuncs.com/api-ws/v1/inference", requestURL)

	message := []byte(`{"header":{"action":"run-task","task_id":"task"},"payload":{"model":"client-model","parameters":{"format":"pcm"}}}`)
	rewritten, err := rewriteASRRunTaskModel(message, info.UpstreamModelName)
	require.NoError(t, err)
	var event map[string]any
	require.NoError(t, common.Unmarshal(rewritten, &event))
	payload, ok := event["payload"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "fun-asr-realtime", payload["model"])

	info.ChannelMeta.UpstreamModelName = "qwen3-asr-flash-realtime"
	requestURL, err = (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "wss://workspace.cn-beijing.maas.aliyuncs.com/api-ws/v1/realtime?model=qwen3-asr-flash-realtime", requestURL)
	assert.True(t, isASRRealtimeModel(info.UpstreamModelName))
	assert.False(t, isFunASRRealtimeModel(info.UpstreamModelName))
}

func TestASRRealtimeUsageUsesCumulativeDurationDelta(t *testing.T) {
	accounted := make(map[string]int)
	first, clamp, err := asrUsageDelta([]byte(`{"header":{"task_id":"task","event":"result-generated"},"payload":{"usage":{"duration":3}}}`), accounted)
	require.NoError(t, err)
	require.Nil(t, clamp)
	require.NotNil(t, first)
	assert.Equal(t, 50, first.InputTokenDetails.AudioTokens)

	second, clamp, err := asrUsageDelta([]byte(`{"header":{"task_id":"task","event":"result-generated"},"payload":{"usage":{"duration":9}}}`), accounted)
	require.NoError(t, err)
	require.Nil(t, clamp)
	require.NotNil(t, second)
	assert.Equal(t, 100, second.InputTokenDetails.AudioTokens)

	duplicate, _, err := asrUsageDelta([]byte(`{"header":{"task_id":"task","event":"result-generated"},"payload":{"usage":{"duration":9}}}`), accounted)
	require.NoError(t, err)
	assert.Nil(t, duplicate)

	_, clamp, err = asrUsageDelta([]byte(`{"header":{"task_id":"other","event":"result-generated"},"payload":{"usage":{"duration":1e300}}}`), accounted)
	require.NoError(t, err)
	require.NotNil(t, clamp)
	assert.Equal(t, common.MaxQuota, clamp.Clamped)
}

func TestQwen3ASRUsageTracksCumulativePCMSeconds(t *testing.T) {
	tracker := newQwen3ASRUsageTracker()
	_, _, err := tracker.consume([]byte(`{"type":"session.update","session":{"input_audio_format":"pcm","sample_rate":16000}}`))
	require.NoError(t, err)

	oneSecond := base64.StdEncoding.EncodeToString(make([]byte, 16000*2))
	first, clamp, err := tracker.consume([]byte(`{"type":"input_audio_buffer.append","audio":"` + oneSecond + `"}`))
	require.NoError(t, err)
	require.Nil(t, clamp)
	require.NotNil(t, first)
	assert.Equal(t, 17, first.InputTokens)
	assert.Equal(t, 17, first.InputTokenDetails.AudioTokens)

	second, clamp, err := tracker.consume([]byte(`{"type":"input_audio_buffer.append","audio":"` + oneSecond + `"}`))
	require.NoError(t, err)
	require.Nil(t, clamp)
	require.NotNil(t, second)
	assert.Equal(t, 16, second.InputTokens)
	assert.Equal(t, 16, second.InputTokenDetails.AudioTokens)
}

func TestQwen3ASRUsageRejectsUnbillableAudio(t *testing.T) {
	tracker := newQwen3ASRUsageTracker()
	_, _, err := tracker.consume([]byte(`{"type":"session.update","session":{"input_audio_format":"opus","sample_rate":16000}}`))
	require.ErrorContains(t, err, "unsupported Qwen3 ASR input_audio_format")

	tracker = newQwen3ASRUsageTracker()
	_, _, err = tracker.consume([]byte(`{"type":"session.update","session":{"input_audio_format":"pcm","sample_rate":48000}}`))
	require.ErrorContains(t, err, "unsupported Qwen3 ASR sample_rate")

	_, _, err = tracker.consume([]byte(`{"type":"input_audio_buffer.append","audio":"%%%"}`))
	require.ErrorContains(t, err, "decode Qwen3 ASR PCM audio")
}
