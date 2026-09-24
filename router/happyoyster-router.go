package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

const happyOysterAPIPath = "/api/v2/apps/happyoyster-1.0-adventure/openapi/v1"

func SetHappyOysterRouter(router *gin.Engine) {
	group := router.Group(happyOysterAPIPath)
	group.Use(
		middleware.RouteTag("relay"),
		middleware.SystemPerformanceCheck(),
		middleware.HappyOysterClientTokenAuth(),
		middleware.TokenAuth(),
		middleware.HappyOysterModelAuth(),
		func(c *gin.Context) {
			c.Set("resolved_task_model", "happyoyster-1.0-adventure")
			c.Next()
		},
		middleware.ModelRequestRateLimit(),
	)
	{
		group.POST("/worlds", middleware.Distribute(), controller.HappyOysterCreateWorld)
		group.GET("/worlds/build-status", controller.HappyOysterWorldOperation)
		group.POST("/worlds/get-travel-credential", controller.HappyOysterWorldOperation)
		group.GET("/worlds/detail", controller.HappyOysterWorldOperation)
		group.GET("/worlds", controller.HappyOysterListWorlds)
		group.POST("/worlds/delete", controller.HappyOysterWorldOperation)
		group.POST("/travels/enter-travel", controller.HappyOysterEnterTravel)
		group.GET("/travels/status", controller.HappyOysterTravelOperation)
		group.POST("/travels/end", controller.HappyOysterTravelOperation)
		group.GET("/travels", controller.HappyOysterListTravels)
		group.GET("/travels/artifacts", controller.HappyOysterTravelOperation)
	}

	token := router.Group("/api/v1")
	token.Use(
		middleware.RouteTag("relay"),
		middleware.SystemPerformanceCheck(),
		middleware.TokenAuth(),
		middleware.HappyOysterModelAuth(),
		func(c *gin.Context) {
			c.Set("resolved_task_model", "happyoyster-1.0-adventure")
			c.Next()
		},
		middleware.ModelRequestRateLimit(),
	)
	token.POST("/tokens", middleware.Distribute(), controller.HappyOysterIssueTemporaryAPIKey)
}
