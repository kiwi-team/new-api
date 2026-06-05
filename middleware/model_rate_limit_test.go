package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
)

// TestModelRequestRateLimit_PerUserTokenIsolation pins the invariant that the
// in-memory model-request rate limiter buckets are keyed by (user, token):
//
//	a) Two requests for the SAME (user, token) share a counter and the
//	   successMaxCount-th + 1 request is blocked with 429.
//	b) Two requests for the SAME user, SAME group, but DIFFERENT tokens do NOT
//	   share a counter — each token can independently reach its own
//	   successMaxCount. This is the user-stated requirement: different keys of
//	   the same user must have independent rate limits even when their groups
//	   carry the same rule.
//	c) Two requests for the SAME user, DIFFERENT groups (different rules), also
//	   stay isolated by token.
//
// The test drives the public ModelRequestRateLimit() middleware so the real
// wiring (group lookup -> memoryRateLimitHandler) is exercised end-to-end.
func TestModelRequestRateLimit_PerUserTokenIsolation(t *testing.T) {
	if string(constant.ContextKeyTokenGroup) == "" {
		t.Skip("constant.ContextKeyTokenGroup is empty; skipping isolation test")
	}

	gin.SetMode(gin.TestMode)

	prevEnabled := setting.ModelRequestRateLimitEnabled
	prevDuration := setting.ModelRequestRateLimitDurationMinutes
	prevTotal := setting.ModelRequestRateLimitCount
	prevSuccess := setting.ModelRequestRateLimitSuccessCount
	prevRedis := common.RedisEnabled

	setting.ModelRequestRateLimitMutex.Lock()
	prevGroup := setting.ModelRequestRateLimitGroup
	setting.ModelRequestRateLimitMutex.Unlock()

	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled = prevEnabled
		setting.ModelRequestRateLimitDurationMinutes = prevDuration
		setting.ModelRequestRateLimitCount = prevTotal
		setting.ModelRequestRateLimitSuccessCount = prevSuccess
		common.RedisEnabled = prevRedis

		setting.ModelRequestRateLimitMutex.Lock()
		setting.ModelRequestRateLimitGroup = prevGroup
		setting.ModelRequestRateLimitMutex.Unlock()
	})

	common.RedisEnabled = false
	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 1000

	const successMaxCount = 3
	setting.ModelRequestRateLimitMutex.Lock()
	setting.ModelRequestRateLimitGroup = map[string][2]int{
		"cc":      {0, successMaxCount},
		"default": {0, successMaxCount},
	}
	setting.ModelRequestRateLimitMutex.Unlock()

	if _, _, ok := setting.GetGroupRateLimit("cc"); !ok {
		t.Skip("setting.GetGroupRateLimit could not find group 'cc'; precondition failed")
	}
	if _, _, ok := setting.GetGroupRateLimit("default"); !ok {
		t.Skip("setting.GetGroupRateLimit could not find group 'default'; precondition failed")
	}

	handler := ModelRequestRateLimit()

	fire := func(userID, tokenID int, group string) int {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		c.Set("id", userID)
		c.Set("token_id", tokenID)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, group)
		common.SetContextKey(c, constant.ContextKeyUserGroup, group)

		handler(c)
		// gin's c.Status() only writes to the recorder's Code on body flush;
		// for an aborted-no-body response the recorder stays at 200 even when
		// the middleware logically returned 429. Read gin's internal status.
		return c.Writer.Status()
	}

	t.Run("same_user_and_token_shares_counter", func(t *testing.T) {
		const userID = 5
		const tokenID = 101
		const group = "cc"

		for i := 1; i <= successMaxCount; i++ {
			code := fire(userID, tokenID, group)
			if code != http.StatusOK {
				t.Fatalf("request %d for user=%d token=%d group=%q: got status %d, want %d",
					i, userID, tokenID, group, code, http.StatusOK)
			}
		}
		code := fire(userID, tokenID, group)
		if code != http.StatusTooManyRequests {
			t.Fatalf("request %d for user=%d token=%d group=%q: got status %d, want %d (rate limit should fire)",
				successMaxCount+1, userID, tokenID, group, code, http.StatusTooManyRequests)
		}
	})

	// This is the regression test for the user's actual requirement: two
	// tokens of the same user, both assigned to the same group ("cc"), must
	// each get their own bucket. The pre-fix bug was that they shared
	// rateLimit:MRRLS:<uid>, so saturating token A would 429 token B.
	t.Run("same_user_same_group_different_tokens_are_isolated", func(t *testing.T) {
		const userID = 6
		const tokenA = 201
		const tokenB = 202
		const group = "cc"

		// Saturate tokenA.
		for i := 1; i <= successMaxCount; i++ {
			if code := fire(userID, tokenA, group); code != http.StatusOK {
				t.Fatalf("warm-up %d for user=%d token=%d: got %d, want 200",
					i, userID, tokenA, code)
			}
		}
		if code := fire(userID, tokenA, group); code != http.StatusTooManyRequests {
			t.Fatalf("user=%d token=%d expected 429 after %d successes, got %d",
				userID, tokenA, successMaxCount, code)
		}

		// tokenB (same user, same group) must still have a fresh allowance.
		for i := 1; i <= successMaxCount; i++ {
			if code := fire(userID, tokenB, group); code != http.StatusOK {
				t.Fatalf("isolation violated: user=%d token=%d request %d returned %d, want 200 "+
					"(token %d being saturated must not affect token %d)",
					userID, tokenB, i, code, tokenA, tokenB)
			}
		}
		if code := fire(userID, tokenB, group); code != http.StatusTooManyRequests {
			t.Fatalf("user=%d token=%d expected 429 after %d independent successes, got %d",
				userID, tokenB, successMaxCount, code)
		}
	})

	t.Run("same_user_different_groups_different_tokens_are_isolated", func(t *testing.T) {
		const userID = 7
		const tokenA = 301
		const tokenB = 302

		for i := 1; i <= successMaxCount; i++ {
			if code := fire(userID, tokenA, "cc"); code != http.StatusOK {
				t.Fatalf("warm-up %d for user=%d token=%d group=cc: got %d, want 200",
					i, userID, tokenA, code)
			}
		}
		if code := fire(userID, tokenA, "cc"); code != http.StatusTooManyRequests {
			t.Fatalf("user=%d token=%d group=cc expected 429, got %d", userID, tokenA, code)
		}

		for i := 1; i <= successMaxCount; i++ {
			if code := fire(userID, tokenB, "default"); code != http.StatusOK {
				t.Fatalf("user=%d token=%d group=default request %d returned %d, want 200",
					userID, tokenB, i, code)
			}
		}
		if code := fire(userID, tokenB, "default"); code != http.StatusTooManyRequests {
			t.Fatalf("user=%d token=%d group=default expected 429, got %d", userID, tokenB, code)
		}
	})
}

