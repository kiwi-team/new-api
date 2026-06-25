package dto

type ErrorLogsRequest struct {
	RequestId    string `json:"request_id,omitempty"`
	ChannelId    int    `json:"channel_id,omitempty"`
	ModelName    string `json:"model_name,omitempty"`
	StartTime    int64  `json:"start_time,omitempty"`
	EndTime      int64  `json:"end_time,omitempty"`
	Page         int    `json:"page,omitempty"`
	PageSize     int    `json:"page_size,omitempty"`
	TokenId      int    `json:"token_id,omitempty"`
	ClientUserId string `json:"client_user_id,omitempty"`
	MtSessionId  string `json:"mt_session_id,omitempty"`
	TraceId      string `json:"trace_id,omitempty"`
	TrajId       string `json:"traj_id,omitempty"`
	SessionId    string `json:"session_id,omitempty"`
	// 自助视图范围限制（非管理员）：限定为 (user_id = ScopeUserId OR client_user_id IN ScopeUids) 的并集。
	// ScopeUserId<=0 时不限制（管理员可见全部）。这两个字段由控制器设置，不来自查询参数。
	ScopeUserId int      `json:"-"`
	ScopeUids   []string `json:"-"`
}
