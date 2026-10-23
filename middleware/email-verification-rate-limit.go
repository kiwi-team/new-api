package middleware

import (
	"github.com/gin-gonic/gin"
)

const (
	EmailVerificationRateLimitMark = "EV"
	EmailVerificationMaxRequests   = 2  // 30秒内最多2次
	EmailVerificationDuration      = 30 // 30秒时间窗口
)

func EmailVerificationRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		securityRateLimiter(c, EmailVerificationMaxRequests, EmailVerificationDuration, redisIPRateLimitKey(EmailVerificationRateLimitMark, c.ClientIP()))
		if !c.IsAborted() {
			c.Next()
		}
	}
}
