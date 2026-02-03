package controller

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
)

// RelayMoonshotFormulas 处理 moonshot formulas 相关的请求
// POST /v1/formulas/:vendor/:formula/fibers - 创建 fiber
// GET /v1/formulas/:vendor/:formula/tools - 获取 tools 列表
func RelayMoonshotFormulas(c *gin.Context) {
	channelType := c.GetInt("channel_type")
	if channelType != constant.ChannelTypeMoonshot {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "This endpoint only supports Moonshot channel",
				"type":    "invalid_request_error",
				"code":    "unsupported_channel",
			},
		})
		return
	}

	baseURL := common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl)
	apiKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)

	// 优先从 context 获取 vendor 和 formula（由 distributor 设置）
	vendor := c.GetString("moonshot_formula_vendor")
	formula := c.GetString("moonshot_formula_name")

	// 如果 context 中没有，则从 URL 参数获取
	if vendor == "" {
		vendor = c.Param("vendor")
	}
	if formula == "" {
		formula = c.Param("formula")
	}

	if vendor == "" || formula == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Missing vendor or formula in request path",
				"type":    "invalid_request_error",
				"code":    "missing_parameters",
			},
		})
		return
	}

	var targetURL string
	var method string

	if c.Request.Method == http.MethodPost {
		// POST /v1/formulas/:vendor/:formula/fibers
		targetURL = fmt.Sprintf("%s/v1/formulas/%s/%s/fibers", baseURL, vendor, formula)
		method = http.MethodPost
	} else if c.Request.Method == http.MethodGet {
		// GET /v1/formulas/:vendor/:formula/tools
		targetURL = fmt.Sprintf("%s/v1/formulas/%s/%s/tools", baseURL, vendor, formula)
		method = http.MethodGet
	} else {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"error": gin.H{
				"message": "Method not allowed",
				"type":    "invalid_request_error",
				"code":    "method_not_allowed",
			},
		})
		return
	}

	// 创建上游请求
	var reqBody io.Reader
	if method == http.MethodPost {
		reqBody = c.Request.Body
	}

	req, err := http.NewRequest(method, targetURL, reqBody)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("Failed to create request: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to create upstream request",
				"type":    "internal_error",
				"code":    "request_creation_failed",
			},
		})
		return
	}

	// 设置请求头
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("Failed to send request to upstream: %v", err))
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": "Failed to connect to upstream",
				"type":    "upstream_error",
				"code":    "upstream_connection_failed",
			},
		})
		return
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("Failed to read upstream response: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to read upstream response",
				"type":    "internal_error",
				"code":    "response_read_failed",
			},
		})
		return
	}

	// 复制响应头
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// 返回上游响应
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}
