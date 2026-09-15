package job

import "time"

const maxRetryDelay = time.Duration(1<<63 - 1)

// RetryDelay returns the exponential retry backoff for an attempt without
// allowing time.Duration arithmetic to wrap negative. An explicit MaxDelay
// remains authoritative; otherwise the delay saturates at the largest positive
// duration representable by time.Duration.
func RetryDelay(attempt int, policy RetryPolicy) time.Duration {
	base := policy.BaseDelay
	if base <= 0 {
		base = time.Second
	}
	if policy.MaxDelay > 0 && base >= policy.MaxDelay {
		return policy.MaxDelay
	}

	delay := base
	for i := 1; i < attempt; i++ {
		if policy.MaxDelay > 0 && delay >= policy.MaxDelay {
			return policy.MaxDelay
		}
		if delay > maxRetryDelay/2 {
			if policy.MaxDelay > 0 {
				return policy.MaxDelay
			}
			return maxRetryDelay
		}
		delay *= 2
	}
	if policy.MaxDelay > 0 && delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	return delay
}
