package middleware

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ValidateClaudeMessagesOutput rejects unsupported structured-output constraints
// before channel selection, including channel races and passthrough forwarding.
func ValidateClaudeMessagesOutput() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost || c.Request.URL.Path != "/v1/messages" {
			c.Next()
			return
		}

		var request struct {
			OutputConfig struct {
				Format struct {
					Type   string         `json:"type"`
					Schema map[string]any `json:"schema"`
				} `json:"format"`
			} `json:"output_config"`
		}
		status := http.StatusBadRequest
		var problems []string
		if err := common.UnmarshalBodyReusable(c, &request); err != nil {
			status = invalidRequestStatus(err)
			problems = []string{err.Error()}
		} else if request.OutputConfig.Format.Type == "json_schema" {
			if request.OutputConfig.Format.Schema == nil {
				problems = []string{"output_config.format.schema: must be a JSON object"}
			} else {
				problems = validateClaudeOutputSchema(request.OutputConfig.Format.Schema, "output_config.format.schema")
			}
		}
		if len(problems) == 0 {
			c.Next()
			return
		}

		message := i18n.Translate(i18n.LangZhCN, i18n.MsgRelayInvalidRequestFormat) + " / " +
			i18n.Translate(i18n.LangEn, i18n.MsgRelayInvalidRequestFormat) + ": " + strings.Join(problems, "; ")
		apiErr := types.WithClaudeError(types.ClaudeError{
			Type:    "invalid_request_error",
			Message: message,
		}, status, types.ErrOptionWithSkipRetry())
		apiErr = service.ClientError(c, apiErr)
		c.AbortWithStatusJSON(status, gin.H{"type": "error", "error": apiErr.ToClaudeError()})
	}
}

// Anthropic accepts only a subset of JSON Schema constraints:
// https://platform.claude.com/docs/en/build-with-claude/structured-outputs#json-schema-limitations
func validateClaudeOutputSchema(schema map[string]any, path string) []string {
	var problems []string
	for _, keyword := range []string{
		"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf",
		"minLength", "maxLength", "maxItems",
	} {
		if _, exists := schema[keyword]; exists {
			problems = append(problems, fmt.Sprintf("%s.%s: '%s' is not supported by Anthropic structured outputs", path, keyword, keyword))
		}
	}
	if value, exists := schema["minItems"]; exists && value != float64(0) && value != float64(1) {
		problems = append(problems, fmt.Sprintf("%s.minItems: only 0 or 1 is supported by Anthropic structured outputs (got %v)", path, value))
	}

	// Visit schema-bearing keywords only. Property names and values in default,
	// enum, examples or descriptions are application data, not constraints.
	for _, keyword := range []string{"properties", "$defs", "definitions"} {
		children, _ := schema[keyword].(map[string]any)
		for _, name := range slices.Sorted(maps.Keys(children)) {
			if child, ok := children[name].(map[string]any); ok {
				problems = append(problems, validateClaudeOutputSchema(child, path+"."+keyword+"."+name)...)
			}
		}
	}
	for _, keyword := range []string{"items", "prefixItems", "anyOf", "allOf", "oneOf"} {
		switch children := schema[keyword].(type) {
		case map[string]any:
			problems = append(problems, validateClaudeOutputSchema(children, path+"."+keyword)...)
		case []any:
			for i, value := range children {
				if child, ok := value.(map[string]any); ok {
					problems = append(problems, validateClaudeOutputSchema(child, fmt.Sprintf("%s.%s[%d]", path, keyword, i))...)
				}
			}
		}
	}
	return problems
}
