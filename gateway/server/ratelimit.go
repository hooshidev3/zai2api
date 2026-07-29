// Package server — Per-model rate limiting with token bucket (RPM/TPM).
package server

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimit holds per-model rate limit configuration.
type RateLimit struct {
	Model      string `json:"model"`
	MaxRPM     int    `json:"max_rpm"`
	MaxTPM     int    `json:"max_tpm"`
	MaxContext int    `json:"max_context"`
}

// tokenWindow tracks requests and tokens within a 1-minute sliding window.
type tokenWindow struct {
	mu          sync.Mutex
	reqTimes    []time.Time
	tokenCount  int64
	windowStart time.Time
}

// RateLimiter enforces per-model RPM/TPM limits.
type RateLimiter struct {
	mu      sync.RWMutex
	limits  map[string]*RateLimit
	windows map[string]*tokenWindow
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limits:  make(map[string]*RateLimit),
		windows: make(map[string]*tokenWindow),
	}
}

// SetLimit sets or updates a model's rate limit.
func (rl *RateLimiter) SetLimit(limit RateLimit) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limits[limit.Model] = &limit
}

// RemoveLimit removes a model's rate limit.
func (rl *RateLimiter) RemoveLimit(model string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.limits, model)
	delete(rl.windows, model)
}

// Allow checks if a request for the given model is allowed.
// Returns true if allowed, false if rate limit exceeded.
func (rl *RateLimiter) Allow(model string, tokens int) bool {
	rl.mu.RLock()
	limit, ok := rl.limits[model]
	rl.mu.RUnlock()

	if !ok {
		return true // No limit configured
	}

	rl.mu.Lock()
	w, ok := rl.windows[model]
	if !ok {
		w = &tokenWindow{windowStart: time.Now()}
		rl.windows[model] = w
	}
	rl.mu.Unlock()

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()

	// Reset window every minute
	if now.Sub(w.windowStart) > time.Minute {
		w.reqTimes = nil
		w.tokenCount = 0
		w.windowStart = now
	}

	// Check RPM
	if limit.MaxRPM > 0 && len(w.reqTimes) >= limit.MaxRPM {
		return false
	}

	// Check TPM
	if limit.MaxTPM > 0 && w.tokenCount+int64(tokens) > int64(limit.MaxTPM) {
		return false
	}

	w.reqTimes = append(w.reqTimes, now)
	w.tokenCount += int64(tokens)
	return true
}

// Middleware returns a gin middleware that enforces rate limits.
// Must be placed AFTER the dispatcher sets c.Set("model", ...).
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		model := c.GetString("model")
		if model == "" {
			c.Next()
			return
		}

		// Resolve alias to actual model for rate limiting
		if s, ok := c.Get("server"); ok {
			if srv, ok := s.(*Server); ok {
				model = srv.resolveAlias(model)
			}
		}

		if !rl.Allow(model, 0) {
			c.AbortWithStatusJSON(429, gin.H{
				"error": gin.H{
					"type":    "rate_limit_exceeded",
					"message": fmt.Sprintf("Rate limit for model %s exceeded", model),
				},
			})
			return
		}
		c.Next()
	}
}

// LoadLimitsFromDB loads rate limits from the database at startup.
func (rl *RateLimiter) LoadLimitsFromDB(db *sql.DB) {
	if db == nil {
		return
	}
	rows, err := db.Query("SELECT model, max_rpm, max_tpm, max_context FROM model_rate_limits")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var l RateLimit
		rows.Scan(&l.Model, &l.MaxRPM, &l.MaxTPM, &l.MaxContext)
		rl.SetLimit(l)
	}
}
