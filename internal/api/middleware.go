package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taskforge/internal"
	apperrors "github.com/taskforge/pkg/errors"
)

type rateLimiter struct {
	mu      sync.Mutex
	windows map[string]*slidingWindow
}

type slidingWindow struct {
	timestamps []int64
	limit      int
	window     int64
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{windows: make(map[string]*slidingWindow)}
}

func (rl *rateLimiter) allow(key string, limit int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now().UnixMilli()
	win, ok := rl.windows[key]
	if !ok {
		win = &slidingWindow{
			timestamps: make([]int64, 0, limit+1),
			limit:      limit,
			window:     1000,
		}
		rl.windows[key] = win
	}

	cutoff := now - win.window
	j := 0
	for i, ts := range win.timestamps {
		if ts > cutoff {
			win.timestamps = win.timestamps[i:]
			j = len(win.timestamps)
			break
		}
		j++
	}
	if j == len(win.timestamps) {
		win.timestamps = win.timestamps[:0]
	}

	if len(win.timestamps) >= win.limit {
		return false
	}

	win.timestamps = append(win.timestamps, now)
	return true
}

var globalRateLimiter = newRateLimiter()

func ResetRateLimiterForTest() {
	globalRateLimiter = newRateLimiter()
}

const ctxKeyTenantID = "tenant_id"
const ctxKeyTenant = "tenant"

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}
		if apiKey == "" {
			apiKey = "default"
		}
		c.Set(ctxKeyTenantID, apiKey)
		c.Set(ctxKeyTenant, &internal.Tenant{ID: apiKey, APIKey: apiKey})
		c.Next()
	}
}

func RateLimitMiddleware(maxPerSecond int) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := getTenantID(c)
		if tenantID == "" {
			c.Next()
			return
		}
		if !globalRateLimiter.allow("ratelimit:"+tenantID, maxPerSecond) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, apperrors.RateLimitError())
			return
		}
		c.Next()
	}
}

func ErrorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			appErr, ok := err.Err.(*apperrors.AppError)
			if ok {
				c.JSON(appErr.Code, gin.H{"error": appErr.Message})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			}
		}
	}
}

func getTenantID(c *gin.Context) string {
	v, _ := c.Get(ctxKeyTenantID)
	s, _ := v.(string)
	return s
}
