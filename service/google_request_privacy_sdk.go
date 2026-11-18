package service

import (
	"context"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	googlehttp "google.golang.org/api/transport/http"
)

// NewGoogleStorageClient retains the Storage SDK's credential implementation.
// The policy sits below Google's auth/telemetry transport so refreshed tokens
// and SDK-generated headers are processed at the final sending boundary.
func NewGoogleStorageClient(ctx context.Context, credentials string) (*storage.Client, error) {
	options := []option.ClientOption{option.WithCredentialsJSON([]byte(credentials))}
	policy := GoogleRequestPrivacyFromContext(ctx)
	if policy == nil {
		return storage.NewClient(ctx, options...)
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if !policy.Enabled {
		return storage.NewClient(ctx, options...)
	}
	// These are the same scopes and endpoint defaults used by storage.NewClient.
	// Keep the SDK's TLS configuration and base pool rather than replacing it
	// with a relay or unauthenticated application client.
	base, _, err := googlehttp.NewClient(ctx,
		option.WithoutAuthentication(), option.WithTelemetryDisabled(),
		internaloption.WithDefaultEndpointTemplate("https://storage.UNIVERSE_DOMAIN/storage/v1/"),
		internaloption.WithDefaultMTLSEndpoint("https://storage.mtls.googleapis.com/storage/v1/"),
		internaloption.WithDefaultUniverseDomain("googleapis.com"),
	)
	if err != nil {
		return nil, err
	}
	// Test binaries may omit module build metadata; the fallback matches the
	// pinned SDK. Production builds resolve the dependency's actual version.
	userAgent := "gcloud-golang-storage/1.43.0"
	if build, ok := debug.ReadBuildInfo(); ok {
		for _, dependency := range build.Deps {
			if dependency.Path == "cloud.google.com/go/storage" {
				userAgent = "gcloud-golang-storage/" + strings.TrimPrefix(dependency.Version, "v")
				break
			}
		}
	}
	authOptions := []option.ClientOption{
		option.WithCredentialsJSON([]byte(credentials)),
		option.WithScopes(storage.ScopeFullControl, "https://www.googleapis.com/auth/cloud-platform"),
		option.WithUserAgent(userAgent),
	}
	if os.Getenv("STORAGE_EMULATOR_HOST") != "" {
		authOptions = []option.ClientOption{option.WithoutAuthentication()}
	}
	transport, err := googlehttp.NewTransport(ctx,
		&googlePrivacyTransport{base: base.Transport, policy: *policy}, authOptions...)
	if err != nil {
		return nil, err
	}
	options = append(options, option.WithHTTPClient(&http.Client{Transport: transport}))
	return storage.NewClient(ctx, options...)
}
