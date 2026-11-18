package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

type googleRequestPrivacyKey struct{}

func GoogleRequestPrivacyForChannel(channelType int, settings dto.ChannelSettings) *dto.GoogleRequestPrivacySettings {
	switch channelType {
	case constant.ChannelTypeGemini, constant.ChannelTypeVertexAi:
		return settings.GoogleRequestPrivacy
	case constant.ChannelTypeTaskPlugin:
		if settings.TaskPluginKey == "google" || settings.TaskPluginKey == "vertex-ai" {
			return settings.GoogleRequestPrivacy
		}
		return nil
	default:
		return nil
	}
}

// WithGoogleRequestPrivacy captures an immutable policy for SDK operations and
// cleanup without carrying the caller's request headers or credentials.
func WithGoogleRequestPrivacy(ctx context.Context, policy *dto.GoogleRequestPrivacySettings) context.Context {
	if policy == nil {
		if ctx.Value(googleRequestPrivacyKey{}) != nil {
			return context.WithValue(ctx, googleRequestPrivacyKey{}, (*dto.GoogleRequestPrivacySettings)(nil))
		}
		return ctx
	}
	return context.WithValue(ctx, googleRequestPrivacyKey{}, *policy)
}

func GoogleRequestPrivacyFromContext(ctx context.Context) *dto.GoogleRequestPrivacySettings {
	if policy, ok := ctx.Value(googleRequestPrivacyKey{}).(dto.GoogleRequestPrivacySettings); ok {
		return &policy
	}
	return nil
}

// PrepareGoogleRequest runs after adaptor/SDK header construction. Host matching
// deliberately excludes custom gateways, OAuth endpoints and lookalike domains.
func PrepareGoogleRequest(req *http.Request, policy *dto.GoogleRequestPrivacySettings) (*http.Request, error) {
	if policy == nil {
		return req, nil
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if !policy.Enabled || req == nil || req.URL == nil {
		return req, nil
	}
	host := strings.ToLower(req.URL.Hostname())
	isGoogle := host == "generativelanguage.googleapis.com" || host == "aiplatform.googleapis.com" ||
		host == "storage.googleapis.com" || host == "storage.mtls.googleapis.com"
	if region, ok := strings.CutSuffix(host, "-aiplatform.googleapis.com"); ok {
		isGoogle = region != "" && !strings.Contains(region, ".")
	}
	if bucket, ok := strings.CutSuffix(host, ".storage.googleapis.com"); ok {
		isGoogle = bucket != ""
	}
	if !isGoogle {
		return req, nil
	}
	for name, values := range req.Header {
		lower := strings.ToLower(name)
		willChange := googlePrivateHeader(lower) || lower == "user-agent" && (policy.UserAgentMode == "custom" || policy.UserAgentMode == "omit") ||
			lower == "x-goog-api-client" && policy.SDKVersionHeaders == "omit"
		if !willChange {
			continue
		}
		for _, value := range values {
			identity := strings.ToLower(value)
			if strings.Contains(identity, "new-api") || strings.Contains(identity, "newapi") || strings.Contains(identity, "new api") || strings.Contains(identity, "quantumnous") {
				return nil, errors.New("Google request privacy policy conflicts with protected project attribution")
			}
		}
	}
	out := req.Clone(req.Context())
	if out.Header == nil {
		out.Header = make(http.Header)
	}
	for name := range out.Header {
		lower := strings.ToLower(name)
		if googlePrivateHeader(lower) || lower == "user-agent" && (policy.UserAgentMode == "custom" || policy.UserAgentMode == "omit") ||
			lower == "x-goog-api-client" && policy.SDKVersionHeaders == "omit" {
			delete(out.Header, name)
		}
	}
	switch policy.UserAgentMode {
	case "omit":
		// Presence with an empty value suppresses net/http's default UA.
		out.Header["User-Agent"] = []string{""}
	case "custom":
		out.Header.Set("User-Agent", policy.CustomUserAgent)
	}
	return out, nil
}

func googlePrivateHeader(name string) bool {
	switch name {
	case "forwarded", "x-real-ip", "via", "origin", "referer", "x-amzn-trace-id", "x-cloud-trace-context", "traceparent", "tracestate", "baggage":
		return true
	}
	return strings.HasPrefix(name, "x-forwarded-") || strings.HasPrefix(name, "sec-ch-ua")
}

type googlePrivacyTransport struct {
	base   http.RoundTripper
	policy dto.GoogleRequestPrivacySettings
}

func (t *googlePrivacyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	prepared, err := PrepareGoogleRequest(req, &t.policy)
	if err != nil {
		return nil, err
	}
	return t.base.RoundTrip(prepared)
}

// GooglePrivacyHTTPClient copies a client while retaining its transport,
// authentication, proxy, redirect, timeout and connection-pool configuration.
func GooglePrivacyHTTPClient(client *http.Client, policy *dto.GoogleRequestPrivacySettings) (*http.Client, error) {
	if policy == nil {
		return client, nil
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if !policy.Enabled {
		return client, nil
	}
	if client == nil {
		// Match the Gen AI SDK's zero-value client rather than inheriting a
		// process-wide DefaultClient cookie jar, timeout or redirect policy.
		client = &http.Client{}
	}
	copy := *client
	base := copy.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = &googlePrivacyTransport{base: base, policy: *policy}
	return &copy, nil
}
