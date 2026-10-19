package media

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/relaykit/types"
)

type MediaResolver struct {
	ResolveGeminiMedia   func(c context.Context, source types.FileSource, messageIndex, partIndex int) (*GeminiMedia, error)
	GetBase64Data        func(c context.Context, source types.FileSource, reason ...string) (string, string, error)
	DecodeBase64FileData func(base64String string) (string, string, error)
}

// GeminiMedia preserves file references without forcing the host to materialize base64.
type GeminiMedia struct {
	MIMEType string
	Base64   string
	URI      string
}

func ResolveGeminiMedia(c context.Context, source types.FileSource, messageIndex, partIndex int) (*GeminiMedia, error) {
	mediaResolverMu.RLock()
	resolver := mediaResolver.ResolveGeminiMedia
	mediaResolverMu.RUnlock()
	if resolver != nil {
		result, err := resolver(c, source, messageIndex, partIndex)
		if err == nil && result != nil && (result.MIMEType == "" || (result.Base64 == "") == (result.URI == "")) {
			return nil, errors.New("invalid Gemini media result")
		}
		if err != nil || result != nil {
			return result, err
		}
	}
	data, mime, err := ResolveBase64Data(c, source, "formatting media for Gemini")
	if err != nil {
		return nil, err
	}
	return &GeminiMedia{MIMEType: mime, Base64: data}, nil
}

var (
	mediaResolverMu sync.RWMutex
	mediaResolver   MediaResolver
)

func SetMediaResolver(resolver MediaResolver) {
	mediaResolverMu.Lock()
	defer mediaResolverMu.Unlock()

	mediaResolver = resolver
}

func ResolveBase64Data(c context.Context, source types.FileSource, reason ...string) (string, string, error) {
	mediaResolverMu.RLock()
	resolver := mediaResolver.GetBase64Data
	mediaResolverMu.RUnlock()
	if resolver == nil {
		return "", "", errors.New("relayconvert media resolver is not configured")
	}
	return resolver(c, source, reason...)
}

func DecodeBase64FileData(base64String string) (string, string, error) {
	mediaResolverMu.RLock()
	resolver := mediaResolver.DecodeBase64FileData
	mediaResolverMu.RUnlock()
	if resolver == nil {
		return "", "", errors.New("relayconvert media resolver is not configured")
	}
	return resolver(base64String)
}

// DefaultResponsesImageDetail is used when the source protocol carries no detail
// hint. Sending the default explicitly also supports strict Responses validators
// that reject input_image parts without detail.
const DefaultResponsesImageDetail = "auto"

// ResponsesImageDetail preserves supported image detail levels and defaults
// missing or unrecognized hints to "auto".
func ResponsesImageDetail(detail string) string {
	switch normalized := strings.ToLower(strings.TrimSpace(detail)); normalized {
	case "low", "high", "auto", "original":
		return normalized
	default:
		return DefaultResponsesImageDetail
	}
}

// IsTextMimeType reports whether a MIME type is plain text that a target can
// carry as text: text/* and application/json, ignoring parameters.
func IsTextMimeType(mimeType string) bool {
	mediaType, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(mimeType)), ";")
	mediaType = strings.TrimSpace(mediaType)
	return strings.HasPrefix(mediaType, "text/") || mediaType == "application/json"
}

// DecodeText decodes base64 file data as text. It reports false when the data
// is not valid base64 or the bytes are not UTF-8.
func DecodeText(base64Data string) (string, bool) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Data))
	if err != nil || !utf8.Valid(decoded) {
		return "", false
	}
	return string(decoded), true
}
