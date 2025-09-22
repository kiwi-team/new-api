package serper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/model"
	searchcommon "one-api/search/common"
	"one-api/service"

	"github.com/gin-gonic/gin"
)

// SerperAdaptor implements the SearchAdptor interface for the Serper search engine.
type SerperAdaptor struct {
}

// Init initializes the adaptor.
func (s *SerperAdaptor) Init(info *searchcommon.SearchInfo) {
}

// GetRequestURL returns the request URL for the Serper API.
func (s *SerperAdaptor) GetRequestURL(info *searchcommon.SearchInfo) (string, error) {
	//https://google.serper.dev/search
	return "https://google.serper.dev/search", nil
}

// SetupRequestHeader sets up the request header for the Serper API.
func (s *SerperAdaptor) SetupRequestHeader(c *gin.Context, req *http.Request, info *searchcommon.SearchInfo) error {
	req.Header.Set("X-API-KEY", info.ChannelKey)
	req.Header.Set("Content-Type", "application/json")
	return nil
}

// DoRequest prepares the request body for the Serper API.
func (s *SerperAdaptor) DoRequest(c *gin.Context, info *searchcommon.SearchInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := s.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	requestBodyJSON, err := json.Marshal(info.SearchParameters)
	if err != nil {
		return nil, fmt.Errorf("marshal request body failed: %w", err)
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, bytes.NewBuffer(requestBodyJSON))
	s.SetupRequestHeader(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	client := service.GetHttpClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// DoResponse handles the response from the Serper API.
func (s *SerperAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *searchcommon.SearchInfo) (any, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}
	var responseData model.SerperSearchResult
	err := json.NewDecoder(resp.Body).Decode(&responseData)
	if err != nil {
		return nil, err
	}
	return responseData, nil
}
