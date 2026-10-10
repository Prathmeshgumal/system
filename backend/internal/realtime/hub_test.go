package realtime

import (
	"encoding/json"
	"testing"
	"time"
)

func recv(t *testing.T, c *Client) map[string]any {
	t.Helper()
	select {
	case b := <-c.send:
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	case <-time.After(time.Second):
		t.Fatal("no event received")
		return nil
	}
}

func TestHub_PublishReachesEveryTabOfEveryRecipient(t *testing.T) {
	h := NewHub()
	aliceTab1, aliceTab2 := h.Register(1), h.Register(1)
	bob := h.Register(2)
	carol := h.Register(3)

	h.Publish([]int64{1, 2}, map[string]string{"type": "hello"})

	for _, c := range []*Client{aliceTab1, aliceTab2, bob} {
		if got := recv(t, c); got["type"] != "hello" {
			t.Errorf("got %v", got)
		}
	}
	select {
	case <-carol.send:
		t.Error("carol wasn't a recipient but got the event")
	default:
	}
	if n := h.Connections(); n != 4 {
		t.Errorf("Connections = %d, want 4", n)
	}
}

func TestHub_UnregisterStopsDelivery(t *testing.T) {
	h := NewHub()
	c := h.Register(1)
	h.Unregister(c)
	h.Unregister(c) // twice is harmless
	h.Publish([]int64{1}, "x")
	select {
	case <-c.send:
		t.Error("unregistered client got an event")
	default:
	}
	if h.Connections() != 0 {
		t.Error("client still counted")
	}
}

func TestHub_SlowClientIsEvictedWithoutBlockingOthers(t *testing.T) {
	h := NewHub()
	slow := h.Register(1) // nobody reads its outbox
	fast := h.Register(2)

	done := make(chan struct{})
	go func() {
		for i := range outboxSize + 10 {
			h.Publish([]int64{1, 2}, i)
			<-fast.send // the fast client keeps up
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow client")
	}
	select {
	case <-slow.Kicked():
	default:
		t.Fatal("slow client was not evicted")
	}
	if h.Connections() != 1 {
		t.Errorf("Connections = %d, want 1", h.Connections())
	}
}
