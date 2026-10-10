package realtime

import (
	"testing"
	"time"
)

func TestBucket_BurstThenRefill(t *testing.T) {
	now := time.Unix(0, 0)
	b := newBucket(3, 2) // burst of 3, refills 2 per second

	allowed := 0
	for range 10 {
		if b.allow(now) {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("burst allowed %d, want 3", allowed)
	}
	if b.allow(now.Add(400 * time.Millisecond)) {
		t.Error("allowed before a full token refilled")
	}
	if !b.allow(now.Add(600 * time.Millisecond)) {
		t.Error("not allowed after 0.6 s (1.2 tokens refilled)")
	}
	// A long pause refills to the burst size, not beyond.
	later := now.Add(time.Hour)
	allowed = 0
	for range 10 {
		if b.allow(later) {
			allowed++
		}
	}
	if allowed != 3 {
		t.Errorf("after an hour allowed %d, want 3 (capped at burst)", allowed)
	}
}
