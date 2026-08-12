package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Adaptor-owned prices must not require a duplicate entry in the global model
// price configuration. Legacy LTX models use this path.
func TestResolveTaskBasePriceUsesAdaptorDynamicPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "unconfigured-dynamic-price-model",
		UsingGroup:      "default",
		UserGroup:       "default",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			DynamicModelPrice: 0.195,
		},
	}

	priceData, err := resolveTaskBasePrice(context, info)
	require.NoError(t, err)
	assert.True(t, priceData.UsePrice)
	assert.InDelta(t, 0.195, priceData.ModelPrice, 1e-12)
	assert.Equal(t, 97500, priceData.Quota)
	assert.Equal(t, 1.0, priceData.GroupRatioInfo.GroupRatio)
	assert.Nil(t, info.QuotaClamp)
}

func TestSuccessfulTaskSubmitStatusAcceptsAsyncResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{name: "synchronous success", statusCode: http.StatusOK, want: true},
		{name: "async accepted", statusCode: http.StatusAccepted, want: true},
		{name: "unexpected created response", statusCode: http.StatusCreated, want: false},
		{name: "redirect", statusCode: http.StatusMultipleChoices, want: false},
		{name: "upstream error", statusCode: http.StatusBadGateway, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSuccessfulTaskSubmitStatus(tt.statusCode))
		})
	}
}
