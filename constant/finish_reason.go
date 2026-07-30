package constant

import "github.com/QuantumNous/new-api/relaykit/types"

// Finish reasons moved to types with the conversion kit.
var (
	FinishReasonStop          = types.FinishReasonStop
	FinishReasonToolCalls     = types.FinishReasonToolCalls
	FinishReasonLength        = types.FinishReasonLength
	FinishReasonFunctionCall  = types.FinishReasonFunctionCall
	FinishReasonContentFilter = types.FinishReasonContentFilter
)

// ErrorFinishReasons contains finish_reason values that indicate upstream errors
// disguised as normal responses (e.g. context window exceeded with empty content).
// Requests with these finish_reasons should be treated as failures.
var ErrorFinishReasons = map[string]bool{
	"model_context_window_exceeded": true,
	"sensitive":                     true,
}
