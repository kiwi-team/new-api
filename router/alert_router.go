package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func registerAlertRoutes(api *gin.RouterGroup) {
	alerts := api.Group("/alerts", middleware.UserAuth(), middleware.DisableCache())
	alerts.Use(alertRequestGuards()...)
	alerts.GET("/catalog", controller.AlertCatalog)
	alerts.GET("/migrations", controller.ListAlertMigrations)
	alerts.POST("/migrations", controller.ImportAlertMigration)
	alerts.POST("/migrations/batch", controller.ImportAlertMigrationsBatch)
	alerts.POST("/migrations/:id/:operation", controller.SwitchAlertMigration)
	alerts.GET("/rules", controller.ListAlertRules)
	alerts.POST("/rules", controller.SaveAlertRule)
	alerts.POST("/rules/preview", controller.PreviewAlertRule)
	alerts.POST("/rules/backtest", controller.PreviewAlertRule)
	alerts.POST("/rules/apply-template", controller.SaveAlertRule)
	alerts.GET("/rules/:id", controller.GetAlertRule)
	alerts.GET("/rules/:id/impact", controller.AlertActionImpact)
	alerts.GET("/rules/:id/report", controller.QueryAlertReport)
	alerts.PATCH("/rules/:id", controller.SaveAlertRule)
	alerts.DELETE("/rules/:id", controller.DeleteAlertRule)
	alerts.POST("/rules/:id/pause", controller.SetAlertRulePaused)
	alerts.POST("/rules/:id/resume", controller.SetAlertRulePaused)
	alerts.GET("/targets", controller.ListAlertTargets)
	alerts.POST("/targets/import-notification", controller.ImportAlertNotification)
	alerts.POST("/targets", controller.SaveAlertTarget)
	alerts.GET("/targets/:id", controller.GetAlertTarget)
	alerts.POST("/targets/:id/test", controller.TestAlertTarget)
	alerts.PATCH("/targets/:id", controller.SaveAlertTarget)
	alerts.DELETE("/targets/:id", controller.DeleteAlertTarget)
	alerts.GET("/events", controller.ListAlertEvents)
	alerts.GET("/events/:id", controller.GetAlertEvent)
	alerts.GET("/events/:id/jobs", controller.GetAlertEvent)
	alerts.GET("/incidents", controller.ListAlertIncidents)
	alerts.GET("/incidents/:id", controller.GetAlertIncident)
	alerts.POST("/jobs/:id/retry", controller.RetryAlertJob)
	alerts.GET("/holds", controller.ListAlertHolds)
	alerts.POST("/holds/:id/release", controller.ReleaseAlertHold)
	alerts.POST("/channels/:id/protection", controller.SetAlertChannelPolicy)
	alerts.POST("/incidents/:id/:operation", controller.HandleAlertIncident)
	alerts.GET("/engine-health", controller.GetAlertEngineHealth)
	alerts.GET("/channel-models", controller.AlertChannelModels)
	alerts.GET("/channels/:id/models", controller.GetAlertChannelModelControls)
	alerts.POST("/channels/:id/models/control", controller.SetAlertChannelModelControl)
}

// Keep body/origin validation before rate middleware: Gin middleware may call
// Next internally, so checking Origin after invoking a limiter is too late.
func alertRequestGuards() []gin.HandlerFunc {
	writeLimit := middleware.UserCriticalRateLimit("alert-management")
	return []gin.HandlerFunc{
		func(c *gin.Context) {
			if c.Request.Body != nil {
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024)
			}
			if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && !c.GetBool("use_access_token") {
				middleware.SessionCookieOriginGuard()(c)
				return
			}
			c.Next()
		},
		func(c *gin.Context) {
			if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
				writeLimit(c)
				return
			}
			c.Next()
		},
	}
}
