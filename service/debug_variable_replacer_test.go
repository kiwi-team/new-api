package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// {{var}} 在请求体模板里代表一个完整的 JSON 值，替换结果自带引号/null，
// 因此模板不能再给占位符补引号，否则会产生 ""gpt-4"" 这样的非法 JSON。
// 路径模板里的 {var} 走 ReplaceInPath，那里才是不带引号的原始值。
func TestVariableReplacer_ReplaceSimple(t *testing.T) {
	variables := map[string]interface{}{
		"model":       "gpt-4",
		"temperature": 0.7,
		"stream":      true,
		"stop":        nil,
	}

	replacer := NewVariableReplacer(variables, nil)

	template := `{"model":{{model}},"temperature":{{temperature}},"stream":{{stream}},"stop":{{stop}},"kept":{{missing}}}`
	result, err := replacer.Replace(template)

	require.NoError(t, err)
	assert.Equal(t, `{"model":"gpt-4","temperature":0.7,"stream":true,"stop":null,"kept":{{missing}}}`, result)
}

func TestVariableReplacer_ReplacePrevResponse(t *testing.T) {
	variables := map[string]interface{}{}
	prevResponse := map[string]interface{}{
		"id": "msg_123",
		"content": []interface{}{
			map[string]interface{}{
				"text": "Hello, world!",
			},
		},
	}

	replacer := NewVariableReplacer(variables, prevResponse)

	template := `{"conversation_id":{{prev_response.id}},"previous_text":{{prev_response.content.0.text}},"missing":{{prev_response.usage.total_tokens}}}`
	result, err := replacer.Replace(template)

	require.NoError(t, err)
	assert.Equal(t, `{"conversation_id":"msg_123","previous_text":"Hello, world!","missing":{{prev_response.usage.total_tokens}}}`, result)
}

func TestVariableReplacer_ReplaceInPath(t *testing.T) {
	variables := map[string]interface{}{
		"model_name": "gpt-4",
		"version":    "v1",
	}

	replacer := NewVariableReplacer(variables, nil)

	path := "/{version}/models/{model_name}/info"
	result, err := replacer.ReplaceInPath(path)

	if err != nil {
		t.Errorf("ReplaceInPath failed: %v", err)
	}

	expected := "/v1/models/gpt-4/info"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestExtractVariables(t *testing.T) {
	template := `{"model":"{{model_name}}","temperature":{{temperature}},"stream":{{stream}}}`
	variables := ExtractVariables(template)

	expected := []string{"model_name", "temperature", "stream"}
	if len(variables) != len(expected) {
		t.Errorf("Expected %d variables, got %d", len(expected), len(variables))
	}

	for i, v := range variables {
		if v != expected[i] {
			t.Errorf("Expected variable %s, got %s", expected[i], v)
		}
	}
}

func TestExtractPathVariables(t *testing.T) {
	path := "/v1/models/{model_name}/versions/{version_id}"
	variables := ExtractPathVariables(path)

	expected := []string{"model_name", "version_id"}
	if len(variables) != len(expected) {
		t.Errorf("Expected %d variables, got %d", len(expected), len(variables))
	}

	for i, v := range variables {
		if v != expected[i] {
			t.Errorf("Expected variable %s, got %s", expected[i], v)
		}
	}
}

func TestConvertType(t *testing.T) {
	tests := []struct {
		value      interface{}
		targetType string
		expected   interface{}
		shouldFail bool
	}{
		{"123", "number", 123.0, false},
		{"true", "boolean", true, false},
		{123, "string", "123", false},
		{"invalid", "number", nil, true},
	}

	for _, test := range tests {
		result, err := ConvertType(test.value, test.targetType)
		if test.shouldFail {
			if err == nil {
				t.Errorf("Expected error for %v -> %s", test.value, test.targetType)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != test.expected {
				t.Errorf("Expected %v, got %v", test.expected, result)
			}
		}
	}
}
