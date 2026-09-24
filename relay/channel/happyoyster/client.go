package happyoyster

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

const APIPath = "/api/v2/apps/happyoyster-1.0-adventure/openapi/v1"

func Do(ctx *http.Request, baseURL, key, proxy string, settings dto.ChannelSettings, method, path string, query url.Values, body []byte) (int, []byte, error) {
	return do(ctx, baseURL, key, proxy, settings, method, APIPath+path, query, body)
}

func GenerateTemporaryAPIKey(ctx *http.Request, baseURL, key, proxy string, settings dto.ChannelSettings, expireInSeconds int) (int, []byte, error) {
	query := url.Values{"expire_in_seconds": []string{fmt.Sprintf("%d", expireInSeconds)}}
	return do(ctx, baseURL, key, proxy, settings, http.MethodPost, "/api/v1/tokens", query, nil)
}

func do(ctx *http.Request, baseURL, key, proxy string, settings dto.ChannelSettings, method, path string, query url.Values, body []byte) (int, []byte, error) {
	endpoint := strings.TrimRight(baseURL, "/") + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx.Context(), method, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxySettings(proxy, settings)
	if err != nil {
		return 0, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	const maxResponse = 16 << 20
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if len(payload) > maxResponse {
		return resp.StatusCode, nil, fmt.Errorf("HappyOyster response exceeds %d bytes", maxResponse)
	}
	return resp.StatusCode, payload, nil
}
