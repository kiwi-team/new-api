package gemini

import (
	"context"
	"fmt"
	"testing"
)

// TestUploadFileToGemini_Integration 是一个集成测试，它会真实地上传文件到 Gemini API。
func TestUploadFileToGemini_Integration(t *testing.T) {
	// 1. 从环境变量中获取 API 密钥
	apiKey := "AIzaSyAp4eSJmA6BSuS0SLIadwJnmBjzsabjtI0"
	baseUrl := "https://aice.seedsnote.com/google"
	//fileUri := "1.mp4"
	//fileUri := "17519768096356221.mp3"
	fileUri := "https://ark-project.tos-cn-beijing.volces.com/images/view.jpeg"
	//fileUri := "https://toiotech.s3.cn-northwest-1.amazonaws.com.cn/images/163-1.png"

	// 3. 创建一个真实的 Gemini 客户端
	ctx := context.Background()
	uploadedFile, err := UploadFileToGemini(ctx, fileUri, apiKey, baseUrl)
	if err != nil {
		t.Fatalf("UploadFileToGemini failed: %v", err)
	}
	fmt.Printf("uploadedFile: %#v\n", uploadedFile)

	// 6. 断言结果
	if uploadedFile == nil {
		t.Fatal("Expected a file from Gemini, but got nil")
	}
	if uploadedFile.Name == "" {
		t.Error("Expected uploaded file to have a name, but it was empty")
	}

	t.Logf("Successfully uploaded file to Gemini. Remote name: %s", uploadedFile.Name)
}
