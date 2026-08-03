package router

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelStatusRoutesUseOperatePermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodPost, "/:id/status", authz.ChannelOperate, controller.UpdateChannelStatus)
	assertChannelRoutePermission(t, http.MethodPost, "/status/batch", authz.ChannelOperate, controller.BatchUpdateChannelStatus)
	assertChannelRoutePermission(t, http.MethodPut, "/", authz.ChannelWrite, controller.UpdateChannel)
}

func TestChannelDeleteRoutesUseSensitiveWritePermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodDelete, "/:id", authz.ChannelSensitiveWrite, controller.DeleteChannel)
	assertChannelRoutePermission(t, http.MethodPost, "/batch", authz.ChannelSensitiveWrite, controller.DeleteChannelBatch)
	assertChannelRoutePermission(t, http.MethodDelete, "/disabled", authz.ChannelSensitiveWrite, controller.DeleteDisabledChannel)
	assertChannelRoutePermission(t, http.MethodPut, "/", authz.ChannelWrite, controller.UpdateChannel)
	assertChannelRoutePermission(t, http.MethodPut, "/tag", authz.ChannelWrite, controller.EditTagChannels)
	assertChannelRoutePermission(t, http.MethodPost, "/batch/tag", authz.ChannelWrite, controller.BatchSetChannelTag)
}

func TestChannelStatusRoutesRegisterWithoutConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerChannelRoutes(api)
	})
}

// TestChannelNameListRouteResolvesStatically guards the id/name picker feed
// used by the route-config, token and log-filter screens. Without its own
// route, "/api/channel/channel-name-list" falls through to "/:id" and the
// handler fails with a strconv parse error instead of returning the list.
func TestChannelNameListRouteResolvesStatically(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/channel-name-list", authz.ChannelRead, controller.GetNameIdList)
	assertChannelRouteRegistered(t, http.MethodGet, "/api/channel/channel-name-list")
}

// TestChannelRoutesLostInUpstreamMergeStayRegistered pins the endpoints that
// were dropped when the upstream refactor replaced this fork's inline route
// block. Each one has a live handler and a caller in the console, so a
// missing route surfaces as a confusing runtime error rather than a 404.
func TestChannelRoutesLostInUpstreamMergeStayRegistered(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/channel-list-by-model", authz.ChannelRead, controller.GetChannelsByModelName)
	assertChannelRoutePermission(t, http.MethodGet, "/channel-list-by-model-newapi", authz.ChannelRead, controller.GetChannelsByModelNameNewAPI)
	assertChannelRoutePermission(t, http.MethodPost, "/codex/oauth/start", authz.ChannelSensitiveWrite, controller.StartCodexOAuth)
	assertChannelRoutePermission(t, http.MethodPost, "/codex/oauth/complete", authz.ChannelSensitiveWrite, controller.CompleteCodexOAuth)
	assertChannelRoutePermission(t, http.MethodPost, "/:id/codex/oauth/start", authz.ChannelSensitiveWrite, controller.StartCodexOAuthForChannel)
	assertChannelRoutePermission(t, http.MethodPost, "/:id/codex/oauth/complete", authz.ChannelSensitiveWrite, controller.CompleteCodexOAuthForChannel)

	assertChannelRouteRegistered(t, http.MethodGet, "/api/channel/channel-list-by-model")
	assertChannelRouteRegistered(t, http.MethodPost, "/api/channel/codex/oauth/start")
	assertChannelRouteRegistered(t, http.MethodPost, "/api/channel/:id/codex/oauth/start")
}

func assertChannelRouteRegistered(t *testing.T, method string, path string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerChannelRoutes(engine.Group("/api"))

	for _, route := range engine.Routes() {
		if route.Method == method && route.Path == path {
			return
		}
	}
	t.Fatalf("route %s %s is not registered", method, path)
}

func assertChannelRoutePermission(t *testing.T, method string, path string, permission authz.Permission, handler any) {
	t.Helper()
	for _, route := range channelPermissionRoutes {
		if route.method == method && route.path == path {
			assert.Equal(t, permission, route.permission)
			assert.Equal(t, reflect.ValueOf(handler).Pointer(), reflect.ValueOf(route.handler).Pointer())
			return
		}
	}
	t.Fatalf("route %s %s not found", method, path)
}
