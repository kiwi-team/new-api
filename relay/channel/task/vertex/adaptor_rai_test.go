package vertex

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// operation done 但没有视频产物时必须判失败：以前这里会落到成功分支，
// 导致内容安全拦截既不退款、下游也拿不到视频地址。
func TestParseTaskResultDoneWithoutVideoFails(t *testing.T) {
	a := &TaskAdaptor{}
	cases := []struct {
		name       string
		body       string
		wantReason string
	}{
		{
			name:       "rai filtered with reasons",
			body:       `{"name":"op/1","done":true,"response":{"raiMediaFilteredCount":1,"raiMediaFilteredReasons":["58061214","29310472"]}}`,
			wantReason: "58061214; 29310472",
		},
		{
			name:       "rai filtered without reasons",
			body:       `{"name":"op/1","done":true,"response":{"raiMediaFilteredCount":1}}`,
			wantReason: "video generation blocked by safety filter",
		},
		{
			name:       "no video and no filter",
			body:       `{"name":"op/1","done":true,"response":{}}`,
			wantReason: "operation done but no video returned",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ti, err := a.ParseTaskResult([]byte(tc.body))
			require.NoError(t, err)
			assert.Equal(t, model.TaskStatusFailure, ti.Status)
			assert.Equal(t, tc.wantReason, ti.Reason)
		})
	}
}
