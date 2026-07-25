package rabbitmq

import (
	"crypto/rand"
	"math/big"
	"time"

	"alga/logger"
)

// RetrySchedule is the single authoritative retry/backoff policy for every
// retry queue in the RabbitMQ topology. It follows an exponential backoff of
// 1m -> 5m -> 15m -> 1h. Each entry is one retry stage; a message is sent
// through the corresponding retry queue and dead-lettered to the terminal DLQ
// only after the final stage is exhausted.
var RetrySchedule = []time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	time.Hour,
}

// jitterFactor bounds the random spread applied to a base backoff so that
// correlated failures do not all re-deliver in lockstep (thundering herd).
const jitterFactor = 0.2

// jitterCeil returns the upper bound of the jittered backoff for a stage. Retry
// queues are declared with this as their fixed x-message-ttl so the per-message
// jittered expiration (which may be up to +jitterFactor) is never clipped by
// RabbitMQ's min(queueTTL, messageExpiration) rule.
func jitterCeil(d time.Duration) time.Duration {
	return d + time.Duration(float64(d)*jitterFactor)
}

// withJitter returns d adjusted by ±jitterFactor using a cryptographically
// random source, so the spread cannot be predicted or induced. On failure to
// read randomness it returns d unchanged (no jitter) rather than risk a panic.
func withJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	span := int64(float64(d) * 2 * jitterFactor)
	offset, err := rand.Int(rand.Reader, big.NewInt(span+1))
	if err != nil {
		logger.Warn("retry jitter RNG unavailable; using base delay", "error", err)
		return d
	}
	// offset in [0, 2*jitterFactor); shift into [-jitterFactor, +jitterFactor).
	adjust := offset.Int64() - int64(float64(d)*jitterFactor)
	return d + time.Duration(adjust)
}

// retryExpiration returns the jittered per-message TTL for the given 1-based
// retry attempt. ok is false when the attempt is outside the schedule, so the
// caller can refuse to publish instead of silently dropping the message.
func retryExpiration(retryCount int) (time.Duration, bool) {
	stage := retryCount - 1
	if stage < 0 || stage >= len(RetrySchedule) {
		return 0, false
	}
	return withJitter(RetrySchedule[stage]), true
}

// retryTTLms returns the RetrySchedule backoff for a 0-based stage as
// milliseconds, sized so the per-message jittered expiration is never clipped
// by RabbitMQ's min(queueTTL, messageExpiration) rule. It is the single source
// of truth for the retry-queue x-message-ttl values declared in the topology.
func retryTTLms(stage int) int32 {
	return int32(jitterCeil(RetrySchedule[stage]).Milliseconds()) //#nosec G115 -- RetrySchedule bounded to <=1h; jitterCeil <=+20% => <=4.32M ms, well under math.MaxInt32
}
