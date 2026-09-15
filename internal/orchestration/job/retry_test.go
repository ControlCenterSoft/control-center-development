package job

import (
	"testing"
	"time"
)

func TestRetryDelayPreservesOrdinaryBackoff(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		policy  RetryPolicy
		want    time.Duration
	}{
		{name: "default base", attempt: 1, want: time.Second},
		{name: "exponential", attempt: 3, policy: RetryPolicy{BaseDelay: 2 * time.Second}, want: 8 * time.Second},
		{name: "explicit cap", attempt: 3, policy: RetryPolicy{BaseDelay: 2 * time.Second, MaxDelay: 5 * time.Second}, want: 5 * time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RetryDelay(test.attempt, test.policy); got != test.want {
				t.Fatalf("RetryDelay(%d, %#v) = %s, want %s", test.attempt, test.policy, got, test.want)
			}
		})
	}
}

func TestRetryDelaySaturatesBeforeDurationOverflow(t *testing.T) {
	base := maxRetryDelay/2 + 1
	got := RetryDelay(2, RetryPolicy{BaseDelay: base})
	if got != maxRetryDelay {
		t.Fatalf("RetryDelay overflow saturation = %s, want %s", got, maxRetryDelay)
	}
	if got <= 0 {
		t.Fatalf("RetryDelay overflow saturation must stay positive, got %s", got)
	}
}

func TestRetryDelayHonorsCapBeforeOverflow(t *testing.T) {
	cap := maxRetryDelay - time.Second
	got := RetryDelay(3, RetryPolicy{BaseDelay: maxRetryDelay / 2, MaxDelay: cap})
	if got != cap {
		t.Fatalf("RetryDelay overflow cap = %s, want %s", got, cap)
	}
}
