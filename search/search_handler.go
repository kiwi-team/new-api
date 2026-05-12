package search

import (
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/search/engine"
	"github.com/QuantumNous/new-api/search/engine/jina"
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
	adptor, err := GetSearchAdptor(searchInfo.Engine, searchInfo.SearchType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		// jina_reader 在上游非 2xx 时会自行透传响应，handler 不再覆盖。
		if errors.Is(err, jina.ErrUpstreamForwarded) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	searchInfo.UseTimeSeconds = time.Now().Unix() - startTime.Unix()

	// 通过 SearchResultLogger 接口统一落库，新增引擎只需让自己的 result 类型
	// 实现该接口即可，handler 不再做类型断言。
	if logger, ok := respData.(model.SearchResultLogger); ok {
		infoCopy := *searchInfo
		go func() {
			_ = model.LOG_DB.Create(logger.ToSearchLog(infoCopy)).Error
		}()
	}

	// TODO(billing): search 类请求当前未接入扣费，仅落日志。
	//   serper / jina_reader 均待补，参考 relay 链路里 PostConsumeQuota 的实现。

	common.ApiSuccess(c, respData)
}

func GetSearchAdptor(engineName string, searchType string) (engine.SearchAdptor, error) {
	switch engineName {
	case "serper":
		return &serper.SerperAdaptor{}, nil
	case "jina":
		switch searchType {
		case "reader":
			return jina.JinaReaderAdaptor{}, nil
		// 后续 case "search": s.jina.ai 时在此扩展
		}
	}
	return nil, errors.New("search engine not implemented")
}
