package channel_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/gemini_realtime"
	geminitask "github.com/QuantumNous/new-api/relay/channel/task/gemini"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	vertextask "github.com/QuantumNous/new-api/relay/channel/task/vertex"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type googlePrivacyCall struct {
	Host, Path, Query, Method, Body string
	Headers                         http.Header
	Protocol                        int
}

type googlePrivacyFixture struct {
	server    *httptest.Server
	calls     chan googlePrivacyCall
	transport *http.Transport
}

// Use real verified TLS and a real HTTP receiver while preserving the official
// host in the request. All dials are mapped locally; no request can reach Google.
func newGooglePrivacyFixture(t *testing.T, http2 bool, respond func(http.ResponseWriter, *http.Request, []byte)) *googlePrivacyFixture {
	t.Helper()
	f := &googlePrivacyFixture{calls: make(chan googlePrivacyCall, 64)}
	f.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		f.calls <- googlePrivacyCall{Host: r.Host, Path: r.URL.Path, Query: r.URL.RawQuery, Method: r.Method, Body: string(body), Headers: r.Header.Clone(), Protocol: r.ProtoMajor}
		w.Header().Set("Content-Type", "application/json")
		if respond != nil {
			respond(w, r, body)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	f.server.EnableHTTP2 = http2
	f.server.StartTLS()
	t.Cleanup(f.server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(f.server.Certificate())
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, ServerName: f.server.Certificate().DNSNames[0], MinVersion: tls.VersionTLS12}
	transport.ForceAttemptHTTP2 = http2
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, f.server.Listener.Addr().String())
	}
	f.transport = transport
	previousTransport := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = previousTransport
		transport.CloseIdleConnections()
	})
	client, err := service.GetHttpClientWithProxySettings("", dto.ChannelSettings{})
	require.NoError(t, err)
	previousClient := *client
	client.Transport = transport
	client.CheckRedirect = nil
	t.Cleanup(func() { *client = previousClient })
	return f
}

func (f *googlePrivacyFixture) next(t *testing.T) googlePrivacyCall {
	t.Helper()
	select {
	case call := <-f.calls:
		return call
	case <-time.After(5 * time.Second):
		t.Fatal("missing expected local Google request")
		return googlePrivacyCall{}
	}
}

