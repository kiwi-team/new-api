package volcengine

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func handleSeedream55ImageResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	var body map[string]any
	var result dto.SimpleResponse
	if err = common.Unmarshal(data, &body); err == nil {
		err = common.Unmarshal(data, &result)
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if body["error"] != nil {
		return nil, types.NewOpenAIError(fmt.Errorf("image generation failed: %v", body["error"]), types.ErrorCodeBadResponseBody, http.StatusBadRequest)
	}
	entries, _ := body["data"].([]any)
	if entry, ok := body["data"].(map[string]any); ok {
		entries = []any{entry}
	}
	images := []map[string]any{}
	for _, value := range entries {
		entry, _ := value.(map[string]any)
		image := map[string]any{}
		for _, key := range []string{"url", "b64_json", "revised_prompt"} {
			if value, ok := entry[key].(string); ok && strings.TrimSpace(value) != "" {
				image[key] = value
			}
		}
		if image["url"] != nil || image["b64_json"] != nil {
			images = append(images, image)
		}
	}
	if len(images) == 0 {
		return nil, types.NewOpenAIError(fmt.Errorf("image generation returned no images"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	facts, count := relaycommon.Seedream55ResponseUsage(body)
	info.UpdateImageCount(int64(count))
	if info.BillingRequestInput != nil && info.TieredBillingSnapshot != nil && info.TieredBillingSnapshot.TaskUsageBilling {
		measured := maps.Clone(info.BillingRequestInput.Usage)
		maps.Copy(measured, facts)
		info.BillingRequestInput.Usage = measured
	}
	output, err := common.Marshal(map[string]any{"created": body["created"], "data": images, "metadata": body})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	service.IOCopyBytesGracefully(c, resp, output)
	usage := result.Usage
	usage.PromptTokens = usage.InputTokens
	usage.CompletionTokens = usage.OutputTokens
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return &usage, nil
}
