package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestLoginProtectorPreservesActiveAccountBlockUnderKeyChurn(t *testing.T) {
	policy := testLoginProtectionPolicy()
	protector := newLoginProtector(policy)
	now := time.Date(2026, 9, 15, 11, 30, 0, 0, time.UTC)

	if protector.RecordFailure("protected-user", "192.0.2.10", now) {
		t.Fatal("first protected-user failure unexpectedly blocked")
	}
	if !protector.RecordFailure("protected-user", "192.0.2.11", now.Add(time.Second)) {
		t.Fatal("protected-user did not reach the account block threshold")
	}

	protectedKey := loginAccountProtectionKey("protected-user")
	for i := 0; i < policy.MaxTrackedKeys*2; i++ {
		protector.RecordFailure(
			fmt.Sprintf("churn-user-%d", i),
			fmt.Sprintf("198.51.%d.%d", (i/250)%250, (i%250)+1),
			now.Add(2*time.Second+time.Duration(i)*time.Millisecond),
		)
	}

	if got := len(protector.buckets); got > policy.MaxTrackedKeys {
		t.Fatalf("tracked login keys exceeded bound: got=%d max=%d", got, policy.MaxTrackedKeys)
	}
	if _, ok := protector.buckets[protectedKey]; !ok {
		t.Fatal("active account block was evicted under key churn")
	}
	if !protector.Blocked("protected-user", "203.0.113.200", now.Add(time.Minute)) {
		t.Fatal("active account block was weakened before block expiry")
	}
}

func TestLoginProtectorFailsClosedWhenAllTrackedKeysAreBlocked(t *testing.T) {
	policy := testLoginProtectionPolicy()
	protector := newLoginProtector(policy)
	now := time.Date(2026, 9, 15, 11, 30, 0, 0, time.UTC)
	blockedUntil := now.Add(policy.BlockDuration)

	oldestKey := ""
	for i := 0; i < policy.MaxTrackedKeys; i++ {
		key := protectedLoginKey("saturated", fmt.Sprintf("%d", i))
		if i == 0 {
			oldestKey = key
		}
		protector.buckets[key] = loginAttemptBucket{
			WindowStartedAt: now.Add(-time.Minute),
			Failures:        policy.AccountFailureLimit,
			BlockedUntil:    blockedUntil,
			TouchedAt:       now.Add(time.Duration(i) * time.Millisecond),
		}
	}

	if !protector.RecordFailure("new-user", "203.0.113.250", now.Add(time.Second)) {
		t.Fatal("saturated protector admitted a new failure instead of failing closed")
	}
	if got := len(protector.buckets); got != policy.MaxTrackedKeys {
		t.Fatalf("tracked key count changed under saturation: got=%d want=%d", got, policy.MaxTrackedKeys)
	}
	if _, ok := protector.buckets[oldestKey]; !ok {
		t.Fatal("oldest active block was evicted under saturation")
	}
	if _, ok := protector.buckets[loginAccountProtectionKey("new-user")]; ok {
		t.Fatal("new account key displaced an active block under saturation")
	}
	if _, ok := protector.buckets[loginSourceProtectionKey("203.0.113.250")]; ok {
		t.Fatal("new source key displaced an active block under saturation")
	}
}