func TestGoogleRequestPrivacyWire(t *testing.T) {
	for _, http2 := range []bool{false, true} {
		t.Run(fmt.Sprintf("http2=%t", http2), func(t *testing.T) {
			fixture := newGooglePrivacyFixture(t, http2, nil)
			for _, mode := range []string{"preserve", "custom", "omit", "disabled"} {
				t.Run(mode, func(t *testing.T) {
					policy := &dto.GoogleRequestPrivacySettings{Enabled: mode != "disabled", UserAgentMode: mode, CustomUserAgent: "example-client/1.0", SDKVersionHeaders: "omit"}
					if mode == "disabled" {
						policy.UserAgentMode = "omit"
					}
					client, err := service.GooglePrivacyHTTPClient(http.DefaultClient, policy)
					require.NoError(t, err)
					body := `{"generationConfig":{"maxOutputTokens":0},"contents":[{"parts":[{"text":"test"}]}]}`
					req, err := http.NewRequest(http.MethodPost, "https://generativelanguage.googleapis.com/v1beta/models/gemini-test:generateContent?alt=sse", strings.NewReader(body))
					require.NoError(t, err)
					req.Header = http.Header{
						"User-Agent": {"original-client"},
						"Origin":     {"https://client.invalid"}, "x-forwarded-for": {"192.0.2.10"}, "X-Real-IP": {"192.0.2.11"},
						"Forwarded": {"for=192.0.2.10"}, "Referer": {"https://client.invalid/page"}, "Via": {"ingress"},
						"Traceparent": {"test-trace"}, "Tracestate": {"state"}, "Baggage": {"user=42"}, "X-Amzn-Trace-Id": {"alb-trace"}, "Sec-CH-UA": {"browser"},
						"X-Cloud-Trace-Context": {"google-trace"},
						"X-Goog-Api-Client":     {"sdk/1.0"}, "Authorization": {"Bearer test-access-token"}, "X-Goog-Api-Key": {"test-api-key"},
						"X-Goog-User-Project": {"test-project"}, "Content-Type": {"application/json"}, "Accept": {"text/event-stream"},
						"X-Goog-Upload-Command": {"upload, finalize"}, "New-Api-User": {"42"},
					}
					if mode == "custom" || mode == "omit" {
						req.Header["user-agent"] = []string{"duplicate-client"}
					}
					original := req.Header.Clone()
					resp, err := client.Do(req)
					require.NoError(t, err)
					require.NoError(t, resp.Body.Close())
					call := fixture.next(t)
					assert.Equal(t, body, call.Body)
					assert.Equal(t, "alt=sse", call.Query)
					assert.Equal(t, original, req.Header)
					assert.Equal(t, "Bearer test-access-token", call.Headers.Get("Authorization"))
					assert.Equal(t, "test-api-key", call.Headers.Get("X-Goog-Api-Key"))
					assert.Equal(t, "test-project", call.Headers.Get("X-Goog-User-Project"))
					assert.Equal(t, "upload, finalize", call.Headers.Get("X-Goog-Upload-Command"))
					assert.Equal(t, "42", call.Headers.Get("New-Api-User"))
					wantProtocol := 1
					if http2 {
						wantProtocol = 2
					}
					assert.Equal(t, wantProtocol, call.Protocol)
					if mode == "disabled" {
						assert.Equal(t, "https://client.invalid", call.Headers.Get("Origin"))
						assert.Equal(t, "sdk/1.0", call.Headers.Get("X-Goog-Api-Client"))
						return
					}
					for _, name := range []string{"Origin", "Referer", "Forwarded", "X-Forwarded-For", "X-Real-IP", "Via", "Traceparent", "Tracestate", "Baggage", "X-Amzn-Trace-Id", "X-Cloud-Trace-Context", "Sec-CH-UA", "X-Goog-Api-Client"} {
						assert.Empty(t, call.Headers.Values(name), name)
					}
					switch mode {
					case "omit":
						assert.Empty(t, call.Headers.Values("User-Agent"))
					case "custom":
						assert.Equal(t, []string{"example-client/1.0"}, call.Headers.Values("User-Agent"))
					case "preserve":
						assert.Contains(t, call.Headers.Get("User-Agent"), "original-client")
					}
				})
			}
		})
	}
}

