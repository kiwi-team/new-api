package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeMessagesOutputValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	previousSave, previousOptions := common.SaveErrorLog, common.OptionMap
	common.SaveErrorLog, common.OptionMap = false, map[string]string{}
	t.Cleanup(func() { common.SaveErrorLog, common.OptionMap = previousSave, previousOptions })

	const prefix = "output_config.format.schema"
	type testCase struct {
		name      string
		config    string
		path      string
		body      string
		bodyLimit int64
		status    int
		problems  []string
	}
	cases := []testCase{
		{
			name: "reported confidence and steps constraints",
			config: `{"format":{"type":"json_schema","schema":{"type":"object","properties":{
				"probe_no_tool":{"type":"object","properties":{"confidence":{"type":"number","minimum":0,"maximum":1}},"required":["confidence"],"additionalProperties":false},
				"steps":{"type":"array","minItems":3,"maxItems":3,"items":{"type":"string"}}
			},"required":["probe_no_tool","steps"],"additionalProperties":false}}}`,
			status: http.StatusBadRequest,
			problems: []string{
				prefix + ".properties.probe_no_tool.properties.confidence.minimum: 'minimum' is not supported",
				prefix + ".properties.probe_no_tool.properties.confidence.maximum: 'maximum' is not supported",
				prefix + ".properties.steps.minItems: only 0 or 1 is supported",
				prefix + ".properties.steps.maxItems: 'maxItems' is not supported",
			},
		},
		{
			name:     "nested array and nullable schema",
			config:   `{"format":{"type":"json_schema","schema":{"type":"array","items":{"anyOf":[{"type":"number","minimum":0},{"type":"null"}]}}}}`,
			status:   http.StatusBadRequest,
			problems: []string{prefix + ".items.anyOf[0].minimum: 'minimum' is not supported"},
		},
		{
			name:     "referenced definition",
			config:   `{"format":{"type":"json_schema","schema":{"$ref":"#/$defs/Score","$defs":{"Score":{"allOf":[{"type":"number","maximum":1}]}}}}}`,
			status:   http.StatusBadRequest,
			problems: []string{prefix + ".$defs.Score.allOf[0].maximum: 'maximum' is not supported"},
		},
		{
			name:     "legacy definitions",
			config:   `{"format":{"type":"json_schema","schema":{"$ref":"#/definitions/Score","definitions":{"Score":{"type":"number","minimum":0}}}}}`,
			status:   http.StatusBadRequest,
			problems: []string{prefix + ".definitions.Score.minimum: 'minimum' is not supported"},
		},
		{
			name:     "missing schema",
			config:   `{"format":{"type":"json_schema"}}`,
			status:   http.StatusBadRequest,
			problems: []string{prefix + ": must be a JSON object"},
		},
		{
			name:   "malformed JSON",
			body:   `{"model":"claude-opus-5",`,
			status: http.StatusBadRequest,
		},
		{
			name:      "oversized request keeps 413",
			bodyLimit: 16,
			status:    http.StatusRequestEntityTooLarge,
		},
		{name: "no output config", status: http.StatusNoContent},
		{name: "null output config", config: `null`, status: http.StatusNoContent},
		{name: "effort only", config: `{"effort":"high"}`, status: http.StatusNoContent},
		{
			name: "supported nullable fields and constraint names as properties",
			config: `{"format":{"type":"json_schema","schema":{"type":"object","properties":{
				"minimum":{"type":"string"},"maximum":{"type":"number"},"minItems":{"type":"integer"},"maxItems":{"type":"boolean"},
				"confidence":{"anyOf":[{"type":"number"},{"type":"null"}],"description":"Between 0 and 1"},
				"steps":{"type":"array","minItems":1,"items":{"type":"string"}}
			},"required":["minimum","maximum","minItems","maxItems","confidence","steps"],"additionalProperties":false}}}`,
			status: http.StatusNoContent,
		},
		{
			name:   "constraints inside example data are not schemas",
			config: `{"format":{"type":"json_schema","schema":{"type":"object","properties":{},"additionalProperties":false,"default":{"minimum":0,"maxItems":3},"examples":[{"maximum":1,"minItems":3}]}}}`,
			status: http.StatusNoContent,
		},
		{
			name:   "non-strict tool schemas retain their constraints",
			body:   `{"model":"claude-opus-5","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"count":{"type":"integer","minimum":0}}}}]}`,
			status: http.StatusNoContent,
		},
		{
			name:   "other endpoints are unaffected",
			path:   "/v1/chat/completions",
			config: `{"format":{"type":"json_schema","schema":{"type":"number","minimum":0}}}`,
			status: http.StatusNoContent,
		},
		{
			name:   "token counting endpoint is unaffected",
			path:   "/v1/messages/count_tokens",
			config: `{"format":{"type":"json_schema","schema":{"type":"number","minimum":0}}}`,
			status: http.StatusNoContent,
		},
	}
	for _, keyword := range []string{"exclusiveMinimum", "exclusiveMaximum", "multipleOf", "minLength", "maxLength"} {
		cases = append(cases, testCase{
			name:     "unsupported " + keyword,
			config:   fmt.Sprintf(`{"format":{"type":"json_schema","schema":{"type":"object","properties":{"value":{"%s":1}},"additionalProperties":false}}}`, keyword),
			status:   http.StatusBadRequest,
			problems: []string{prefix + ".properties.value." + keyword + ": '" + keyword + "' is not supported"},
		})
	}
	for _, value := range []string{"0", "1", "1.0", "-1", "1.5", "null", `"1"`} {
		status := http.StatusBadRequest
		if value == "0" || value == "1" || value == "1.0" {
			status = http.StatusNoContent
		}
		cases = append(cases, testCase{
			name:     "minItems " + value,
			config:   fmt.Sprintf(`{"format":{"type":"json_schema","schema":{"type":"array","minItems":%s,"items":{"type":"string"}}}}`, value),
			status:   status,
			problems: []string{prefix + ".minItems: only 0 or 1 is supported"},
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.path
			if path == "" {
				path = "/v1/messages"
			}
			body := tc.body
			if body == "" {
				config := ""
				if tc.config != "" {
					config = `,"output_config":` + tc.config
				}
				body = fmt.Sprintf(`{"model":"claude-opus-5","max_tokens":8192,"stream":true,"messages":[{"role":"user","content":"hi"}]%s}`, config)
			}
			reachedHandler := false
			engine := gin.New()
			engine.Use(BodyStorageCleanup(), func(c *gin.Context) {
				if tc.bodyLimit > 0 {
					c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, tc.bodyLimit)
				}
			}, ValidateClaudeMessagesOutput())
			engine.POST(path, func(c *gin.Context) {
				reachedHandler = true
				// Validation must leave the complete original body available to
				// channel routing, passthrough forwarding and request logging.
				got, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				assert.Equal(t, body, string(got))
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, path+"?beta=true", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept-Language", "en-US")
			engine.ServeHTTP(recorder, request)

			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			assert.Equal(t, tc.status == http.StatusNoContent, reachedHandler)
			if tc.status == http.StatusNoContent {
				return
			}
			var response struct {
				Type  string `json:"type"`
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, "error", response.Type)
			assert.Equal(t, "invalid_request_error", response.Error.Type)
			assert.Contains(t, response.Error.Message, "请求格式错误，请通过 IM 软件联系管理员")
			assert.Contains(t, response.Error.Message, "Invalid request format. Please contact the administrator via IM software")
			for _, problem := range tc.problems {
				assert.Contains(t, response.Error.Message, problem)
			}
		})
	}
}
