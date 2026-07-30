package gemini

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 这两个用例是打真实网络的集成测试：需要一个可用的 Gemini API Key，
// 且被测 URL 必须可访问，因此默认跳过，仅在显式配置后手动运行。
//
//	GEMINI_TEST_API_KEY=xxx go test ./relay/channel/gemini/ -run Integration
const (
	geminiTestAPIKeyEnv  = "GEMINI_TEST_API_KEY"
	geminiTestBaseURLEnv = "GEMINI_TEST_BASE_URL"
	geminiTestFileURLEnv = "GEMINI_TEST_FILE_URL"
)

const defaultGeminiTestFileURL = "https://ark-project.tos-cn-beijing.volces.com/images/view.jpeg"

func TestUploadFileToGemini_Integration(t *testing.T) {
	apiKey := os.Getenv(geminiTestAPIKeyEnv)
	if apiKey == "" {
		t.Skipf("%s not set, skipping Gemini upload integration test", geminiTestAPIKeyEnv)
	}
	fileUri := os.Getenv(geminiTestFileURLEnv)
	if fileUri == "" {
		fileUri = defaultGeminiTestFileURL
	}

	uploadedFile, err := UploadFileToGemini(context.Background(), fileUri, apiKey, os.Getenv(geminiTestBaseURLEnv))
	require.NoError(t, err, "UploadFileToGemini failed")
	require.NotNil(t, uploadedFile, "expected a file from Gemini")
	assert.NotEmpty(t, uploadedFile.URI, "expected uploaded file to have a URI")
}

func TestGetFileMimeType_Integration(t *testing.T) {
	fileUri := os.Getenv(geminiTestFileURLEnv)
	if fileUri == "" {
		t.Skipf("%s not set, skipping Gemini mime type integration test", geminiTestFileURLEnv)
	}

	mimeType, err := GetFileMimeType(fileUri)
	require.NoError(t, err, "GetFileMimeType failed")
	assert.NotEmpty(t, mimeType)
}
