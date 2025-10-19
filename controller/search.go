package controller

import (
	"github.com/QuantumNous/new-api/search"

	"github.com/gin-gonic/gin"
)

func Search(c *gin.Context) {
	search.SearchHandler(c)
}
