package helper

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

func GetAndValidateDecisionsRequest(c *gin.Context) (*dto.DecisionsRequest, error) {
	request := &dto.DecisionsRequest{}
	if err := common.UnmarshalBodyReusable(c, request); err != nil {
		return nil, err
	}
	if err := ValidateDecisionsRequest(request); err != nil {
		return nil, err
	}
	return request, nil
}

// ValidateDecisionsRequest also runs after channel parameter overrides, before
// conversion. Decisions has no max_tokens, stream or batch multiplier fields.
func ValidateDecisionsRequest(request *dto.DecisionsRequest) error {
	if request.Model == "" {
		return errors.New("model is required")
	}
	if len(request.Questions) < 1 || len(request.Questions) > 200 {
		return errors.New("questions must contain between 1 and 200 questions")
	}
	for i, question := range request.Questions {
		if question.Instructions == nil {
			return fmt.Errorf("questions[%d].instructions is required", i)
		}
		switch question.Type {
		case "predicate":
		case "choice":
			if len(question.Choices) < 2 || len(question.Choices) > 255 {
				return fmt.Errorf("questions[%d].choices must contain between 2 and 255 choices", i)
			}
			seen := make(map[any]bool, len(question.Choices))
			for _, choice := range question.Choices {
				switch choice.Value.(type) {
				case string, bool:
				default:
					return fmt.Errorf("questions[%d] choice values must be strings or booleans", i)
				}
				if seen[choice.Value] {
					return fmt.Errorf("questions[%d] choice values must be unique", i)
				}
				seen[choice.Value] = true
			}
		case "score":
			if len(question.Levels) == 0 {
				return fmt.Errorf("questions[%d].levels is required", i)
			}
			for _, level := range question.Levels {
				if level.Label == nil {
					return fmt.Errorf("questions[%d] level labels are required", i)
				}
			}
		default:
			return fmt.Errorf("questions[%d].type must be predicate, choice or score", i)
		}
	}

	var input any
	if err := common.Unmarshal(request.Input, &input); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if _, ok := input.(string); ok {
		return nil
	}
	items, ok := input.([]any)
	if !ok {
		return errors.New("input must be a string or an array")
	}
	images := 0
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return errors.New("input items must be objects")
		}
		// Both messages and tool outputs can contain images. Other protocol
		// validation is left to the native upstream without rewriting history.
		for _, field := range []string{"content", "output"} {
			parts, _ := object[field].([]any)
			for _, part := range parts {
				if content, ok := part.(map[string]any); ok && content["type"] == "input_image" {
					images++
				}
			}
		}
	}
	if images > 128 {
		return errors.New("input must contain at most 128 images")
	}
	return nil
}