func TestGoogleRequestPrivacyBoundaries(t *testing.T) {
	policy := &dto.GoogleRequestPrivacySettings{Enabled: true, UserAgentMode: "omit"}
	ctx := service.WithGoogleRequestPrivacy(context.Background(), policy)
	assert.Nil(t, service.GoogleRequestPrivacyFromContext(service.WithGoogleRequestPrivacy(ctx, nil)), "a retry without a policy must not inherit the previous channel's policy")
	corrupt := `{"proxy":"http://localhost:8080","google_request_privacy":{"enabled":"invalid"}}`
	corruptChannel := &model.Channel{Setting: &corrupt}
	settings := corruptChannel.GetSetting()
	assert.Equal(t, "http://localhost:8080", settings.Proxy)
	require.Error(t, settings.GoogleRequestPrivacy.Validate())
	assert.Equal(t, corrupt, *corruptChannel.Setting)
	for _, tc := range []struct {
		host     string
		filtered bool
	}{
		{"generativelanguage.googleapis.com", true}, {"aiplatform.googleapis.com", true}, {"us-central1-aiplatform.googleapis.com", true},
		{"storage.googleapis.com", true}, {"bucket.storage.googleapis.com", true},
		{"generativelanguage.googleapis.com.evil.invalid", false}, {"evil-generativelanguage.googleapis.com", false},
		{"evil.us-central1-aiplatform.googleapis.com", false}, {"oauth2.googleapis.com", false}, {"openrouter.ai", false}, {"gateway.invalid", false},
	} {
		t.Run(tc.host, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "https://"+tc.host+"/test", strings.NewReader("request-body"))
			require.NoError(t, err)
			req.Header.Set("Origin", "https://client.invalid")
			prepared, err := service.PrepareGoogleRequest(req, policy)
			require.NoError(t, err)
			if !tc.filtered {
				assert.Equal(t, req.Header, prepared.Header)
				return
			}
			assert.Empty(t, prepared.Header.Get("Origin"))
			assert.Equal(t, "https://client.invalid", req.Header.Get("Origin"))
			assert.Equal(t, req.ContentLength, prepared.ContentLength)
			replay, err := prepared.GetBody()
			require.NoError(t, err)
			data, err := io.ReadAll(replay)
			require.NoError(t, err)
			require.NoError(t, replay.Close())
			assert.Equal(t, "request-body", string(data))
		})
	}
	assert.Nil(t, service.GoogleRequestPrivacyForChannel(constant.ChannelTypeOpenRouter, dto.ChannelSettings{GoogleRequestPrivacy: policy}))
	for _, attribution := range []string{"new-api", "NewAPI", "New API", "QuantumNous"} {
		req, err := http.NewRequest(http.MethodGet, "https://generativelanguage.googleapis.com/test", nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", attribution)
		_, err = service.PrepareGoogleRequest(req, policy)
		require.ErrorContains(t, err, "protected project attribution")
		assert.Equal(t, attribution, req.UserAgent())
	}
	for _, invalid := range []*dto.GoogleRequestPrivacySettings{
		{Enabled: true, UserAgentMode: "bad"}, {SDKVersionHeaders: "bad"},
		{Enabled: true, UserAgentMode: "custom"}, {Enabled: true, UserAgentMode: "custom", CustomUserAgent: "injected\r\nheader"},
		{Enabled: true, UserAgentMode: "custom", CustomUserAgent: strings.Repeat("中", 86)},
		{Enabled: true, UserAgentMode: "custom", CustomUserAgent: "injected\x00header"},
	} {
		ch := &model.Channel{Type: constant.ChannelTypeGemini}
		ch.SetSetting(dto.ChannelSettings{GoogleRequestPrivacy: invalid})
		require.Error(t, ch.ValidateSettings())
		_, err := service.GooglePrivacyHTTPClient(http.DefaultClient, invalid)
		require.Error(t, err)
	}
	for _, enabled := range []bool{false, true} {
		policy := &dto.GoogleRequestPrivacySettings{Enabled: enabled, UserAgentMode: "custom"}
		if enabled {
			require.Error(t, policy.Validate())
		} else {
			require.NoError(t, policy.Validate(), "disabling an unfinished custom policy must remain possible")
		}
	}
}

func TestGoogleRequestPrivacyGeminiSubmissionAndBackground(t *testing.T) {
	fixture := newGooglePrivacyFixture(t, true, nil)
	policy := &dto.GoogleRequestPrivacySettings{Enabled: true, UserAgentMode: "omit"}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType: constant.ChannelTypeGemini, ChannelBaseUrl: "https://generativelanguage.googleapis.com", UpstreamModelName: "gemini-test", ApiKey: "selected-key",
		HeadersOverride: map[string]any{"*": ""}, ChannelSetting: dto.ChannelSettings{GoogleRequestPrivacy: policy},
	}}
	for _, stream := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
		c.Request.Header.Set("Origin", "https://client.invalid")
		c.Request.Header.Set("User-Agent", "caller-agent")
		c.Request.Header.Set("Content-Type", "application/json")
		info.IsStream, info.DisablePing = stream, true
		resp, err := (&gemini.Adaptor{}).DoRequest(c, info, strings.NewReader(`{"contents":[]}`))
		require.NoError(t, err)
		require.NoError(t, resp.(*http.Response).Body.Close())
		call := fixture.next(t)
		assert.Empty(t, call.Headers.Get("Origin"))
		assert.Empty(t, call.Headers.Values("User-Agent"))
		assert.Equal(t, "selected-key", call.Headers.Get("X-Goog-Api-Key"))
		assert.Empty(t, call.Headers.Get("Authorization"), "official host must use the official adaptor branch")
	}
	adaptor := &geminitask.TaskAdaptor{}
	info.IsStream = false
	info.UpstreamModelName = "veo-test"
	adaptor.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader("{}"))
	resp, err := adaptor.DoRequest(c, info, strings.NewReader(`{"instances":[]}`))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Empty(t, fixture.next(t).Headers.Values("User-Agent"))
	resp, err = adaptor.FetchTask("https://generativelanguage.googleapis.com", "original-task-key", map[string]any{"task_id": taskcommon.EncodeLocalTaskID("operations/test")}, "")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	call := fixture.next(t)
	assert.Empty(t, call.Headers.Values("User-Agent"))
	assert.Equal(t, "original-task-key", call.Headers.Get("X-Goog-Api-Key"))
}

