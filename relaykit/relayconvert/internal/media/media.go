package media

import (
	"errors"
	"sync"

	"context"
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
