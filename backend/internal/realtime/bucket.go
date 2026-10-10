package realtime

import "time"

// bucket is a token-bucket rate limiter for one connection's client frames.
//
// It holds up to `burst` tokens and refills `perSec` tokens per second; each
// frame spends one. Normal use (send a message, start typing the next one,
// switch chats) fits in the burst; a client spamming frames runs dry and its
// frames are dropped. A fixed "one frame per N ms" rule dropped legitimate
// frames: see Part 2.1 learnings.
type bucket struct {
	tokens, burst, perSec float64
	last                  time.Time
}

func newBucket(burst, perSec float64) *bucket {
	return &bucket{tokens: burst, burst: burst, perSec: perSec}
}

func (b *bucket) allow(now time.Time) bool {
	if !b.last.IsZero() {
		b.tokens = min(b.burst, b.tokens+now.Sub(b.last).Seconds()*b.perSec)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
