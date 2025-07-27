package controller

import (
	"net/http"
	"one-api/common"
	"one-api/dto"
	"one-api/model"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetAllErrorLogs(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if p < 1 {
		p = 1
	}
	if pageSize < 0 {
		pageSize = common.ItemsPerPage
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")
	requestId := c.Query("request_id")
	channel, _ := strconv.Atoi(c.Query("channel"))
	logs, total, err := model.GetAllErrorLog(&dto.ErrorLogsRequest{
		RequestId: requestId,
		ChannelId: channel,
		ModelName: modelName,
		StartTime: startTimestamp,
		EndTime:   endTimestamp,
		Page:      p,
		PageSize:  pageSize,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": map[string]any{
			"items":     logs,
			"total":     total,
			"page":      p,
			"page_size": pageSize,
		},
	})
}