// TestMemoryRateLimitKeys_NoCollision pins the key-format contract: the three
// kinds of buckets (total / success / check) cannot collide with each other
// regardless of (userId, tokenId) input, and different (uid, tokenId) tuples
// produce structurally distinct keys.
func TestMemoryRateLimitKeys_NoCollision(t *testing.T) {
	cases := []struct{ userId, tokenId string }{
		{"5", "101"},
		{"5", "102"},
		{"5", "0"},
		{"6", "101"},
		{"42", "999"},
		// Pathological/adversarial inputs — token id should always be a plain
		// integer in practice, but the helper must be robust.
		{"5", "check"},
		{"5", ":check"},
		{"5", "t=cc"},
	}

	keys := make(map[string]string)
	for _, c := range cases {
		total, success, check := memoryRateLimitKeys(c.userId, c.tokenId)

		if total == success || total == check || success == check {
			t.Fatalf("key kinds collided for (uid=%q, tokenId=%q): total=%q success=%q check=%q",
				c.userId, c.tokenId, total, success, check)
		}

		for _, k := range []string{total, success, check} {
			if prev, ok := keys[k]; ok {
				t.Fatalf("cross-case collision: key %q already produced by %q, now produced again by (uid=%q, tokenId=%q)",
					k, prev, c.userId, c.tokenId)
			}
			keys[k] = c.userId + "/" + c.tokenId
		}
	}
}
