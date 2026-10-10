// Package realtime keeps track of who is connected over WebSocket and pushes
// events to them. Design: knowledge-base/adr/0003-websocket-protocol.md.
package realtime

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// outboxSize is how many events may wait for one connection. A healthy client
// drains it in milliseconds; one that falls this far behind is too slow and
// gets disconnected (it reconnects and catches up over REST).
const outboxSize = 64

// Client is one WebSocket connection (one browser tab).
type Client struct {
	userID int64
	send   chan []byte   // the outbox; only the connection's writer reads it
	kicked chan struct{} // closed when the hub evicts this client
	once   sync.Once
	reason string // why it was kicked; written before kicked is closed
}

// Kicked is closed when the hub gave up on this client; Reason says why.
func (c *Client) Kicked() <-chan struct{} { return c.kicked }
func (c *Client) Reason() string          { return c.reason }

func (c *Client) kick(reason string) {
	c.once.Do(func() {
		c.reason = reason
		close(c.kicked)
	})
}

// Hub maps user id → that user's open connections. One per server process;
// Phase 3 makes this cross-server with Redis pub/sub.
type Hub struct {
	mu    sync.Mutex
	conns map[int64]map[*Client]struct{}
	count int
}

func NewHub() *Hub { return &Hub{conns: map[int64]map[*Client]struct{}{}} }

func (h *Hub) Register(userID int64) *Client {
	c := &Client{userID: userID, send: make(chan []byte, outboxSize), kicked: make(chan struct{})}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[userID] == nil {
		h.conns[userID] = map[*Client]struct{}{}
	}
	h.conns[userID][c] = struct{}{}
	h.count++
	return c
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.remove(c)
}

// remove needs h.mu held.
func (h *Hub) remove(c *Client) {
	set := h.conns[c.userID]
	if _, ok := set[c]; !ok {
		return
	}
	delete(set, c)
	if len(set) == 0 {
		delete(h.conns, c.userID)
	}
	h.count--
}

// Publish sends event to every connection of every user in userIDs.
// It never blocks: the event is encoded once, then dropped into each outbox;
// a full outbox means a stuck client, which is evicted instead of waited on.
func (h *Hub) Publish(userIDs []int64, event any) {
	b, err := json.Marshal(event)
	if err != nil {
		slog.Error("encode event", "err", err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, id := range userIDs {
		for c := range h.conns[id] {
			select {
			case c.send <- b:
			default:
				slog.Warn("websocket client too slow, disconnecting", "user_id", id)
				h.remove(c)
				c.kick(ReasonTooSlow)
			}
		}
	}
}

const (
	ReasonTooSlow  = "too slow"
	ReasonShutdown = "server shutting down"
)

// Shutdown disconnects everyone. http.Server.Shutdown doesn't wait for or
// close WebSockets (they're hijacked connections), so main calls this.
func (h *Hub) Shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.conns {
		for c := range set {
			h.remove(c)
			c.kick(ReasonShutdown)
		}
	}
}

// Connections is the number of open connections (for tests and, later, metrics).
func (h *Hub) Connections() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}