func TestGoogleRequestPrivacyWebsocket(t *testing.T) {
	fixture := newGooglePrivacyFixture(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if assert.NoError(t, err) {
			_ = conn.Close()
		}
	})
	previous := websocket.DefaultDialer
	dialer := *previous
	dialer.Proxy = nil
	dialer.TLSClientConfig = fixture.transport.TLSClientConfig.Clone()
	dialer.NetDialContext = fixture.transport.DialContext
	websocket.DefaultDialer = &dialer
	t.Cleanup(func() { websocket.DefaultDialer = previous })
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType: constant.ChannelTypeGemini, ChannelBaseUrl: "https://generativelanguage.googleapis.com", ApiKey: "live-api-key",
		ChannelSetting:  dto.ChannelSettings{GoogleRequestPrivacy: &dto.GoogleRequestPrivacySettings{Enabled: true, UserAgentMode: "omit"}},
		HeadersOverride: map[string]any{"*": ""},
	}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/live", nil)
	c.Request.Header.Set("Origin", "https://caller.invalid")
	c.Request.Header.Set("User-Agent", "caller-agent")
	conn, err := channel.DoWssRequest(&gemini_realtime.Adaptor{}, c, info, nil)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	call := fixture.next(t)
	assert.Empty(t, call.Headers.Values("User-Agent"))
	assert.Empty(t, call.Headers.Values("Origin"))
	query, err := url.ParseQuery(call.Query)
	require.NoError(t, err)
	assert.Equal(t, "live-api-key", query.Get("key"))
}

