package claude

import (
	"fmt"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// ValidateMessages checks a converted conversation after tool blocks have been
// assembled, without creating synthetic text or user turns.
func ValidateMessages(messages []dto.ClaudeMessage) error {
	if len(messages) == 0 {
		return fmt.Errorf("claude messages request requires at least one message")
	}
	if messages[0].Role != "user" {
		return fmt.Errorf("claude messages request must start with a user message")
	}
	for i, message := range messages {
		switch content := message.Content.(type) {
		case string:
			if content != "" {
				continue
			}
		case []dto.ClaudeMediaMessage:
			if len(content) > 0 {
				continue
			}
		}
		return fmt.Errorf("claude messages[%d].content must not be empty", i)
	}
	return nil
}
