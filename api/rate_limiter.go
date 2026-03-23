package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// loginAttempt tracks failed login attempts per IP
type loginAttempt struct {
	count    int
	lastTry  time.Time
	blockedUntil time.Time
}

// LoginRateLimiter protects login endpoint from brute-force attacks
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt

	maxAttempts  int           // max failures before blocking
	blockTime    time.Duration // how long to block after max failures
	windowTime   time.Duration // time window for counting failures
}

// NewLoginRateLimiter creates a rate limiter for login attempts
func NewLoginRateLimiter() *LoginRateLimiter {
	rl := &LoginRateLimiter{
		attempts:    make(map[string]*loginAttempt),
		maxAttempts: 5,
		blockTime:   5 * time.Minute,
		windowTime:  15 * time.Minute,
	}

	// Clean up stale entries every 10 minutes
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.cleanup()
		}
	}()

	return rl
}

// Check returns true if the IP is allowed to attempt login
func (rl *LoginRateLimiter) Check(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	a, exists := rl.attempts[ip]
	if !exists {
		return true
	}

	// Check if still blocked
	if !a.blockedUntil.IsZero() && time.Now().Before(a.blockedUntil) {
		return false
	}

	// Check if window expired — reset
	if time.Since(a.lastTry) > rl.windowTime {
		delete(rl.attempts, ip)
		return true
	}

	return a.count < rl.maxAttempts
}

// RecordFailure records a failed login attempt
func (rl *LoginRateLimiter) RecordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	a, exists := rl.attempts[ip]
	if !exists {
		a = &loginAttempt{}
		rl.attempts[ip] = a
	}

	// Reset if window expired
	if time.Since(a.lastTry) > rl.windowTime {
		a.count = 0
		a.blockedUntil = time.Time{}
	}

	a.count++
	a.lastTry = time.Now()

	if a.count >= rl.maxAttempts {
		a.blockedUntil = time.Now().Add(rl.blockTime)
	}
}

// RecordSuccess clears failed attempts for an IP after successful login
func (rl *LoginRateLimiter) RecordSuccess(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

// cleanup removes stale entries
func (rl *LoginRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, a := range rl.attempts {
		// Remove if both window and block have expired
		if now.Sub(a.lastTry) > rl.windowTime && (a.blockedUntil.IsZero() || now.After(a.blockedUntil)) {
			delete(rl.attempts, ip)
		}
	}
}

// LoginRateLimitMiddleware creates a gin middleware for login rate limiting
func LoginRateLimitMiddleware(rl *LoginRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !rl.Check(ip) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many login attempts. Please try again later.",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
