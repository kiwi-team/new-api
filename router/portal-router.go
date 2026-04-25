package router

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func SetPortalRouter(router *gin.Engine, portalFS embed.FS, portalIndexPage []byte) {
	// Build an http.FileSystem from the embedded customer-portal/dist
	subFS, err := fs.Sub(portalFS, "customer-portal/dist")
	if err != nil {
		panic(err)
	}
	httpFS := http.FS(subFS)
	fileServer := http.StripPrefix("/portal", http.FileServer(httpFS))

	router.GET("/portal/*any", func(c *gin.Context) {
		path := c.Request.URL.Path
		filePath := strings.TrimPrefix(path, "/portal")
		if filePath == "" {
			filePath = "/"
		}

		// Try to open the file from embedded FS
		f, openErr := httpFS.Open(filePath)
		if openErr == nil {
			stat, statErr := f.Stat()
			f.Close()
			if statErr == nil && !stat.IsDir() {
				// File exists and is not a directory — serve it
				fileServer.ServeHTTP(c.Writer, c.Request)
				c.Abort()
				return
			}
		}

		// File not found — for asset paths, return 404
		if strings.HasPrefix(filePath, "/assets/") {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		// SPA fallback: serve index.html for all other routes
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", portalIndexPage)
	})
}
