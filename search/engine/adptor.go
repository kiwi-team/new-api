package engine

import (
	"io"
	"net/http"

	searchcommon "github.com/QuantumNous/new-api/search/common"

	"github.com/gin-gonic/gin"
)

type SearchAdptor interface {
	Init(info *searchcommon.SearchInfo)
	GetRequestURL(info *searchcommon.SearchInfo) (string, error)
	SetupRequestHeader(c *gin.Context, req *http.Request, info *searchcommon.SearchInfo) error
	DoRequest(c *gin.Context, info *searchcommon.SearchInfo, requestBody io.Reader) (*http.Response, error)
	DoResponse(c *gin.Context, resp *http.Response, info *searchcommon.SearchInfo) (any, error)
}
