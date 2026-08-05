package ltx

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LTX 没有真实的上游任务 ID：DoRequest 返回的是本地 mock。该 mock 的 task_id
// 必须是预生成的公开 ID（task_xxxx），因为它会被 RelayTaskSubmit 当作「上游 ID」
// 存进 PrivateData.UpstreamTaskID，并在轮询时回传给 FetchTask 按 task_id 查行。
// 一旦这里换成随机串，落库的 task_id 与之对不上，任务永远查不到也不会被推进。
func TestDoRequestMockCarriesPublicTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_abcdefg"},
	}

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.DoRequest(c, info, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var mock LtxTaskResponse
	require.NoError(t, common.Unmarshal(body, &mock))
	assert.Equal(t, "task_abcdefg", mock.TaskID)
	assert.Equal(t, string(model.TaskStatusSubmitted), mock.Status)
}
