package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"

	"github.com/stretchr/testify/assert"
)

func TestLtx25ModelsAreAvailableInChannelModelCatalog(t *testing.T) {
	models := channelId2Models[constant.ChannelTypeLtx]
	assert.Contains(t, models, "ltx-2-5-fast")
	assert.Contains(t, models, "ltx-2-5-pro")

	assert.Equal(t, "ltx", openAIModelsMap["ltx-2-5-fast"].OwnedBy)
	assert.Equal(t, "ltx", openAIModelsMap["ltx-2-5-pro"].OwnedBy)
}
