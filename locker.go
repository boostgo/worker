package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/boostgo/storage/redis"
)

// Locker implements distributed locking using Redis
type Locker struct {
	client        redis.Client
	lockKey       string
	lockValue     string
	lockTTL       time.Duration
	renewInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewLocker creates a new Redis-based distributed locker
func NewLocker(client redis.Client, workerName string, lockTTL time.Duration) *Locker {
	lockValue := generateLockValue()

	return &Locker{
		client:        client,
		lockKey:       fmt.Sprintf("worker:lock:%s", workerName),
		lockValue:     lockValue,
		lockTTL:       lockTTL,
		renewInterval: lockTTL / 3, // Renew at 1/3 of TTL
	}
}

// generateLockValue creates a unique identifier for this lock instance
func generateLockValue() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// TryLock attempts to acquire the distributed lock
func (l *Locker) TryLock(ctx context.Context) error {
	// Try to set the lock with NX (only if not exists) and EX (expiration)
	result, err := l.client.SetNX(ctx, l.lockKey, l.lockValue, l.lockTTL)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !result {
		return ErrLocked
	}

	// Start background renewal process
	l.ctx, l.cancel = context.WithCancel(ctx)
	go l.renewLock()

	return nil
}

// Unlock releases the distributed lock
func (l *Locker) Unlock() error {
	if l.cancel != nil {
		l.cancel()
	}

	// Lua script to ensure we only delete our own lock
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	_, err := l.client.Eval(context.Background(), script, []string{l.lockKey}, l.lockValue)
	return err
}

// renewLock periodically renews the lock to prevent expiration
func (l *Locker) renewLock() {
	ticker := time.NewTicker(l.renewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			// Lua script to renew lock only if we own it
			script := `
				if redis.call("get", KEYS[1]) == ARGV[1] then
					return redis.call("expire", KEYS[1], ARGV[2])
				else
					return 0
				end
			`

			result, err := l.client.Eval(l.ctx, script, []string{l.lockKey}, l.lockValue, int(l.lockTTL.Seconds()))
			if err != nil || result.(int64) == 0 {
				// Failed to renew or lost the lock
				l.cancel()
				return
			}
		}
	}
}

// IsLocked checks if the lock is currently held by this instance
func (l *Locker) IsLocked() bool {
	val, err := l.client.Get(context.Background(), l.lockKey)
	if err != nil {
		return false
	}

	return val == l.lockValue
}

// LockMiddleware creates a middleware that ensures only one instance can run
func LockMiddleware(locker *Locker) Middleware {
	return func(ctx context.Context) error {
		return locker.TryLock(ctx)
	}
}

// UnlockMiddleware creates a middleware that releases the lock after execution
func UnlockMiddleware(locker *Locker) Middleware {
	return func(ctx context.Context) error {
		return locker.Unlock()
	}
}

// CancelRunningWorker creates a middleware that cancels execution if another instance acquires the lock
type CancelRunningWorker struct {
	locker *Locker
	cancel context.CancelFunc
}

// NewCancelRunningWorker creates a new cancel middleware
func NewCancelRunningWorker(locker *Locker) *CancelRunningWorker {
	return &CancelRunningWorker{
		locker: locker,
	}
}

// Middleware returns a middleware that monitors lock status and cancels if lock is lost
func (c *CancelRunningWorker) Middleware() Middleware {
	return func(ctx context.Context) error {
		// Check if we still have the lock
		if !c.locker.IsLocked() {
			return ErrLocked
		}

		// Set up context cancellation monitoring
		ctx, cancel := context.WithCancel(ctx)
		c.cancel = cancel

		// Start monitoring in background
		go c.monitorLock(ctx)

		return nil
	}
}

// monitorLock continuously checks if the lock is still held
func (c *CancelRunningWorker) monitorLock(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.locker.IsLocked() {
				// Lock lost, cancel the context
				if c.cancel != nil {
					c.cancel()
				}
				return
			}
		}
	}
}
