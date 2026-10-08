package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertRequestPreservesGinContextForMediaResolution(t *testing.T) {
	previousDebug := common.DebugEnabled
	common.DebugEnabled = true
	t.Cleanup(func() {
		common.DebugEnabled = previousDebug
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	maxTokens := uint(2000)
	request := &dto.GeneralOpenAIRequest{
		Model:     "claude-haiku-4-5",
		MaxTokens: &maxTokens,
		Messages: []dto.Message{{
			Role: "user",
			Content: []dto.MediaContent{
				{Type: dto.ContentTypeText, Text: "describe this image"},
				{Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{
					Url: "data:image/png;base64,aGVsbG8=",
				}},
			},
		}},
	}

	result, err := ConvertRequest(c, &relaycommon.RelayInfo{}, types.RelayFormatClaude, request)
	require.NoError(t, err)

	converted, ok := result.Value.(*dto.ClaudeRequest)
	require.True(t, ok)
	require.Len(t, converted.Messages, 1)
	content, err := converted.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, content, 2)
	assert.Equal(t, "image", content[1].Type)
	require.NotNil(t, content[1].Source)
	assert.Equal(t, "image/png", content[1].Source.MediaType)
	assert.Equal(t, "aGVsbG8=", content[1].Source.Data)
}
