package app_test

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"testing"
)

type chatResp struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	MyRole      string `json:"my_role"`
	Unread      int64  `json:"unread"`
	MemberCount int64  `json:"member_count"`
	OtherUser   *struct {
		Username string `json:"username"`
	} `json:"other_user"`
	LastMessage *msgResp `json:"last_message"`
	Members     []struct {
		Role string `json:"role"`
		User struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	} `json:"members"`
}

type msgResp struct {
	ID       int64  `json:"id"`
	ChatID   int64  `json:"chat_id"`
	Seq      int64  `json:"seq"`
	Body     string `json:"body"`
	ClientID string `json:"client_id"`
	Sender   *struct {
		Username string `json:"username"`
	} `json:"sender"`
	EditedAt  *string `json:"edited_at"`
	DeletedAt *string `json:"deleted_at"`
}

type historyResp struct {
	Messages []msgResp `json:"messages"`
	HasMore  bool      `json:"has_more"`
}

func uuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (c *client) direct(other *client) chatResp {
	c.t.Helper()
	var out chatResp
	if code := c.do("POST", "/api/chats/direct", map[string]int64{"user_id": other.UserID}, &out); code != 201 && code != 200 {
		c.t.Fatalf("start direct = %d", code)
	}
	return out
}

func (c *client) send(chatID int64, body string) msgResp {
	c.t.Helper()
	var out msgResp
	if code := c.do("POST", fmt.Sprintf("/api/chats/%d/messages", chatID),
		map[string]string{"body": body, "client_id": uuid()}, &out); code != 201 {
		c.t.Fatalf("send = %d", code)
	}
	return out
}

func TestChat_DirectChatFlow(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")

	c := alice.direct(bob)
	if c.Kind != "direct" || c.OtherUser == nil || c.OtherUser.Username != "bob" || len(c.Members) != 2 {
		t.Fatalf("direct chat = %+v", c)
	}
	// Bob starting it from his side finds the same chat (200, not 201).
	var again chatResp
	if code := bob.do("POST", "/api/chats/direct", map[string]int64{"user_id": alice.UserID}, &again); code != 200 || again.ID != c.ID {
		t.Fatalf("second start = %d %+v", code, again)
	}

	m := alice.send(c.ID, "hi bob")
	if m.Seq != 1 || m.Sender == nil || m.Sender.Username != "alice" {
		t.Fatalf("message = %+v", m)
	}

	var list struct{ Chats []chatResp }
	bob.do("GET", "/api/chats", nil, &list)
	if len(list.Chats) != 1 || list.Chats[0].Unread != 1 || list.Chats[0].LastMessage.Body != "hi bob" ||
		list.Chats[0].OtherUser.Username != "alice" {
		t.Fatalf("bob's list = %+v", list.Chats)
	}

	if code := bob.do("POST", fmt.Sprintf("/api/chats/%d/read", c.ID), map[string]int64{"seq": 1}, nil); code != 200 {
		t.Fatalf("mark read = %d", code)
	}
	bob.do("GET", "/api/chats", nil, &list)
	if list.Chats[0].Unread != 0 {
		t.Errorf("unread after read = %d", list.Chats[0].Unread)
	}
}

func TestChat_RetrySendIsIdempotent(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)

	body := map[string]string{"body": "once", "client_id": uuid()}
	var m1, m2 msgResp
	c1 := alice.do("POST", fmt.Sprintf("/api/chats/%d/messages", c.ID), body, &m1)
	c2 := alice.do("POST", fmt.Sprintf("/api/chats/%d/messages", c.ID), body, &m2)
	if c1 != 201 || c2 != 200 || m1.ID != m2.ID {
		t.Fatalf("first = %d #%d, retry = %d #%d", c1, m1.ID, c2, m2.ID)
	}
}

func TestChat_OutsidersSeeNothing(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	eve := newClient(t, srv).signup("eve")
	c := alice.direct(bob)
	m := alice.send(c.ID, "secret")

	for name, req := range map[string][2]string{
		"get chat":     {"GET", fmt.Sprintf("/api/chats/%d", c.ID)},
		"history":      {"GET", fmt.Sprintf("/api/chats/%d/messages", c.ID)},
		"edit message": {"PATCH", fmt.Sprintf("/api/messages/%d", m.ID)},
	} {
		var body any
		if req[0] == "PATCH" {
			body = map[string]string{"body": "pwned"}
		}
		if code := eve.do(req[0], req[1], body, nil); code != http.StatusNotFound {
			t.Errorf("%s by outsider = %d, want 404", name, code)
		}
	}
	if code := eve.do("POST", fmt.Sprintf("/api/chats/%d/messages", c.ID),
		map[string]string{"body": "hi", "client_id": uuid()}, nil); code != 404 {
		t.Errorf("send by outsider = %d, want 404", code)
	}
}

func TestChat_EditDeleteOnlyOwnMessages(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)
	m := alice.send(c.ID, "helo")
	path := fmt.Sprintf("/api/messages/%d", m.ID)

	if code := bob.do("PATCH", path, map[string]string{"body": "nope"}, nil); code != 403 {
		t.Errorf("bob edits alice's message = %d, want 403", code)
	}
	var edited msgResp
	if code := alice.do("PATCH", path, map[string]string{"body": "hello"}, &edited); code != 200 || edited.Body != "hello" || edited.EditedAt == nil {
		t.Errorf("edit = %d %+v", code, edited)
	}
	var deleted msgResp
	if code := alice.do("DELETE", path, nil, &deleted); code != 200 || deleted.Body != "" || deleted.DeletedAt == nil {
		t.Errorf("delete = %d %+v", code, deleted)
	}
	if code := alice.do("PATCH", path, map[string]string{"body": "back"}, nil); code != 409 {
		t.Errorf("edit deleted = %d, want 409", code)
	}
}

