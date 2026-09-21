package volcengine

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSeedASRAuth(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		appKey     string
		accessKey  string
		modernKey  string
		shouldFail bool
	}{
		{name: "modern", key: "api-key", modernKey: "api-key"},
		{name: "legacy", key: "app-key|access-key", appKey: "app-key", accessKey: "access-key"},
		{name: "empty", shouldFail: true},
		{name: "missing legacy key", key: "app-key|", shouldFail: true},
		{name: "too many legacy fields", key: "app|access|extra", shouldFail: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			appKey, accessKey, modernKey, err := parseSeedASRAuth(test.key)
			if test.shouldFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.appKey, appKey)
			assert.Equal(t, test.accessKey, accessKey)
			assert.Equal(t, test.modernKey, modernKey)
		})
	}
}

func TestSetupSeedASRStreamingHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	header := http.Header{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "api-key"}}

	require.NoError(t, setupSeedASRStreamingHeaders(context, header, info))
	assert.Equal(t, "api-key", header.Get("X-Api-Key"))
	assert.Equal(t, seedASRStreamingResourceID, header.Get("X-Api-Resource-Id"))
	assert.Equal(t, "-1", header.Get("X-Api-Sequence"))
	assert.NotEmpty(t, header.Get("X-Api-Request-Id"))
	assert.Equal(t, header.Get("X-Api-Request-Id"), header.Get("X-Api-Connect-Id"))
}

func TestVolcengineRealtimeProtocolUsesMappedModel(t *testing.T) {
	adaptor := &RealtimeAdaptor{}
	tests := []struct {
		name    string
		model   string
		path    string
		wantURL string
		wantErr bool
	}{
		{
			name:    "Seed ASR on unified path",
			model:   "doubao-seed-asr-2.0-streaming",
			path:    "/v1/realtime?model=doubao-seed-asr-2.0-streaming",
			wantURL: seedASRStreamingURL,
		},
		{
			name:    "Dialogue on unified path",
			model:   "doubao-realtime",
			path:    "/v1/realtime?model=doubao-realtime",
			wantURL: doubaoRealtimeDialogueURL,
		},
		{
			name:    "Model overrides legacy ASR path",
			model:   "doubao-realtime",
			path:    "/v1/realtime/volcengine/asr?model=doubao-realtime",
			wantURL: doubaoRealtimeDialogueURL,
		},
		{
			name:    "Unsupported model",
			model:   "unknown-realtime-model",
			path:    "/v1/realtime",
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				RequestURLPath: test.path,
				ChannelMeta:    &relaycommon.ChannelMeta{UpstreamModelName: test.model},
			}
			requestURL, err := adaptor.GetRequestURL(info)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantURL, requestURL)
		})
	}
}

func TestParseSeedASRMetadataValidation(t *testing.T) {
	tests := []struct {
		name       string
		metadata   string
		shouldFail bool
	}{
		{name: "allowed", metadata: `{"enable_itn":false,"end_window_size":300,"bits":16,"channel":2,"corpus":{"context":"hotwords"}}`},
		{name: "unknown top-level", metadata: `{"callback":"https://example.com"}`, shouldFail: true},
		{name: "unknown corpus", metadata: `{"corpus":{"callback":"https://example.com"}}`, shouldFail: true},
		{name: "invalid end window", metadata: `{"end_window_size":299}`, shouldFail: true},
		{name: "invalid channels", metadata: `{"channel":3}`, shouldFail: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			form := &multipart.Form{Value: map[string][]string{"metadata": {test.metadata}}}
			metadata, err := parseSeedASRMetadata(form)
			if test.shouldFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, metadata.EnableITN)
			assert.False(t, *metadata.EnableITN, "explicit false must be preserved")
		})
	}
}

func TestSeedASRFrameDuration(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte(`{"audio_info":{"duration":12345},"result":{"text":"hello"}}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	payload := compressed.Bytes()
	frameBytes := make([]byte, 8+len(payload))
	frameBytes[0] = 0x11 // protocol v1, four-byte header
	frameBytes[1] = 0x90 // full server response, no sequence
	frameBytes[2] = 0x11 // JSON serialization, gzip compression
	binary.BigEndian.PutUint32(frameBytes[4:8], uint32(len(payload)))
	copy(frameBytes[8:], payload)

	frame, err := parseSeedASRFrame(frameBytes)
	require.NoError(t, err)
	duration, ok := seedASRFrameDuration(frame)
	require.True(t, ok)
	assert.Equal(t, int64(12345), duration)

	frameBytes[7]++
	_, err = parseSeedASRFrame(frameBytes)
	require.Error(t, err, "declared payload length must match the WebSocket message")
}

func TestWriteSeedASRResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	result := &seedASRFileResponse{}
	result.AudioInfo.Duration = 2250
	result.Result.Text = "hello world"
	result.Result.Utterances = []seedASRUtterance{{Text: "hello", StartTime: 0, EndTime: 1000}, {Text: "world", StartTime: 1250, EndTime: 2250}}

	tests := []struct {
		format      string
		contentType string
		contains    string
	}{
		{format: "json", contentType: "application/json", contains: `"text":"hello world"`},
		{format: "verbose_json", contentType: "application/json", contains: `"duration":2.25`},
		{format: "srt", contentType: "text/plain", contains: "00:00:01,250 --> 00:00:02,250"},
		{format: "vtt", contentType: "text/plain", contains: "WEBVTT"},
	}
	for _, test := range tests {
		t.Run(test.format, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			info := &relaycommon.RelayInfo{Request: &dto.AudioRequest{ResponseFormat: test.format}}
			require.NoError(t, writeSeedASRResponse(context, info, result))
			assert.Equal(t, 200, recorder.Code)
			assert.Contains(t, recorder.Header().Get("Content-Type"), test.contentType)
			assert.Contains(t, recorder.Body.String(), test.contains)
		})
	}
}
