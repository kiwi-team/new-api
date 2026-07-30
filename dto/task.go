package dto

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/constant"
)

// TaskSubmitResult 是一次 task 提交尝试的结果。
// 控制器用它来结算额度、写消费日志并落库任务。
type TaskSubmitResult struct {
	// UpstreamTaskID 是上游返回的真实任务 ID（与对外暴露的 task_xxxx 不同）
	UpstreamTaskID string
	// TaskData 是上游提交响应的原始报文
	TaskData []byte
	// Platform 是本次请求最终判定的 task 平台，会随任务落库供 fetch 阶段派发
	Platform constant.TaskPlatform
	// Quota 是提交后计费调整完成的最终额度
	Quota int
}

type TaskError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Data       any    `json:"data"`
	StatusCode int    `json:"-"`
	LocalError bool   `json:"-"`
	Error      error  `json:"-"`
}

type TaskData interface {
	SunoDataResponse | []SunoDataResponse | string | any
}

const TaskSuccessCode = "success"

type TaskResponse[T TaskData] struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func (t *TaskResponse[T]) IsSuccess() bool {
	return t.Code == TaskSuccessCode
}

type TaskDto struct {
	ID         int64           `json:"id"`
	CreatedAt  int64           `json:"created_at"`
	UpdatedAt  int64           `json:"updated_at"`
	TaskID     string          `json:"task_id"`
	Platform   string          `json:"platform"`
	UserId     int             `json:"user_id"`
	Group      string          `json:"group"`
	ChannelId  int             `json:"channel_id"`
	Quota      int             `json:"quota"`
	Action     string          `json:"action"`
	Status     string          `json:"status"`
	FailReason string          `json:"fail_reason"`
	ResultURL  string          `json:"result_url,omitempty"` // 任务结果 URL（视频地址等）
	SubmitTime int64           `json:"submit_time"`
	StartTime  int64           `json:"start_time"`
	FinishTime int64           `json:"finish_time"`
	Progress   string          `json:"progress"`
	Properties any             `json:"properties"`
	Username   string          `json:"username,omitempty"`
	Data       json.RawMessage `json:"data"`
}

type FetchReq struct {
	IDs []string `json:"ids"`
}
