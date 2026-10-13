package router

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

const happyOysterAPIPath = "/api/v2/apps/happyoyster-1.0-adventure/openapi/v1"

func SetHappyOysterRouter(router *gin.Engine) {
	for _, modelName := range []string{constant.HappyOysterAdventure, constant.HappyOysterDirecting} {
		group := router.Group(constant.HappyOysterAPIPath(modelName))
		group.Use(
			middleware.RouteTag("relay"),
			middleware.SystemPerformanceCheck(),
			func(c *gin.Context) { c.Set("resolved_task_model", modelName); c.Next() },
			middleware.HappyOysterClientTokenAuth(),
			middleware.TokenAuth(),
			middleware.HappyOysterModelAuth(),
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
			if modelName == constant.HappyOysterDirecting {
				for _, operation := range []string{"rewind", "pause", "resume", "instruct", "update-script"} {
					group.POST("/travels/"+operation, controller.HappyOysterTravelOperation)
				}
			}
		}
	}

	token := router.Group("/api/v1")
	token.Use(
		middleware.RouteTag("relay"),
		middleware.SystemPerformanceCheck(),
		middleware.TokenAuth(),
		func(c *gin.Context) {
			name := c.DefaultQuery("model", constant.HappyOysterAdventure)
			if name != constant.HappyOysterAdventure && name != constant.HappyOysterDirecting {
				c.AbortWithStatusJSON(400, gin.H{"code": 400000, "message": "unsupported HappyOyster model", "data": nil})
				return
			}
			c.Set("resolved_task_model", name)
			c.Next()
		},
		middleware.HappyOysterModelAuth(),
		middleware.ModelRequestRateLimit(),
	)
	token.POST("/tokens", middleware.Distribute(), controller.HappyOysterIssueTemporaryAPIKey)
}
