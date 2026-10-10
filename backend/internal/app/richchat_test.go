package app_test

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type reaction struct {
	Emoji   string  `json:"emoji"`
	UserIDs []int64 `json:"user_ids"`
}

type richMsg struct {
	msgResp
	Reactions []reaction `json:"reactions"`
	ReplyTo   *struct {
		ID      int64  `json:"id"`
		Body    string `json:"body"`
		Deleted bool   `json:"deleted"`
		Sender  struct {
			Username string `json:"username"`
		} `json:"sender"`
	} `json:"reply_to"`
}

func TestReactions_AddRemoveAndBroadcast(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	eve := newClient(t, srv).signup("eve")
	c := alice.direct(bob)
	m := alice.send(c.ID, "pizza tonight?")
	bobWS := bob.connect()

	path := fmt.Sprintf("/api/messages/%d/reactions/%s", m.ID, url.PathEscape("👍"))
	var got richMsg
	for range 2 { // twice: idempotent
		if code := bob.do("PUT", path, nil, &got); code != 200 {
			t.Fatalf("react = %d", code)
		}
	}
	if len(got.Reactions) != 1 || got.Reactions[0].Emoji != "👍" || len(got.Reactions[0].UserIDs) != 1 {
		t.Fatalf("reactions = %+v", got.Reactions)
	}
	if e := next(t, bobWS); e.Type != "message.updated" || e.Message.ID != m.ID {
		t.Errorf("event = %+v", e)
	}

	// The reaction shows in history too.
	var h struct{ Messages []richMsg }
	alice.do("GET", fmt.Sprintf("/api/chats/%d/messages", c.ID), nil, &h)
	if len(h.Messages[0].Reactions) != 1 {
		t.Errorf("history reactions = %+v", h.Messages[0].Reactions)
	}

	bob.do("DELETE", path, nil, &got)
	if len(got.Reactions) != 0 {
		t.Errorf("after delete = %+v", got.Reactions)
	}
	if code := bob.do("PUT", fmt.Sprintf("/api/messages/%d/reactions/%s", m.ID, url.PathEscape("🍕")), nil, nil); code != 400 {
		t.Errorf("emoji outside the set = %d, want 400", code)
	}
	if code := eve.do("PUT", path, nil, nil); code != 404 {
		t.Errorf("outsider reacts = %d, want 404", code)
	}
}

func TestReplies_QuoteFollowsTheOriginal(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)
	orig := alice.send(c.ID, "lunch at 1?")

	var reply richMsg
	code := bob.do("POST", fmt.Sprintf("/api/chats/%d/messages", c.ID),
		map[string]any{"body": "yes", "client_id": uuid(), "reply_to_id": orig.ID}, &reply)
	if code != 201 || reply.ReplyTo == nil || reply.ReplyTo.Body != "lunch at 1?" || reply.ReplyTo.Sender.Username != "alice" {
		t.Fatalf("reply = %d %+v", code, reply.ReplyTo)
	}

	// The quote is read live: editing and deleting the original shows up in it.
	alice.do("PATCH", fmt.Sprintf("/api/messages/%d", orig.ID), map[string]string{"body": "lunch at 2?"}, nil)
	var h struct{ Messages []richMsg }
	alice.do("GET", fmt.Sprintf("/api/chats/%d/messages", c.ID), nil, &h)
	if q := h.Messages[1].ReplyTo; q == nil || q.Body != "lunch at 2?" {
		t.Errorf("quote after edit = %+v", q)
	}
	alice.do("DELETE", fmt.Sprintf("/api/messages/%d", orig.ID), nil, nil)
	alice.do("GET", fmt.Sprintf("/api/chats/%d/messages", c.ID), nil, &h)
	if q := h.Messages[1].ReplyTo; q == nil || !q.Deleted || q.Body != "" {
		t.Errorf("quote after delete = %+v", q)
	}

	// A reply must point into the same chat.
	other := alice.direct(newClient(t, srv).signup("carol"))
	elsewhere := alice.send(other.ID, "x")
	if code := bob.do("POST", fmt.Sprintf("/api/chats/%d/messages", c.ID),
		map[string]any{"body": "?", "client_id": uuid(), "reply_to_id": elsewhere.ID}, nil); code != 400 {
		t.Errorf("reply across chats = %d, want 400", code)
	}
}

func TestReadReceipts_GoToEveryMember(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)
	alice.send(c.ID, "1")
	aliceWS := alice.connect()

	bob.do("POST", fmt.Sprintf("/api/chats/%d/read", c.ID), map[string]int64{"seq": 1}, nil)
	e := next(t, aliceWS)
	if e.Type != "chat.read" || e.Seq != 1 || e.UserID != bob.UserID {
		t.Errorf("alice got %+v, want bob's read receipt", e)
	}
}

func TestTyping_RelayedToOthersOnly(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	eve := newClient(t, srv).signup("eve")
	c := alice.direct(bob)
	aliceWS, bobWS, eveWS := alice.connect(), bob.connect(), eve.connect()

	typing := func(ws *websocket.Conn, chatID int64) {
		t.Helper()
		frame := fmt.Sprintf(`{"type":"typing","chat_id":%d}`, chatID)
		if err := ws.Write(context.Background(), websocket.MessageText, []byte(frame)); err != nil {
			t.Fatal(err)
		}
	}

	typing(aliceWS, c.ID)
	if e := next(t, bobWS); e.Type != "typing" || e.ChatID != c.ID || e.UserID != alice.UserID {
		t.Errorf("bob got %+v", e)
	}
	// Eve isn't in the chat; typing into it is ignored, and she hears nothing.
	time.Sleep(600 * time.Millisecond) // past the per-connection rate limit
	typing(eveWS, c.ID)
	expectNothing(t, aliceWS, "the sender's own socket")
	expectNothing(t, eveWS, "an outsider")
	expectNothing(t, bobWS, "a typing event from a non-member")
}

func TestTyping_RateLimitedPerConnection(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)
	aliceWS, bobWS := alice.connect(), bob.connect()

	frame := []byte(fmt.Sprintf(`{"type":"typing","chat_id":%d}`, c.ID))
	for range 20 {
		_ = aliceWS.Write(context.Background(), websocket.MessageText, frame)
	}
	next(t, bobWS) // the first one gets through…
	expectNothing(t, bobWS, "19 more typing events in the same instant")
}

func expectNothing(t *testing.T, ws *websocket.Conn, what string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, b, err := ws.Read(ctx); err == nil {
		t.Errorf("%s: got %s, want nothing", what, b)
	}
}
