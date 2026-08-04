package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// media_ref.go 提供跨渠道复用的素材（图片/视频/音频）引用解析工具。
//
// 统一 /v1/videos 的 references 字段后，各 task adaptor 需要把同一个素材引用
// （公网 URL / data URI / 裸 base64 / gs:// ）翻译成各自上游要求的形态：
//   - Gemini / Veo 需要 inline base64 + mimeType
//   - 阿里 wan2.7 的参考视频只接受公网 URL 或 oss://，不接受 base64
//   - 可灵 / 豆包 图片两种都可以
//
// 这里集中处理，替代此前散落在 gemini/omni.go、vertex/adaptor.go、reve/adaptor.go
// 中的三份近似拷贝。注意不要基于 service.GetImageFromUrl 构建：它会硬拒绝
// 非 image/* 的 Content-Type，无法用于视频与音频。

// MediaRefKind 描述一个素材引用的形态。
type MediaRefKind int

const (
	MediaRefUnknown   MediaRefKind = iota
	MediaRefHTTPURL                // http(s):// 公网地址
	MediaRefGSURL                  // gs:// (Google Cloud Storage)
	MediaRefOSSURL                 // oss:// (阿里云 OSS 临时地址)
	MediaRefDataURI                // data:<mime>;base64,<data>
	MediaRefRawBase64              // 无前缀的裸 base64
)

// ClassifyMediaRef 判断素材引用的形态。
func ClassifyMediaRef(ref string) MediaRefKind {
	ref = strings.TrimSpace(ref)
	switch {
	case ref == "":
		return MediaRefUnknown
	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return MediaRefHTTPURL
	case strings.HasPrefix(ref, "gs://"):
		return MediaRefGSURL
	case strings.HasPrefix(ref, "oss://"):
		return MediaRefOSSURL
	case strings.HasPrefix(ref, "data:"):
		return MediaRefDataURI
	default:
		return MediaRefRawBase64
	}
}

// IsRemoteMediaRef 判断素材是否已经是上游可直接拉取的远程地址。
func IsRemoteMediaRef(ref string) bool {
	switch ClassifyMediaRef(ref) {
	case MediaRefHTTPURL, MediaRefGSURL, MediaRefOSSURL:
		return true
	default:
		return false
	}
}

// ResolveMediaRef 把任意形态的素材引用归一化为 (base64 数据, mime 类型)。
//
// 支持 data URI、http(s) URL 与裸 base64；gs:// / oss:// 无法在本地取得内容，
// 会返回错误，调用方应改用 ResolveRemoteMediaRef 直接透传。
//
// defaultMime 在无法确定 mime 时兜底（例如裸 base64 且内容嗅探失败）。
// 底层走 types.FileSource + GetBase64Data，天然带 SSRF 校验、请求级缓存与大文件落盘。
func ResolveMediaRef(c *gin.Context, ref string, defaultMime string) (data string, mimeType string, err error) {
	ref = strings.TrimSpace(ref)
	kind := ClassifyMediaRef(ref)
	switch kind {
	case MediaRefUnknown:
		return "", "", fmt.Errorf("empty media reference")
	case MediaRefGSURL, MediaRefOSSURL:
		return "", "", fmt.Errorf("media reference %q cannot be inlined; pass it through as a remote url", ref)
	case MediaRefHTTPURL:
		data, mimeType, err = GetBase64Data(c, types.NewURLFileSource(ref), "resolve_media_ref")
	case MediaRefDataURI, MediaRefRawBase64:
		data, mimeType, err = GetBase64Data(c, types.NewBase64FileSource(ref, ""), "resolve_media_ref")
	}
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(mimeType) == "" || mimeType == "application/octet-stream" {
		if strings.TrimSpace(defaultMime) != "" {
			mimeType = defaultMime
		}
	}
	return data, mimeType, nil
}

// ResolveRemoteMediaRef 返回一个上游可直接拉取的远程地址。
//
// 已经是 http(s):// / gs:// / oss:// 的原样返回；data URI 与裸 base64 会先上传到 S3
// 再返回 S3 地址。用于只接受远程地址的上游（如阿里 wan2.7 的参考视频）。
// 未配置 S3 时上传会失败，此时返回错误，由调用方转成 400 提示用户改传 URL。
func ResolveRemoteMediaRef(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	switch ClassifyMediaRef(ref) {
	case MediaRefUnknown:
		return "", fmt.Errorf("empty media reference")
	case MediaRefHTTPURL, MediaRefGSURL, MediaRefOSSURL:
		return ref, nil
	}
	url, err := UploadOnceToS3(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("upload inline media to storage failed (pass a public url instead): %w", err)
	}
	return url, nil
}
