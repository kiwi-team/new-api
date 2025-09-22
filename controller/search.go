package controller

import (
	"one-api/search"

	"github.com/gin-gonic/gin"
)

func Search(c *gin.Context) {
	search.SearchHandler(c)
}
