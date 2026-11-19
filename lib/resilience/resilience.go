package resilience

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

// CircuitState indicates the current circuit breaker status.
type CircuitState string

const (
	StateClosed   CircuitState = "closed"
	StateOpen     CircuitState = "open"
	StateHalfOpen CircuitState = "half_open"
)

var (
	ErrCircuitOpen  = errors.New("resilience: circuit breaker is open")
	ErrRateLimited  = errors.New("resilience: rate limit exceeded")
)

// CircuitBreaker tracks consecutive failures and sheds load when tripped.
type CircuitBreaker struct {
	mu               sync.Mutex
	Name             string
	FailureThreshold int
	RecoveryTimeout  time.Duration
	state            CircuitState
	failures         int
	successes        int
	lastFailure      time.Time
}

// NewCircuitBreaker constructs a CircuitBreaker with sensible defaults.
func NewCircuitBreaker(name string, failureThreshold int, recoveryTimeout time.Duration) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 5
	}
	if recoveryTimeout <= 0 {
		recoveryTimeout = 30 * time.Second
	}
	return &CircuitBreaker{
		Name:             name,
		FailureThreshold: failureThreshold,
		RecoveryTimeout:  recoveryTimeout,
		state:            StateClosed,
	}
}

// State returns the current breaker state (auto-transitioning open -> half_open after RecoveryTimeout).
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == StateOpen && time.Since(cb.lastFailure) >= cb.RecoveryTimeout {
		cb.state = StateHalfOpen
	}
	return cb.state
}

// Allow checks whether a call is permitted right now.
func (cb *CircuitBreaker) Allow() bool {
	return cb.State() != StateOpen
}

// RecordSuccess resets failure count and closes the breaker.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.successes++
	cb.state = StateClosed
}

// RecordFailure increments failure count and opens the breaker upon reaching FailureThreshold.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= cb.FailureThreshold {
		cb.state = StateOpen
	}
}

// Reset restores the breaker to closed with zero failures.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = StateClosed
}

// Execute runs fn guarded by the circuit breaker.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.Allow() {
		return ErrCircuitOpen
	}
	err := fn()
	if err != nil {
		cb.RecordFailure()
		return err
	}
	cb.RecordSuccess()
	return nil
}

// Status returns a diagnostic map for operational APIs.
func (cb *CircuitBreaker) Status() map[string]any {
	st := cb.State()
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return map[string]any{
		"name":              cb.Name,
		"state":             string(st),
		"failures":          cb.failures,
		"successes":         cb.successes,
		"failure_threshold": cb.FailureThreshold,
		"recovery_sec":      cb.RecoveryTimeout.Seconds(),
	}
}

// RateLimiter implements a thread-safe token bucket limiter.
type RateLimiter struct {
	mu         sync.Mutex
	Name       string
	Rate       float64 // tokens per second
	Burst      float64 // max bucket capacity
	tokens     float64
	lastRefill time.Time
}

// NewRateLimiter constructs a token bucket limiter.
func NewRateLimiter(name string, ratePerSec float64, burst int) *RateLimiter {
	if ratePerSec <= 0 {
		ratePerSec = 10
	}
	if burst <= 0 {
		burst = int(math.Ceil(ratePerSec))
	}
	return &RateLimiter{
		Name:       name,
		Rate:       ratePerSec,
		Burst:      float64(burst),
		tokens:     float64(burst),
		lastRefill: time.Now(),
	}
}

// Allow consumes one token if available, returning false when rate limited.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens = math.Min(rl.Burst, rl.tokens+elapsed*rl.Rate)
	rl.lastRefill = now
	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}
	return false
}

// Status returns token bucket telemetry.
func (rl *RateLimiter) Status() map[string]any {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return map[string]any{
		"name":   rl.Name,
		"rate":   rl.Rate,
		"burst":  rl.Burst,
		"tokens": rl.tokens,
	}
}

// RetryConfig configures exponential backoff retries.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// Retry executes fn up to cfg.MaxAttempts times with exponential backoff, honoring ctx cancellation.
func Retry(ctx context.Context, cfg RetryConfig, fn func() error) error {
	attempts := cfg.MaxAttempts
	if attempts <= 0 {
		attempts = 3
	}
	base := cfg.BaseDelay
	if base <= 0 {
		base = 50 * time.Millisecond
	}
	maxD := cfg.MaxDelay
	if maxD <= 0 {
		maxD = 5 * time.Second
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i == attempts-1 {
			break
		}
		delay := time.Duration(float64(base) * math.Pow(2, float64(i)))
		if delay > maxD {
			delay = maxD
		}
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		} else {
			time.Sleep(delay)
		}
	}
	return lastErr
}

// Do combines optional RateLimiter, CircuitBreaker, and Retry around fn.
func Do(ctx context.Context, rl *RateLimiter, cb *CircuitBreaker, retry *RetryConfig, fn func() error) error {
	runOnce := func() error {
		if rl != nil && !rl.Allow() {
			return ErrRateLimited
		}
		if cb != nil {
			return cb.Execute(fn)
		}
		return fn()
	}
	if retry != nil {
		return Retry(ctx, *retry, runOnce)
	}
	return runOnce()
}
