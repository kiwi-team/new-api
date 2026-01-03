package search

import (
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/search/engine"
	"github.com/QuantumNous/new-api/search/engine/serper"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	searchCommon "github.com/QuantumNous/new-api/search/common"

	"github.com/gin-gonic/gin"
)

func SearchHandler(c *gin.Context) {
	searchInfo := searchCommon.GetSearchInfo(c)
	if searchInfo == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "search info is nil"})
		return
	}
	adptor, err := GetSearchAdptor(searchInfo.Engine)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	adptor.Init(searchInfo)
	resp, err := adptor.DoRequest(c, searchInfo, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	respData, err := adptor.DoResponse(c, resp, searchInfo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	serperSearchResult, ok := respData.(model.SerperSearchResult)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "respData is not *model.SerperSearchResult"})
		return
	}
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	searchInfo.UseTimeSeconds = time.Now().Unix() - startTime.Unix()
	go model.SaveAiSearchLog(&serperSearchResult, *searchInfo)
	common.ApiSuccess(c, respData)
}

func GetSearchAdptor(engine string) (engine.SearchAdptor, error) {
	switch engine {
	case "serper":
		return &serper.SerperAdaptor{}, nil
	default:
		return nil, errors.New("search engine not implemented")
	}
}
