package router

import (
	"one-api/controller"
	"one-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetSearchRouter(router *gin.Engine) {
	searchV1Router := router.Group("/v1")
	searchV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		searchV1Router.POST("/:engine/:action", controller.Search)
	}
}