func TestGoogleRequestPrivacyVertexAndTaskAuthentication(t *testing.T) {
	fixture := newGooglePrivacyFixture(t, true, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			_, _ = io.WriteString(w, `{"access_token":"vertex-access-token","token_type":"Bearer","expires_in":3600}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	})
	policy := &dto.GoogleRequestPrivacySettings{Enabled: true, UserAgentMode: "omit"}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType: constant.ChannelTypeVertexAi, ChannelBaseUrl: "https://aiplatform.googleapis.com", UpstreamModelName: "gemini-test", ApiKey: "vertex-api-key",
		ChannelSetting:       dto.ChannelSettings{GoogleRequestPrivacy: policy},
		ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey},
	}}
	for _, stream := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
		c.Request.Header.Set("Content-Type", "application/json")
		info.IsStream, info.DisablePing = stream, true
		adaptor := &vertex.Adaptor{}
		adaptor.Init(info)
		response, err := adaptor.DoRequest(c, info, strings.NewReader(`{"contents":[]}`))
		require.NoError(t, err)
		require.NoError(t, response.(*http.Response).Body.Close())
		call := fixture.next(t)
		assert.Empty(t, call.Headers.Values("User-Agent"))
		query, err := url.ParseQuery(call.Query)
		require.NoError(t, err)
		assert.Equal(t, "vertex-api-key", query.Get("key"))
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	credentials, err := common.Marshal(map[string]any{
		"project_id": "test-project", "client_email": "test@test-project.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})),
	})
	require.NoError(t, err)
	info.ApiKey = string(credentials)
	adaptor := &vertextask.TaskAdaptor{}
	adaptor.Init(info)
	taskID := taskcommon.EncodeLocalTaskID("projects/test-project/locations/us-central1/publishers/google/models/veo-test/operations/test")
	response, err := adaptor.FetchTask("https://aiplatform.googleapis.com", string(credentials), map[string]any{"task_id": taskID}, "")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	tokenCall := fixture.next(t)
	assert.Equal(t, "www.googleapis.com", tokenCall.Host)
	assert.NotEmpty(t, tokenCall.Headers.Get("User-Agent"))
	call := fixture.next(t)
	assert.Empty(t, call.Headers.Values("User-Agent"))
	assert.Equal(t, "Bearer vertex-access-token", call.Headers.Get("Authorization"))
	assert.Equal(t, "test-project", call.Headers.Get("X-Goog-User-Project"))
}

func TestGoogleRequestPrivacyFilesUploadAndCleanup(t *testing.T) {
	fixture := newGooglePrivacyFixture(t, true, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		switch r.URL.Path {
		case "/source":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = io.WriteString(w, "%PDF-1.7 test file")
		case "/upload/v1beta/files":
			w.Header().Set("X-Goog-Upload-URL", "https://generativelanguage.googleapis.com/resumable")
			_, _ = io.WriteString(w, `{}`)
		case "/resumable":
			w.Header().Set("X-Goog-Upload-Status", "final")
			_, _ = io.WriteString(w, `{"file":{"name":"files/uploaded","uri":"https://generativelanguage.googleapis.com/v1beta/files/uploaded","mimeType":"application/pdf","state":"ACTIVE"}}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = service.WithGoogleRequestPrivacy(ctx, &dto.GoogleRequestPrivacySettings{Enabled: true, UserAgentMode: "custom", CustomUserAgent: "example-client/1.0", SDKVersionHeaders: "omit"})
	file, err := gemini.UploadFileToGemini(ctx, fixture.server.URL+"/source", "sdk-key", "")
	require.NoError(t, err)
	assert.Equal(t, "files/uploaded", file.Name)
	assert.Equal(t, "/source", fixture.next(t).Path)
	for _, path := range []string{"/upload/v1beta/files", "/resumable"} {
		call := fixture.next(t)
		assert.Equal(t, path, call.Path)
		assert.Equal(t, "example-client/1.0", call.Headers.Get("User-Agent"))
		assert.Equal(t, "sdk-key", call.Headers.Get("X-Goog-Api-Key"))
		assert.Empty(t, call.Headers.Values("X-Goog-Api-Client"))
	}
	gemini.DeleteFileFromGemini("sdk-key", file.Name, ctx)
	call := fixture.next(t)
	assert.Equal(t, http.MethodDelete, call.Method)
	assert.Equal(t, "example-client/1.0", call.Headers.Get("User-Agent"))
	assert.Empty(t, call.Headers.Values("X-Goog-Api-Client"))
}

func TestGoogleRequestPrivacyBackgroundResolvesOriginalChannel(t *testing.T) {
	fixture := newGooglePrivacyFixture(t, true, nil)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tasks.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	previousDB, previousMemoryCache := model.DB, common.MemoryCacheEnabled
	model.DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() { model.DB, common.MemoryCacheEnabled = previousDB, previousMemoryCache })
	ch := &model.Channel{Type: constant.ChannelTypeGemini, Name: "original-channel", Key: "channel-key"}
	ch.SetSetting(dto.ChannelSettings{GoogleRequestPrivacy: &dto.GoogleRequestPrivacySettings{Enabled: true, UserAgentMode: "omit"}})
	require.NoError(t, db.Create(ch).Error)
	adaptor := relay.GetTaskAdaptor(constant.TaskPlatform(fmt.Sprint(constant.ChannelTypeGemini)))
	require.NotNil(t, adaptor)
	task := &model.Task{ChannelId: ch.Id, TaskID: taskcommon.EncodeLocalTaskID("operations/background-test")}
	resp, err := adaptor.FetchTask("https://generativelanguage.googleapis.com", "original-task-key", task, "")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	call := fixture.next(t)
	assert.Empty(t, call.Headers.Values("User-Agent"))
	assert.Equal(t, "original-task-key", call.Headers.Get("X-Goog-Api-Key"))
}

func TestGoogleRequestPrivacySDKMetadataAndStorageAuthentication(t *testing.T) {
	var refreshes atomic.Int32
	fixture := newGooglePrivacyFixture(t, true, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.URL.Path == "/token" {
			_, _ = fmt.Fprintf(w, `{"access_token":"refreshed-%d","token_type":"Bearer","expires_in":1}`, refreshes.Add(1))
			return
		}
		_, _ = io.WriteString(w, `{"name":"files/test","bucket":"test-bucket","size":"1","generation":"1","metadata":{"new_api_user_id":"42"}}`)
	})
	policy := &dto.GoogleRequestPrivacySettings{Enabled: true, UserAgentMode: "omit", SDKVersionHeaders: "omit"}
	client, err := service.GooglePrivacyHTTPClient(nil, policy)
	require.NoError(t, err)
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{APIKey: "sdk-api-key", Backend: genai.BackendGeminiAPI, HTTPClient: client})
	require.NoError(t, err)
	_, err = sdk.Files.Get(context.Background(), "files/test", nil)
	require.NoError(t, err)
	call := fixture.next(t)
	assert.Empty(t, call.Headers.Values("User-Agent"))
	assert.Empty(t, call.Headers.Values("X-Goog-Api-Client"))
	assert.Equal(t, "sdk-api-key", call.Headers.Get("X-Goog-Api-Key"))
	ctx := service.WithGoogleRequestPrivacy(context.Background(), policy)
	gemini.DeleteFileFromGemini("sdk-api-key", "files/test", ctx)
	call = fixture.next(t)
	assert.Equal(t, http.MethodDelete, call.Method)
	assert.Empty(t, call.Headers.Values("User-Agent"))
	assert.Empty(t, call.Headers.Values("X-Goog-Api-Client"))
	credentials := `{"type":"authorized_user","client_id":"test-client","client_secret":"test-secret","refresh_token":"test-refresh","token_uri":"https://oauth2.googleapis.com/token","quota_project_id":"sdk-quota-project"}`
	storageClient, err := service.NewGoogleStorageClient(ctx, credentials)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storageClient.Close()) })
	for range 2 {
		attrs, err := storageClient.Bucket("test-bucket").Object("new-api/requests/test").Attrs(ctx)
		require.NoError(t, err)
		assert.Equal(t, "42", attrs.Metadata["new_api_user_id"])
		tokenCall := fixture.next(t)
		assert.Equal(t, "/token", tokenCall.Path)
		assert.NotEmpty(t, tokenCall.Headers.Get("User-Agent"), "OAuth exchange must remain outside the privacy policy")
		call = fixture.next(t)
		assert.Equal(t, fmt.Sprintf("Bearer refreshed-%d", refreshes.Load()), call.Headers.Get("Authorization"))
		assert.Equal(t, "sdk-quota-project", call.Headers.Get("X-Goog-User-Project"))
		assert.Empty(t, call.Headers.Values("User-Agent"))
		assert.Empty(t, call.Headers.Values("X-Goog-Api-Client"))
	}
	assert.EqualValues(t, 2, refreshes.Load(), "expired SDK credentials must refresh normally")
}

func TestGoogleRequestPrivacySettingsDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "privacy.db"))
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			table := db.Table("google_privacy_test_channels").Session(&gorm.Session{})
			require.NoError(t, table.AutoMigrate(&model.Channel{}))
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable("google_privacy_test_channels")) })
			versionQuery := "select version()"
			if dialect == "sqlite" {
				versionQuery = "select sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("%s version: %s", dialect, version)
			ch := &model.Channel{Type: constant.ChannelTypeGemini, Name: "privacy-test", Key: "test-key"}
			ch.SetSetting(dto.ChannelSettings{Proxy: "http://localhost:8080", GoogleFileBucket: "files", HTTP2ConnectionShards: 3})
			require.NoError(t, table.Create(ch).Error)
			for _, enabled := range []bool{true, false} {
				var loaded model.Channel
				require.NoError(t, table.First(&loaded, ch.Id).Error)
				setting := loaded.GetSetting()
				if enabled {
					assert.Nil(t, setting.GoogleRequestPrivacy)
				}
				setting.GoogleRequestPrivacy = &dto.GoogleRequestPrivacySettings{Enabled: enabled, UserAgentMode: "custom", CustomUserAgent: "example-client/1.0", SDKVersionHeaders: "omit"}
				loaded.SetSetting(setting)
				require.NoError(t, loaded.ValidateSettings())
				require.NoError(t, table.Save(&loaded).Error)
				for range 2 {
					var reloaded model.Channel
					require.NoError(t, table.First(&reloaded, ch.Id).Error)
					assert.Equal(t, setting, reloaded.GetSetting())
				}
			}
		})
	}
}
