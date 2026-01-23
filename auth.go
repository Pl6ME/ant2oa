package main

import (
	"os"
	"sync"
	"time"

	"github.com/goccy/go-json"
)

type APIKeyConfig struct {
	RateLimit int    `json:"rate_limit"` // RPM
	Role      string `json:"role"`       // "user", "admin"
	Active    bool   `json:"active"`
}

var (
	apiKeys      = make(map[string]*APIKeyConfig)
	apiKeysMutex sync.RWMutex
	keyLimiters  = make(map[string]*RateLimiter)
	limitersMu   sync.Mutex
)

// RateLimiter struct for per-key limiting
type RateLimiter struct {
	tokens  int
	max     int
	lastRef time.Time
	mu      sync.Mutex
	rate    time.Duration // interval per token
}

func newRateLimiter(rpm int) *RateLimiter {
	if rpm <= 0 {
		return nil
	}
	return &RateLimiter{
		tokens:  rpm,
		max:     rpm,
		lastRef: time.Now(),
		rate:    time.Minute / time.Duration(rpm),
	}
}

func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRef)
	tokensToAdd := int(elapsed / rl.rate)

	if tokensToAdd > 0 {
		rl.tokens += tokensToAdd
		if rl.tokens > rl.max {
			rl.tokens = rl.max
		}
		rl.lastRef = now
	}

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

// loadAPIKeys loads keys from keys.json
func loadAPIKeys() error {
	data, err := os.ReadFile("keys.json")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var keys map[string]*APIKeyConfig
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}

	apiKeysMutex.Lock()
	apiKeys = keys
	// Reset limiters if needed? for now just keep simple
	apiKeysMutex.Unlock()
	return nil
}

// validateAPIKey checks key validity and rate limit
func validateAPIKey(key string) (bool, *APIKeyConfig) {
	apiKeysMutex.RLock()
	hasKeys := len(apiKeys) > 0
	config, exists := apiKeys[key]
	apiKeysMutex.RUnlock()

	// Legacy mode: if no keys configured, allow all
	if !hasKeys {
		return true, nil
	}

	if !exists {
		return false, nil
	}

	if !config.Active {
		return false, config
	}

	// Check rate limit
	if config.RateLimit > 0 {
		limitersMu.Lock()
		limiter, exists := keyLimiters[key]
		if !exists {
			limiter = newRateLimiter(config.RateLimit)
			keyLimiters[key] = limiter
		}
		limitersMu.Unlock()

		if limiter != nil && !limiter.Allow() {
			return false, config // Exists but limited
		}
	}

	return true, config
}
