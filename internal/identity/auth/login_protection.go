package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	DefaultLoginFailureWindow       = 10 * time.Minute
	DefaultLoginAccountFailureLimit = 8
	DefaultLoginSourceFailureLimit  = 30
	DefaultLoginBlockDuration       = 15 * time.Minute
	DefaultLoginProtectionMaxKeys   = 10000
)

type LoginProtectionPolicy struct {
	FailureWindow       time.Duration
	AccountFailureLimit int
	SourceFailureLimit  int
	BlockDuration       time.Duration
	MaxTrackedKeys      int
}

func DefaultLoginProtectionPolicy() LoginProtectionPolicy {
	return LoginProtectionPolicy{
		FailureWindow:       DefaultLoginFailureWindow,
		AccountFailureLimit: DefaultLoginAccountFailureLimit,
		SourceFailureLimit:  DefaultLoginSourceFailureLimit,
		BlockDuration:       DefaultLoginBlockDuration,
		MaxTrackedKeys:      DefaultLoginProtectionMaxKeys,
	}
}

func (p LoginProtectionPolicy) validate() error {
	if p.FailureWindow < time.Minute || p.FailureWindow > time.Hour {
		return errors.New("login failure window outside allowed range")
	}
	if p.AccountFailureLimit < 2 || p.AccountFailureLimit > 100 {
		return errors.New("login account failure limit outside allowed range")
	}
	if p.SourceFailureLimit < p.AccountFailureLimit || p.SourceFailureLimit > 1000 {
		return errors.New("login source failure limit outside allowed range")
	}
	if p.BlockDuration < time.Minute || p.BlockDuration > 24*time.Hour {
		return errors.New("login block duration outside allowed range")
	}
	if p.MaxTrackedKeys < 128 || p.MaxTrackedKeys > 1000000 {
		return errors.New("login protection key bound outside allowed range")
	}
	return nil
}

type loginAttemptBucket struct {
	WindowStartedAt time.Time
	Failures        int
	BlockedUntil    time.Time
	TouchedAt       time.Time
}

type LoginProtector struct {
	mu      sync.Mutex
	policy  LoginProtectionPolicy
	buckets map[string]loginAttemptBucket
}

func newLoginProtector(policy LoginProtectionPolicy) *LoginProtector {
	return &LoginProtector{policy: policy, buckets: make(map[string]loginAttemptBucket)}
}

func (p *LoginProtector) Blocked(username, sourceIP string, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	now = now.UTC()
	accountBlocked := p.blockedLocked(loginAccountProtectionKey(username), now)
	sourceBlocked := p.blockedLocked(loginSourceProtectionKey(sourceIP), now)
	return accountBlocked || sourceBlocked
}

func (p *LoginProtector) RecordFailure(username, sourceIP string, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	now = now.UTC()
	accountBlocked := p.recordFailureLocked(loginAccountProtectionKey(username), p.policy.AccountFailureLimit, now)
	sourceBlocked := p.recordFailureLocked(loginSourceProtectionKey(sourceIP), p.policy.SourceFailureLimit, now)
	return accountBlocked || sourceBlocked
}

func (p *LoginProtector) RecordSuccess(username string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.buckets, loginAccountProtectionKey(username))
}

func (p *LoginProtector) blockedLocked(key string, now time.Time) bool {
	bucket, ok := p.buckets[key]
	if !ok {
		return false
	}
	bucket = p.normalizeBucket(bucket, now)
	if bucket.Failures == 0 && bucket.BlockedUntil.IsZero() {
		delete(p.buckets, key)
		return false
	}
	bucket.TouchedAt = now
	p.buckets[key] = bucket
	return !bucket.BlockedUntil.IsZero() && now.Before(bucket.BlockedUntil)
}

func (p *LoginProtector) recordFailureLocked(key string, limit int, now time.Time) bool {
	bucket := p.normalizeBucket(p.buckets[key], now)
	if !bucket.BlockedUntil.IsZero() && now.Before(bucket.BlockedUntil) {
		bucket.TouchedAt = now
		if !p.putBucketLocked(key, bucket, now) {
			return true
		}
		return true
	}
	if bucket.WindowStartedAt.IsZero() {
		bucket.WindowStartedAt = now
	}
	bucket.Failures++
	if bucket.Failures >= limit {
		bucket.BlockedUntil = now.Add(p.policy.BlockDuration)
	}
	bucket.TouchedAt = now
	if !p.putBucketLocked(key, bucket, now) {
		return true
	}
	return !bucket.BlockedUntil.IsZero() && now.Before(bucket.BlockedUntil)
}

func (p *LoginProtector) normalizeBucket(bucket loginAttemptBucket, now time.Time) loginAttemptBucket {
	if !bucket.BlockedUntil.IsZero() && !now.Before(bucket.BlockedUntil) {
		return loginAttemptBucket{}
	}
	if bucket.BlockedUntil.IsZero() && !bucket.WindowStartedAt.IsZero() && !now.Before(bucket.WindowStartedAt.Add(p.policy.FailureWindow)) {
		return loginAttemptBucket{}
	}
	return bucket
}

func (p *LoginProtector) putBucketLocked(key string, bucket loginAttemptBucket, now time.Time) bool {
	if _, exists := p.buckets[key]; exists {
		p.buckets[key] = bucket
		return true
	}
	if len(p.buckets) >= p.policy.MaxTrackedKeys {
		p.evictOldestEvictableLocked(now)
	}
	if len(p.buckets) >= p.policy.MaxTrackedKeys {
		return false
	}
	p.buckets[key] = bucket
	return true
}

func (p *LoginProtector) evictOldestEvictableLocked(now time.Time) {
	var oldestKey string
	var oldest time.Time
	found := false

	for key, bucket := range p.buckets {
		normalized := p.normalizeBucket(bucket, now)
		if normalized.Failures == 0 && normalized.BlockedUntil.IsZero() {
			delete(p.buckets, key)
			return
		}
		if !normalized.BlockedUntil.IsZero() && now.Before(normalized.BlockedUntil) {
			continue
		}
		if !found || normalized.TouchedAt.Before(oldest) {
			oldestKey = key
			oldest = normalized.TouchedAt
			found = true
		}
	}
	if found {
		delete(p.buckets, oldestKey)
	}
}

func loginAccountProtectionKey(username string) string {
	return protectedLoginKey("account", normalizeUsername(username))
}

func loginSourceProtectionKey(sourceIP string) string {
	sourceIP = strings.TrimSpace(sourceIP)
	if parsed := net.ParseIP(sourceIP); parsed != nil {
		sourceIP = parsed.String()
	} else {
		sourceIP = strings.ToLower(sourceIP)
	}
	if sourceIP == "" {
		sourceIP = "unknown"
	}
	return protectedLoginKey("source", sourceIP)
}

func protectedLoginKey(kind, value string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + value))
	return hex.EncodeToString(sum[:])
}
