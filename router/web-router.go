package router

import (
	"embed"
	"net/http"
	"one-api/common"
	"one-api/controller"
	"one-api/middleware"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

func SetWebRouter(router *gin.Engine, buildFS embed.FS, indexPage []byte) {
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())
	// CSV文件下载中间件
	router.Use(func(c *gin.Context) {
		if strings.HasSuffix(c.Request.URL.Path, ".csv") {
			c.Header("Content-Type", "text/csv")
			c.Header("Content-Disposition", "attachment; filename=\""+strings.TrimPrefix(c.Request.URL.Path, "/")+"\"")
		}
		c.Next()
	})
	// 优先服务运行时创建的静态文件
	router.Use(static.Serve("/", static.LocalFile("./web/dist", false)))
	// 回退到嵌入的静态文件
	router.Use(static.Serve("/", common.EmbedFolder(buildFS, "web/dist")))
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	})
}
