package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type event struct {
	Type    string   `json:"type"`
	ChatID  int64    `json:"chat_id"`
	Seq     int64    `json:"seq"`
	UserID  int64    `json:"user_id"`
	Message *msgResp `json:"message"`
}

// connect gets a ticket and opens a WebSocket, like the browser does.
func (c *client) connect() *websocket.Conn {
	c.t.Helper()
	var tk struct{ Ticket string }
	if code := c.do("POST", "/api/ws/ticket", nil, &tk); code != 200 {
		c.t.Fatalf("ticket = %d", code)
	}
	url := "ws" + strings.TrimPrefix(c.base, "http") + "/ws?ticket=" + tk.Ticket
	conn, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		c.t.Fatalf("dial: %v", err)
	}
	c.t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func next(t *testing.T, conn *websocket.Conn) event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, b, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	var e event
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRealtime_MessageEventsReachEveryMember(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)

	bobWS := bob.connect()
	aliceWS := alice.connect() // the sender's own tab gets it too (other tabs stay in sync)

	m := alice.send(c.ID, "live!")
	for name, ws := range map[string]*websocket.Conn{"bob": bobWS, "alice": aliceWS} {
		e := next(t, ws)
		if e.Type != "message.created" || e.ChatID != c.ID || e.Message == nil || e.Message.Body != "live!" ||
			e.Message.ClientID != m.ClientID || e.Message.Sender.Username != "alice" {
			t.Errorf("%s got %+v", name, e)
		}
	}

	alice.do("PATCH", fmt.Sprintf("/api/messages/%d", m.ID), map[string]string{"body": "edited"}, nil)
	if e := next(t, bobWS); e.Type != "message.updated" || e.Message.Body != "edited" {
		t.Errorf("edit event = %+v", e)
	}
	alice.do("DELETE", fmt.Sprintf("/api/messages/%d", m.ID), nil, nil)
	if e := next(t, bobWS); e.Type != "message.deleted" || e.Message.DeletedAt == nil {
		t.Errorf("delete event = %+v", e)
	}
}

func TestRealtime_ReadMarkerSyncsMyOtherTabs(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)
	alice.send(c.ID, "1")

	tab2 := bob.connect()
	bob.do("POST", fmt.Sprintf("/api/chats/%d/read", c.ID), map[string]int64{"seq": 1}, nil)
	if e := next(t, tab2); e.Type != "chat.read" || e.Seq != 1 {
		t.Errorf("got %+v", e)
	}
}

func TestRealtime_MembershipEvents(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	bobWS := bob.connect()

	var g chatResp
	alice.do("POST", "/api/chats/group", map[string]any{"name": "g", "member_ids": []int64{bob.UserID}}, &g)
	if e := next(t, bobWS); e.Type != "chat.created" || e.ChatID != g.ID {
		t.Errorf("added: %+v", e)
	}
	alice.do("DELETE", fmt.Sprintf("/api/chats/%d/members/%d", g.ID, bob.UserID), nil, nil)
	if e := next(t, bobWS); e.Type != "chat.removed" || e.ChatID != g.ID {
		t.Errorf("removed: %+v", e)
	}
}

func TestRealtime_RejectsBadTicketAndForeignOrigin(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	_, res, err := websocket.Dial(context.Background(), wsURL+"?ticket=forged", nil)
	if err == nil || res == nil || res.StatusCode != http.StatusUnauthorized {
		t.Errorf("forged ticket: err=%v status=%v", err, res)
	}

	// An access token is not a ticket.
	_, res, err = websocket.Dial(context.Background(), wsURL+"?ticket="+alice.token, nil)
	if err == nil || res.StatusCode != http.StatusUnauthorized {
		t.Errorf("access token as ticket: err=%v", err)
	}

	// A page on another site tries to open a socket to us (cross-site WebSocket hijacking).
	var tk struct{ Ticket string }
	alice.do("POST", "/api/ws/ticket", nil, &tk)
	_, res, err = websocket.Dial(context.Background(), wsURL+"?ticket="+tk.Ticket, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
	})
	if err == nil || res.StatusCode != http.StatusForbidden {
		t.Errorf("foreign origin: err=%v", err)
	}
}

func TestRealtime_ConnectionOutlivesServerWriteTimeout(t *testing.T) {
	srv := newServer(t, func(s *http.Server) {
		s.WriteTimeout = 300 * time.Millisecond
		s.ReadTimeout = 300 * time.Millisecond
	})
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)
	ws := bob.connect()

	time.Sleep(time.Second) // well past both timeouts
	alice.send(c.ID, "still there?")
	if e := next(t, ws); e.Type != "message.created" {
		t.Errorf("got %+v", e)
	}
}