func TestChat_HistoryPagination(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)
	for i := 1; i <= 5; i++ {
		alice.send(c.ID, fmt.Sprint("m", i))
	}

	var h historyResp
	bob.do("GET", fmt.Sprintf("/api/chats/%d/messages?limit=2", c.ID), nil, &h)
	if len(h.Messages) != 2 || h.Messages[0].Seq != 4 || h.Messages[1].Seq != 5 || !h.HasMore {
		t.Fatalf("newest page = %+v", h)
	}
	bob.do("GET", fmt.Sprintf("/api/chats/%d/messages?before=4&limit=10", c.ID), nil, &h)
	if len(h.Messages) != 3 || h.Messages[0].Seq != 1 || h.HasMore {
		t.Fatalf("older page = %+v", h)
	}
	bob.do("GET", fmt.Sprintf("/api/chats/%d/messages?after=3", c.ID), nil, &h)
	if len(h.Messages) != 2 || h.Messages[0].Seq != 4 {
		t.Fatalf("catch-up page = %+v", h)
	}
	bob.do("GET", fmt.Sprintf("/api/chats/%d/messages?after=0&limit=2", c.ID), nil, &h)
	if len(h.Messages) != 2 || h.Messages[0].Seq != 1 || !h.HasMore {
		t.Fatalf("catch-up from an empty chat = %+v, want seq 1,2", h)
	}
	if code := bob.do("GET", fmt.Sprintf("/api/chats/%d/messages?limit=1000", c.ID), nil, nil); code != 400 {
		t.Errorf("limit 1000 = %d, want 400", code)
	}
}

func TestChat_GroupAdminRules(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	carol := newClient(t, srv).signup("carol")

	var g chatResp
	code := alice.do("POST", "/api/chats/group", map[string]any{"name": " Trip ", "member_ids": []int64{bob.UserID}}, &g)
	if code != 201 || g.Name != "Trip" || g.MyRole != "admin" || g.MemberCount != 2 {
		t.Fatalf("create group = %d %+v", code, g)
	}
	members := fmt.Sprintf("/api/chats/%d/members", g.ID)

	// A plain member can't add people, rename, or kick.
	if code := bob.do("POST", members, map[string]any{"user_ids": []int64{carol.UserID}}, nil); code != 403 {
		t.Errorf("member adds = %d, want 403", code)
	}
	if code := bob.do("PATCH", fmt.Sprintf("/api/chats/%d", g.ID), map[string]string{"name": "x"}, nil); code != 403 {
		t.Errorf("member renames = %d, want 403", code)
	}

	// The admin can.
	if code := alice.do("POST", members, map[string]any{"user_ids": []int64{carol.UserID}}, &g); code != 200 || g.MemberCount != 3 {
		t.Fatalf("admin adds = %d %+v", code, g)
	}
	if code := alice.do("PATCH", fmt.Sprintf("%s/%d", members, bob.UserID), map[string]string{"role": "admin"}, nil); code != 200 {
		t.Fatalf("promote = %d", code)
	}
	if code := bob.do("DELETE", fmt.Sprintf("%s/%d", members, carol.UserID), nil, nil); code != 204 {
		t.Fatalf("new admin kicks carol = %d", code)
	}
	if code := carol.do("GET", fmt.Sprintf("/api/chats/%d", g.ID), nil, nil); code != 404 {
		t.Errorf("kicked member still sees the chat: %d", code)
	}

	// Anyone can leave; the last admin can't demote themselves.
	if code := alice.do("DELETE", fmt.Sprintf("%s/%d", members, alice.UserID), nil, nil); code != 204 {
		t.Fatalf("leave = %d", code)
	}
	var e apiError
	if code := bob.do("PATCH", fmt.Sprintf("%s/%d", members, bob.UserID), map[string]string{"role": "member"}, &e); code != 400 || e.Error.Code != "last_admin" {
		t.Errorf("last admin demotes self = %d %s", code, e.Error.Code)
	}

	// Member management makes no sense on a direct chat.
	d := alice.direct(bob)
	if code := alice.do("POST", fmt.Sprintf("/api/chats/%d/members", d.ID), map[string]any{"user_ids": []int64{carol.UserID}}, nil); code != 400 {
		t.Errorf("add to direct chat = %d, want 400", code)
	}
}

func TestChat_Validation(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	bob := newClient(t, srv).signup("bob")
	c := alice.direct(bob)
	path := fmt.Sprintf("/api/chats/%d/messages", c.ID)

	cases := map[string]map[string]string{
		"blank body":      {"body": "   ", "client_id": uuid()},
		"bad client_id":   {"body": "hi", "client_id": "123"},
		"4001 characters": {"body": string(make([]rune, 4001)), "client_id": uuid()},
	}
	cases["4001 characters"]["body"] = fmt.Sprintf("%4001s", "x")
	for name, body := range cases {
		if code := alice.do("POST", path, body, nil); code != 400 {
			t.Errorf("%s = %d, want 400", name, code)
		}
	}
	if code := alice.do("POST", "/api/chats/direct", map[string]int64{"user_id": alice.UserID}, nil); code != 400 {
		t.Errorf("chat with self = %d, want 400", code)
	}
	if code := alice.do("POST", "/api/chats/direct", map[string]int64{"user_id": 999999}, nil); code != 400 {
		t.Errorf("chat with unknown user = %d, want 400", code)
	}
}
