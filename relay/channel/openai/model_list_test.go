package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestOpenAIModelListIncludesGPTImage25(t *testing.T) {
	models := (&Adaptor{}).GetModelList()
	for _, modelName := range []string{
		"gpt-image-2.5-sunburst",
		"gpt-image-2.5-sunburst-2026-09-08",
		"gpt-image-2.5-flare",
		"gpt-image-2.5-flare-2026-09-08",
	} {
		require.Contains(t, models, modelName)
		require.True(t, common.IsImageGenerationModel(modelName))
	}
}
