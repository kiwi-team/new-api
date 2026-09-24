package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const ContextKeyHappyOysterClientToken = "happyoyster_client_token"
const ContextKeyHappyOysterUpstreamToken = "happyoyster_upstream_token"

// HappyOysterClientTokenAuth resolves an upstream temporary API key to the
// new-api token that requested it before the normal TokenAuth middleware runs.
// Only a one-way hash of the temporary credential is persisted; the plaintext
// stays in the request context solely for the allowed client-side endpoints.
func HappyOysterClientTokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := c.GetHeader("Authorization")
		value, ok := strings.CutPrefix(authorization, "Bearer ")
		if !ok || !strings.HasPrefix(value, "st-") {
			c.Next()
			return
		}
		sum := sha256.Sum256([]byte(value))
		clientToken, err := model.GetHappyOysterClientToken(hex.EncodeToString(sum[:]))
		if err != nil {
			logger.LogWarn(c, "HappyOyster client token authentication failed: invalid_or_expired")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "invalid or expired HappyOyster client token", "data": nil})
			return
		}
		token, err := model.GetTokenById(clientToken.TokenId)
		if err != nil || token.UserId != clientToken.UserId {
			logger.LogWarn(c, "HappyOyster client token authentication failed: origin_token_unavailable client_token_id=%d", clientToken.Id)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "originating API token is unavailable", "data": nil})
			return
		}
		c.Set(ContextKeyHappyOysterClientToken, clientToken)
		c.Set(ContextKeyHappyOysterUpstreamToken, value)
		c.Request.Header.Set("Authorization", "Bearer sk-"+token.Key)
		c.Next()
	}
}

func HappyOysterModelAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
			value, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
			limits, valid := value.(map[string]bool)
			if !ok || !valid || !limits["happyoyster-1.0-adventure"] {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "API token is not allowed to use happyoyster-1.0-adventure", "data": nil})
				return
			}
		}
		c.Next()
	}
}
