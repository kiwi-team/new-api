package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	taskgemini "github.com/QuantumNous/new-api/relay/channel/task/gemini"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func VideoProxy(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "task_id is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	task, exists, err := model.GetByOnlyTaskId(taskID)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to query task %s: %s", taskID, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to query task",
				"type":    "server_error",
			},
		})
		return
	}
	if !exists || task == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to get task %s: %v", taskID, err))
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Task not found",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if task.Status != model.TaskStatusSuccess {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Task is not completed yet, current status: %s", task.Status),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	channel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to get task %s: not found", taskID))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to retrieve channel information",
				"type":    "server_error",
			},
		})
		return
	}
	if channel.Type == constant.ChannelTypeVertexAi && strings.HasPrefix(task.FailReason, "https://") {
		model.TaskUpdateVideoUrl(task.ID, task.FailReason)
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"url": task.FailReason,
			},
		})
		return
	}
	if strings.Contains(channel.GetBaseURL(), "yunwu") {
		model.TaskUpdateVideoUrl(task.ID, task.FailReason)
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"url": task.FailReason,
			},
		})
		return
	}
	if strings.Contains(channel.GetBaseURL(), "ppinfra") {
		model.TaskUpdateVideoUrl(task.ID, task.FailReason)
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"url": task.FailReason,
			},
		})
		return
	}
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	var videoURL string
	proxy := channel.GetSetting().Proxy
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create proxy client for task %s: %s", taskID, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to create proxy client",
				"type":    "server_error",
			},
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "", nil)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create request: %s", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to create proxy request",
				"type":    "server_error",
			},
		})
		return
	}

	switch channel.Type {
	case constant.ChannelTypeGemini:
		apiKey := task.PrivateData.Key
		if apiKey == "" {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Missing stored API key for Gemini task %s", taskID))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"message": "API key not stored for task",
					"type":    "server_error",
				},
			})
			return
		}

		// Omni interactions tasks: the video is returned inline as base64 or as a file uri.
		if taskgemini.IsOmniTaskID(task.TaskID) {
			omniURI, omniB64, _, oerr := getGeminiOmniVideo(channel, task, apiKey)
			if oerr != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to resolve Gemini Omni video for task %s: %s", taskID, oerr.Error()))
				c.JSON(http.StatusBadGateway, gin.H{
					"error": gin.H{
						"message": "Failed to resolve Gemini video",
						"type":    "server_error",
					},
				})
				return
			}
			if omniB64 != "" {
				// Inline base64 -> upload to S3 and return the S3 url.
				s3url, upErr := service.SimpleUploadToS3(c.Request.Context(), omniB64)
				if upErr != nil {
					logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to upload Omni video to S3 for task %s: %s", taskID, upErr.Error()))
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": gin.H{
							"message": "Failed to upload video to S3",
							"type":    "server_error",
						},
					})
					return
				}
				if err := model.TaskUpdateVideoUrl(task.ID, s3url); err != nil {
					logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to update task %s video url: %s", taskID, err.Error()))
				}
				c.JSON(http.StatusOK, gin.H{
					"data": gin.H{
						"url": s3url,
					},
				})
				return
			}
			// File uri delivery -> stream through the existing upload path below.
			videoURL = omniURI
			req.Header.Set("x-goog-api-key", apiKey)
			break
		}

		videoURL, err = getGeminiVideoURL(channel, task, apiKey)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to resolve Gemini video URL for task %s: %s", taskID, err.Error()))
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"message": "Failed to resolve Gemini video URL",
					"type":    "server_error",
				},
			})
			return
		}
		req.Header.Set("x-goog-api-key", apiKey)
	case constant.ChannelTypeOpenAI, constant.ChannelTypeSora:
		videoURL = fmt.Sprintf("%s/v1/videos/%s/content", baseURL, task.TaskID)
		req.Header.Set("Authorization", "Bearer "+channel.Key)
	default:
		// Video URL is directly in task.FailReason
		videoURL = task.FailReason
	}

	req.URL, err = url.Parse(videoURL)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to parse URL %s: %s", videoURL, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to create proxy request",
				"type":    "server_error",
			},
		})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to fetch video from %s: %s", videoURL, err.Error()))
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": "Failed to fetch video content",
				"type":    "server_error",
			},
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Upstream returned status %d for %s", resp.StatusCode, videoURL))
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Upstream service returned status %d", resp.StatusCode),
				"type":    "server_error",
			},
		})
		return
	}

	url, err := service.UploadIOReaderToS3(c.Request.Context(), resp)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to upload video to S3: %s", err.Error()))
	} else {
		err = model.TaskUpdateVideoUrl(task.ID, url)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to update task %s video url: %s", taskID, err.Error()))
		}

		// todo 更新任务里的视频url
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"url": url,
			},
		})
		return
	}

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}

	c.Writer.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 24 hours
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to stream video content: %s", err.Error()))
	}
}
