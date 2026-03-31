package constant

var (
	FinishReasonStop          = "stop"
	FinishReasonToolCalls     = "tool_calls"
	FinishReasonLength        = "length"
	FinishReasonFunctionCall  = "function_call"
	FinishReasonContentFilter = "content_filter"
)

// ErrorFinishReasons contains finish_reason values that indicate upstream errors
// disguised as normal responses (e.g. context window exceeded with empty content).
// Requests with these finish_reasons should be treated as failures.
var ErrorFinishReasons = map[string]bool{
	"model_context_window_exceeded": true,
	"sensitive":                     true,
}
