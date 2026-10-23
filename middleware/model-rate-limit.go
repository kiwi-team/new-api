package middleware

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
)

// Both backends use a sliding window and reserve concurrent success slots.
// Model traffic limits degrade to per-instance limits during Redis outages;
// authentication limits have a separate database authority.
const modelRateLimitReserveScript = `
local now = tonumber(redis.call('TIME')[1])
local duration = tonumber(ARGV[1])
local total = tonumber(ARGV[2])
local success = tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now-duration)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now-duration)
redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', now)
if total > 0 then
  if redis.call('ZCARD', KEYS[1]) >= total then return 1 end
  redis.call('ZADD', KEYS[1], now, ARGV[4])
  redis.call('EXPIRE', KEYS[1], duration)
end
if success > 0 then
  if redis.call('ZCARD', KEYS[2]) + redis.call('ZCARD', KEYS[3]) >= success then return 2 end
  redis.call('ZADD', KEYS[3], now+60, ARGV[4])
  redis.call('EXPIRE', KEYS[3], 60)
end
return 0`

const modelRateLimitCompleteScript = `
redis.call('ZREM', KEYS[2], ARGV[1])
if ARGV[2] == '1' then
  local now = tonumber(redis.call('TIME')[1])
  redis.call('ZADD', KEYS[1], 'NX', now, ARGV[1])
  redis.call('EXPIRE', KEYS[1], ARGV[3])
end
return 1`

func redisRateLimitHandler(subject string, duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		client := common.RDB
		if client == nil {
			memoryRateLimitHandler(subject, duration, totalMaxCount, successMaxCount)(c)
			return
		}
		// A new namespace avoids interpreting old list/token-bucket keys as ZSETs.
		prefix := "rateLimit:model:v3:" + subject
		keys := []string{prefix + ":total", prefix + ":success", prefix + ":pending"}
		reservation := uuid.NewString()
		decision, err := client.Eval(c.Request.Context(), modelRateLimitReserveScript, keys, duration, totalMaxCount, successMaxCount, reservation).Int()
		if err != nil {
			memoryRateLimitHandler(subject, duration, totalMaxCount, successMaxCount)(c)
			return
		}
		if decision != 0 {
			message, maximum := i18n.MsgRateLimitTotalReached, totalMaxCount
			if decision == 2 {
				message, maximum = i18n.MsgRateLimitReached, successMaxCount
			}
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, i18n.T(c, message, map[string]any{"Minutes": setting.ModelRequestRateLimitDurationMinutes, "Max": maximum}))
			return
		}
		success := 0
		if successMaxCount > 0 {
			stop, done := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(done)
				ticker := time.NewTicker(20 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-stop:
						return
					case <-ticker.C:
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
						err := client.Eval(ctx, `if redis.call('ZSCORE', KEYS[1], ARGV[1]) then local now = tonumber(redis.call('TIME')[1]); redis.call('ZADD', KEYS[1], now+60, ARGV[1]); redis.call('EXPIRE', KEYS[1], 60) end; return 1`, []string{keys[2]}, reservation).Err()
						cancel()
						if err != nil {
							return
						}
					}
				}
			}()
			defer func() {
				close(stop)
				<-done
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := client.Eval(ctx, modelRateLimitCompleteScript, keys[1:], reservation, success, duration).Err(); err != nil {
					common.SysError(common.LogText("Failed to complete model rate limit reservation: %v", err))
				}
			}()
		}
		c.Next()
		if modelRequestSucceeded(c) {
			success = 1
		}
	}
}

// 内存限流处理器
func memoryRateLimitHandler(subject string, duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		totalKey := ModelRequestRateLimitCountMark + subject
		successKey := ModelRequestRateLimitSuccessCountMark + subject

		// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, i18n.T(c, i18n.MsgRateLimitTotalReached, map[string]any{"Minutes": setting.ModelRequestRateLimitDurationMinutes, "Max": totalMaxCount}))
			return
		}

		var reservation *common.RateLimitReservation
		if successMaxCount > 0 {
			reservation = inMemoryRateLimiter.Reserve(successKey, successMaxCount, duration)
			if reservation == nil {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, i18n.T(c, i18n.MsgRateLimitReached, map[string]any{"Minutes": setting.ModelRequestRateLimitDurationMinutes, "Max": successMaxCount}))
				return
			}
			defer reservation.Complete(false)
		}

		// 3. 处理请求
		c.Next()

		// 4. 如果请求成功，记录到实际的成功请求计数中
		reservation.Complete(modelRequestSucceeded(c))
	}
}

func modelRequestSucceeded(c *gin.Context) bool {
	status, _ := common.GetContextKeyType[*relaycommon.StreamStatus](c, constant.ContextKeyResponseStreamStatus)
	return c.Writer.Status() < 400 && !status.ResponseFailed()
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		// 计算限流参数
		duration := rateLimitDurationSeconds(setting.ModelRequestRateLimitDurationMinutes)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount

		// 获取分组
		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}

		// 获取分组的限流配置。配置了独立限流的分组按用户和分组单独计数，
		// 其余分组共用该用户的全局计数
		subject := strconv.Itoa(c.GetInt("id"))
		groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group)
		if found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
			subject += ":" + group
		}

		// 根据存储类型选择并执行限流处理器
		// 内存模式按 (uid, tokenId) 隔离，确保同用户的不同 token 之间相互不影响。
		tokenId := strconv.Itoa(c.GetInt("token_id"))
		if common.RedisEnabled {
			redisRateLimitHandler(":"+subject+":t="+tokenId, duration, totalMaxCount, successMaxCount)(c)
		} else {
			memoryRateLimitHandler(":"+subject+":t="+tokenId, duration, totalMaxCount, successMaxCount)(c)
		}
	}
}

func rateLimitDurationSeconds(durationMinutes int) int64 {
	if durationMinutes <= 0 {
		return 0
	}
	minutes := int64(durationMinutes)
	if minutes > math.MaxInt64/60 {
		return math.MaxInt64
	}
	return minutes * 60
}
