package openrouter

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ConvertDecisionsRequest bridges the OpenAI array schema to OpenRouter's
// named System One questions. Index keys preserve duplicate/absent names and
// typed choice values (false and "false" are distinct OpenAI choices).
func ConvertDecisionsRequest(c *gin.Context, request *dto.DecisionsRequest) (*dto.TypeSafeRequest, error) {
	if request.PromptCacheKey != nil || (len(request.PromptCacheOptions) > 0 && string(request.PromptCacheOptions) != "null") {
		return nil, errors.New("OpenRouter Decisions does not support prompt_cache_key or prompt_cache_options; use a native OpenAI channel")
	}
	var input any
	if err := common.Unmarshal(request.Input, &input); err != nil {
		return nil, err
	}
	state := input
	if messages, ok := input.([]any); ok {
		parts := make([]any, 0)
		for _, message := range messages {
			item, ok := message.(map[string]any)
			if !ok || item["role"] != "user" || (item["type"] != nil && item["type"] != "message") || item["phase"] != nil {
				return nil, errors.New("OpenRouter Decisions supports only user messages; use a native OpenAI channel for roles or tool history")
			}
			if text, ok := item["content"].(string); ok {
				parts = append(parts, text)
				continue
			}
			content, ok := item["content"].([]any)
			if !ok {
				return nil, errors.New("message content must be text or an array of input parts")
			}
			for _, value := range content {
				part, ok := value.(map[string]any)
				if !ok {
					return nil, errors.New("input parts must be objects")
				}
				if part["prompt_cache_breakpoint"] != nil {
					return nil, errors.New("OpenRouter Decisions does not support prompt_cache_breakpoint; use a native OpenAI channel")
				}
				switch part["type"] {
				case "input_text":
					text, ok := part["text"].(string)
					if !ok {
						return nil, errors.New("input_text requires text")
					}
					parts = append(parts, text)
				case "input_image":
					url, ok := part["image_url"].(string)
					if !ok || url == "" {
						return nil, errors.New("input_image requires image_url")
					}
					detail, _ := part["detail"].(string)
					if detail != "" && detail != "auto" && detail != "low" && detail != "high" {
						return nil, errors.New("OpenRouter Decisions supports image detail auto, low or high")
					}
					data, mime, err := service.GetBase64Data(c, types.NewFileSourceFromData(url, ""), "decisions_image")
					if err != nil {
						return nil, fmt.Errorf("could not load Decisions image: %w", err)
					}
					if mime != "image/png" && mime != "image/jpeg" && mime != "image/webp" {
						return nil, errors.New("OpenRouter Decisions images must be PNG, JPEG or WebP")
					}
					image := map[string]any{"url": "data:" + mime + ";base64," + data}
					if detail != "" {
						image["detail"] = detail
					}
					parts = append(parts, map[string]any{"type": "image_url", "image_url": image})
				default:
					return nil, errors.New("OpenRouter Decisions supports input_text and input_image parts")
				}
			}
		}
		state = parts
	}
	stateJSON, err := common.Marshal(state)
	if err != nil {
		return nil, err
	}
	converted := &dto.TypeSafeRequest{Model: request.Model, State: stateJSON, User: request.SafetyIdentifier,
		Questions: make(map[string]json.RawMessage, len(request.Questions))}
	for i, question := range request.Questions {
		value := map[string]any{"type": question.Type, "instructions": question.Instructions}
		switch question.Type {
		case "predicate":
			value["type"] = "noul"
		case "choice":
			criteria := make(map[string]any, len(question.Choices))
			for j, choice := range question.Choices {
				// Retain the actual typed value in the guidance, while using a
				// collision-free wire key even for bool/string lookalikes.
				criteria[strconv.Itoa(j)] = choice
			}
			value["criteria"] = criteria
		case "score":
			value["criteria"] = question.Levels
		default:
			return nil, fmt.Errorf("unsupported question type %q", question.Type)
		}
		encoded, err := common.Marshal(value)
		if err != nil {
			return nil, err
		}
		converted.Questions[strconv.Itoa(i)] = encoded
	}
	return converted, nil
}

type decisionAnswer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul"`
	Choice        *string            `json:"choice"`
	Score         *float64           `json:"score"`
	Confidence    *float64           `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// ConvertDecisionsResponse restores question/option order and OpenAI's typed
// values. Missing probabilities are an upstream error, never invented zeros.
func ConvertDecisionsResponse(body []byte, request *dto.DecisionsRequest, clientModel string) (*dto.DecisionsResponse, error) {
	var upstream struct {
		Answers map[string]decisionAnswer `json:"answers"`
		Usage   *dto.DecisionUsage        `json:"usage"`
	}
	if err := common.Unmarshal(body, &upstream); err != nil {
		return nil, err
	}
	if len(upstream.Answers) != len(request.Questions) {
		return nil, errors.New("Decisions response answer count does not match questions")
	}
	response := &dto.DecisionsResponse{Model: clientModel, Usage: upstream.Usage, Answers: make([]dto.DecisionAnswer, len(request.Questions))}
	for i, question := range request.Questions {
		answer, ok := upstream.Answers[strconv.Itoa(i)]
		if !ok {
			return nil, fmt.Errorf("missing Decisions answer %d", i)
		}
		out := dto.DecisionAnswer{Type: question.Type, Name: question.Name}
		if answer.Type == "refusal" {
			out.Type = "refusal"
			response.Answers[i] = out
			continue
		}
		expectedType := question.Type
		if expectedType == "predicate" {
			expectedType = "noul"
		}
		if answer.Type != expectedType {
			return nil, fmt.Errorf("unexpected Decisions answer type for question %d", i)
		}
		if question.Type == "predicate" {
			if !validDecisionProbability(answer.Noul) {
				return nil, errors.New("missing or invalid noul probability")
			}
			out.Probability = answer.Noul
			response.Answers[i] = out
			continue
		}
		if !validDecisionProbability(answer.Confidence) {
			return nil, errors.New("missing or invalid Decisions confidence")
		}
		out.Confidence = answer.Confidence
		count := len(question.Levels)
		if question.Type == "choice" {
			count = len(question.Choices)
			if answer.Choice == nil {
				return nil, errors.New("missing Decisions choice")
			}
			index, err := strconv.Atoi(*answer.Choice)
			if err != nil || index < 0 || index >= count || strconv.Itoa(index) != *answer.Choice {
				return nil, errors.New("unknown Decisions choice")
			}
			out.Choice = question.Choices[index].Value
		} else {
			if answer.Score == nil || math.IsNaN(*answer.Score) || *answer.Score < 0 || *answer.Score > float64(count-1) {
				return nil, errors.New("missing or invalid Decisions score")
			}
			out.Score = answer.Score
		}
		if len(answer.Probabilities) != count {
			return nil, errors.New("Decisions probability count does not match options")
		}
		out.Probabilities = make([]dto.DecisionProbability, count)
		for j := range count {
			probability, ok := answer.Probabilities[strconv.Itoa(j)]
			if !ok || !validDecisionProbability(&probability) {
				return nil, errors.New("missing or invalid Decisions option probability")
			}
			entry := dto.DecisionProbability{Value: j, Probability: probability}
			if question.Type == "choice" {
				entry.Value = question.Choices[j].Value
			} else {
				entry.Label = question.Levels[j].Label
			}
			out.Probabilities[j] = entry
		}
		response.Answers[i] = out
	}
	return response, nil
}

func validDecisionProbability(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && *value >= 0 && *value <= 1
}
