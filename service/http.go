package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
)

const maxUpstreamRequestIdLength = 128

// CaptureUpstreamRequestId records the first recognized upstream request ID
// using a stable, channel-aware priority. The logs column is varchar(128), so
// oversized values are ignored instead of making the whole log insert fail.
func CaptureUpstreamRequestId(c *gin.Context, header http.Header, channelType int) string {
	headerNames := []string{common.RequestIdKey}
	switch channelType {
	case constant.ChannelTypeAzure:
		headerNames = append(headerNames, "Apim-Request-Id", "X-Ms-Request-Id")
	case constant.ChannelTypeAnthropic:
		headerNames = append(headerNames, "Request-Id")
	case constant.ChannelTypeAws:
		headerNames = append(headerNames, "X-Amzn-RequestId", "X-Amz-Request-Id", "Request-Id")
	case constant.ChannelTypeVolcEngine:
		headerNames = append(headerNames, "X-Tt-Logid")
	}
	headerNames = append(headerNames,
		"X-Shellapi-Request-Id",
		"X-Request-Id",
		"Request-Id",
		"Apim-Request-Id",
		"X-Ms-Request-Id",
		"X-Amzn-RequestId",
		"X-Amz-Request-Id",
		"X-Tt-Logid",
	)

	upstreamRequestId := ""
	for _, headerName := range headerNames {
		value := strings.TrimSpace(header.Get(headerName))
		if value == "" {
			continue
		}
		if len(value) > maxUpstreamRequestIdLength {
			logger.LogWarn(c, "ignored oversized upstream request id from header %s: length=%d", headerName, len(value))
			continue
		}
		upstreamRequestId = value
		break
	}
	if c != nil {
		c.Set(common.UpstreamRequestIdKey, upstreamRequestId)
	}
	return upstreamRequestId
}

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

// ShouldCopyUpstreamHeader checks whether a given upstream response header
// should be copied to the client response. It returns false for Content-Length
// (managed separately) and X-Oneapi-Request-Id (to preserve the local instance
// ID). Request ID capture is handled once per complete response header by
// CaptureUpstreamRequestId so candidate priority stays deterministic.
func ShouldCopyUpstreamHeader(_ *gin.Context, k string, _ []string) bool {
	if strings.EqualFold(k, "Content-Length") {
		return false
	}
	if strings.EqualFold(k, common.RequestIdKey) {
		return false
	}
	return true
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	body := io.NopCloser(bytes.NewBuffer(data))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		CaptureUpstreamRequestId(c, src.Header, common.GetContextKeyInt(c, constant.ContextKeyChannelType))
		for k, v := range src.Header {
			if !ShouldCopyUpstreamHeader(c, k, v) {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// 上游数据已就绪、即将开始向客户端发送（写出 body 之前），单位：秒
	c.Writer.Header().Set("X-Finished-At", fmt.Sprintf("%d", common.GetTimestamp()))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	c.Writer.Flush()
}
