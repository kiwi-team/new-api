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
}
