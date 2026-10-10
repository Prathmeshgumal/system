package chat_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"testing"

	"relay/internal/chat"
	"relay/internal/testdb"
	"relay/internal/users"
)

type fixture struct {
	t     *testing.T
	ctx   context.Context
	store *chat.Store
	users *users.Store
}

func setup(t *testing.T) *fixture {
	db := testdb.New(t)
	return &fixture{t: t, ctx: context.Background(), store: chat.NewStore(db), users: users.NewStore(db)}
}

func (f *fixture) user(name string) int64 {
	f.t.Helper()
	u, err := f.users.Create(f.ctx, users.NewUser{Email: name + "@x.io", Username: name, DisplayName: name, PasswordHash: "x"})
	if err != nil {
		f.t.Fatal(err)
	}
	return u.ID
}

func (f *fixture) send(chatID, sender int64, body string) chat.Message {
	f.t.Helper()
	m, _, err := f.store.Send(f.ctx, chatID, sender, body, newUUID(), nil)
	if err != nil {
		f.t.Fatalf("send %q: %v", body, err)
	}
	return m
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func TestDirect_FindOrCreateIsIdempotent(t *testing.T) {
	f := setup(t)
	alice, bob := f.user("alice"), f.user("bob")

	c1, created, err := f.store.FindOrCreateDirect(f.ctx, alice, bob)
	if err != nil || !created {
		t.Fatalf("first = %+v created=%v err=%v", c1, created, err)
	}
	// Same pair, other direction → same chat.
	c2, created, err := f.store.FindOrCreateDirect(f.ctx, bob, alice)
	if err != nil || created || c2.ID != c1.ID {
		t.Fatalf("second = %+v created=%v err=%v", c2, created, err)
	}
	ids, _ := f.store.MemberIDs(f.ctx, c1.ID)
	if len(ids) != 2 {
		t.Errorf("members = %v", ids)
	}
}

func TestDirect_ConcurrentCreatesMakeOneChat(t *testing.T) {
	f := setup(t)
	alice, bob := f.user("alice"), f.user("bob")

	var wg sync.WaitGroup
	ids := make([]int64, 10)
	for i := range ids {
		wg.Go(func() {
			c, _, err := f.store.FindOrCreateDirect(f.ctx, alice, bob)
			if err != nil {
				t.Error(err)
			}
			ids[i] = c.ID
		})
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("got different chats: %v", ids)
		}
	}
}

func TestSend_SeqIsConsecutiveUnderConcurrency(t *testing.T) {
	f := setup(t)
	alice, bob := f.user("alice"), f.user("bob")
	c, _, _ := f.store.FindOrCreateDirect(f.ctx, alice, bob)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			sender := alice
			if i%2 == 0 {
				sender = bob
			}
			if _, _, err := f.store.Send(f.ctx, c.ID, sender, fmt.Sprint("msg ", i), newUUID(), nil); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	page, err := f.store.History(f.ctx, c.ID, chat.Page{Before: 0, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 20 {
		t.Fatalf("got %d messages", len(page.Messages))
	}
	for i, m := range page.Messages {
		if m.Seq != int64(i+1) {
			t.Fatalf("message %d has seq %d — want 1…20 with no gaps", i, m.Seq)
		}
	}
}

func TestSend_RetryWithSameClientIDReturnsOriginal(t *testing.T) {
	f := setup(t)
	alice, bob := f.user("alice"), f.user("bob")
	c, _, _ := f.store.FindOrCreateDirect(f.ctx, alice, bob)
	cid := newUUID()

	m1, created1, err := f.store.Send(f.ctx, c.ID, alice, "hello", cid, nil)
	if err != nil || !created1 {
		t.Fatal(err)
	}
	m2, created2, err := f.store.Send(f.ctx, c.ID, alice, "hello", cid, nil)
	if err != nil || created2 || m2.ID != m1.ID {
		t.Fatalf("retry = %+v created=%v err=%v", m2, created2, err)
	}

	// Many simultaneous retries (a flaky network) still make one message.
	cid2 := newUUID()
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, _, err := f.store.Send(f.ctx, c.ID, alice, "again", cid2, nil); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	page, _ := f.store.History(f.ctx, c.ID, chat.Page{Limit: 100})
	if len(page.Messages) != 2 {
		t.Errorf("messages = %d, want 2", len(page.Messages))
	}
}

func TestSend_NonMemberGetsNotFound(t *testing.T) {
	f := setup(t)
	alice, bob, eve := f.user("alice"), f.user("bob"), f.user("eve")
	c, _, _ := f.store.FindOrCreateDirect(f.ctx, alice, bob)
	if _, _, err := f.store.Send(f.ctx, c.ID, eve, "hi", newUUID(), nil); !errors.Is(err, chat.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestHistory_BeforeAndAfterCursors(t *testing.T) {
	f := setup(t)
	alice, bob := f.user("alice"), f.user("bob")
	c, _, _ := f.store.FindOrCreateDirect(f.ctx, alice, bob)
	for i := 1; i <= 7; i++ {
		f.send(c.ID, alice, fmt.Sprint(i))
	}

	// Newest page first, each page oldest → newest.
	p1, _ := f.store.History(f.ctx, c.ID, chat.Page{Limit: 3})
	if seqs(p1) != "5,6,7" || !p1.HasMore {
		t.Fatalf("latest page = %s has_more=%v", seqs(p1), p1.HasMore)
	}
	p2, _ := f.store.History(f.ctx, c.ID, chat.Page{Before: 5, Limit: 3})
	if seqs(p2) != "2,3,4" || !p2.HasMore {
		t.Fatalf("before 5 = %s has_more=%v", seqs(p2), p2.HasMore)
	}
	p3, _ := f.store.History(f.ctx, c.ID, chat.Page{Before: 2, Limit: 3})
	if seqs(p3) != "1" || p3.HasMore {
		t.Fatalf("before 2 = %s has_more=%v", seqs(p3), p3.HasMore)
	}

	// Catch-up: everything after what I've seen, oldest first.
	a, _ := f.store.History(f.ctx, c.ID, chat.Page{After: 4, Forward: true, Limit: 2})
	if seqs(a) != "5,6" || !a.HasMore {
		t.Fatalf("after 4 = %s has_more=%v", seqs(a), a.HasMore)
	}
	a2, _ := f.store.History(f.ctx, c.ID, chat.Page{After: 6, Forward: true, Limit: 2})
	if seqs(a2) != "7" || a2.HasMore {
		t.Fatalf("after 6 = %s has_more=%v", seqs(a2), a2.HasMore)
	}

	// A client that had an empty chat catches up with after=0: it needs the
	// OLDEST messages first, not the newest page (which would leave a hole).
	a0, _ := f.store.History(f.ctx, c.ID, chat.Page{After: 0, Forward: true, Limit: 3})
	if seqs(a0) != "1,2,3" || !a0.HasMore {
		t.Fatalf("after 0 = %s has_more=%v", seqs(a0), a0.HasMore)
	}
}

func TestUnreadCounts(t *testing.T) {
	f := setup(t)
	alice, bob := f.user("alice"), f.user("bob")
	c, _, _ := f.store.FindOrCreateDirect(f.ctx, alice, bob)
	f.send(c.ID, alice, "1")
	f.send(c.ID, alice, "2")
	last := f.send(c.ID, alice, "3")

	unread := func(user int64) int64 {
		list, err := f.store.ListForUser(f.ctx, user)
		if err != nil || len(list) != 1 {
			t.Fatalf("list = %+v, %v", list, err)
		}
		return list[0].Unread
	}
	if got := unread(alice); got != 0 {
		t.Errorf("sender's unread = %d, want 0", got)
	}
	if got := unread(bob); got != 3 {
		t.Errorf("bob's unread = %d, want 3", got)
	}
	if _, err := f.store.MarkRead(f.ctx, c.ID, bob, last.Seq-1); err != nil {
		t.Fatal(err)
	}
	if got := unread(bob); got != 1 {
		t.Errorf("after reading up to seq 2, unread = %d, want 1", got)
	}
	// Read markers never go backwards, and can't go past the newest message.
	f.store.MarkRead(f.ctx, c.ID, bob, 1)
	f.store.MarkRead(f.ctx, c.ID, bob, 999)
	if got := unread(bob); got != 0 {
		t.Errorf("unread = %d, want 0", got)
	}
}

func TestListForUser_NewestActivityFirstWithLastMessage(t *testing.T) {
	f := setup(t)
	alice, bob, carol := f.user("alice"), f.user("bob"), f.user("carol")
	direct, _, _ := f.store.FindOrCreateDirect(f.ctx, alice, bob)
	group, _ := f.store.CreateGroup(f.ctx, alice, "trip", []int64{bob, carol})
	f.send(group.ID, bob, "first")
	f.send(direct.ID, bob, "newest")

	list, err := f.store.ListForUser(f.ctx, alice)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if list[0].ID != direct.ID || list[0].LastMessage == nil || list[0].LastMessage.Body != "newest" {
		t.Errorf("first item = %+v", list[0])
	}
	if list[0].OtherUserID != bob || list[1].MemberCount != 3 || list[1].MyRole != "admin" {
		t.Errorf("items = %+v / %+v", list[0], list[1])
	}
}

func TestGroupMembership(t *testing.T) {
	f := setup(t)
	alice, bob, carol, dave := f.user("alice"), f.user("bob"), f.user("carol"), f.user("dave")
	g, err := f.store.CreateGroup(f.ctx, alice, "team", []int64{bob, bob, alice}) // duplicates ignored
	if err != nil {
		t.Fatal(err)
	}
	if ids, _ := f.store.MemberIDs(f.ctx, g.ID); len(ids) != 2 {
		t.Fatalf("members = %v, want alice + bob", ids)
	}

	f.send(g.ID, alice, "before carol joined")
	added, err := f.store.AddMembers(f.ctx, g.ID, []int64{carol, bob})
	if err != nil || len(added) != 1 || added[0] != carol {
		t.Fatalf("added = %v, %v (bob was already in)", added, err)
	}
	// Joining doesn't make old messages unread.
	list, _ := f.store.ListForUser(f.ctx, carol)
	if list[0].Unread != 0 {
		t.Errorf("new member unread = %d, want 0", list[0].Unread)
	}

	// The only admin leaves → the longest-standing member is promoted.
	f.store.AddMembers(f.ctx, g.ID, []int64{dave})
	if err := f.store.RemoveMember(f.ctx, g.ID, alice); err != nil {
		t.Fatal(err)
	}
	m, err := f.store.Member(f.ctx, g.ID, bob)
	if err != nil || m.Role != "admin" {
		t.Errorf("bob = %+v, %v — want promoted to admin", m, err)
	}
	if _, err := f.store.Member(f.ctx, g.ID, alice); !errors.Is(err, chat.ErrNotFound) {
		t.Errorf("alice still a member: %v", err)
	}

	if err := f.store.SetRole(f.ctx, g.ID, carol, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Rename(f.ctx, g.ID, "new name"); err != nil {
		t.Fatal(err)
	}
	got, _ := f.store.Get(f.ctx, g.ID)
	if got.Name != "new name" {
		t.Errorf("name = %q", got.Name)
	}
}

func TestEditAndDelete(t *testing.T) {
	f := setup(t)
	alice, bob := f.user("alice"), f.user("bob")
	c, _, _ := f.store.FindOrCreateDirect(f.ctx, alice, bob)
	m := f.send(c.ID, alice, "helo")

	edited, err := f.store.Edit(f.ctx, m.ID, "hello")
	if err != nil || edited.Body != "hello" || edited.EditedAt == nil {
		t.Fatalf("edit = %+v, %v", edited, err)
	}
	deleted, err := f.store.Delete(f.ctx, m.ID)
	if err != nil || deleted.Body != "" || deleted.DeletedAt == nil {
		t.Fatalf("delete = %+v, %v", deleted, err)
	}
	if _, err := f.store.Edit(f.ctx, m.ID, "zombie"); !errors.Is(err, chat.ErrDeleted) {
		t.Errorf("edit after delete: err = %v", err)
	}
}

func seqs(p chat.MessagePage) string {
	out := ""
	for i, m := range p.Messages {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprint(m.Seq)
	}
	return out
}

func TestReactions_AddIsIdempotentAndGroupsByEmoji(t *testing.T) {
	f := setup(t)
	alice, bob := f.user("alice"), f.user("bob")
	c, _, _ := f.store.FindOrCreateDirect(f.ctx, alice, bob)
	m1, m2 := f.send(c.ID, alice, "one"), f.send(c.ID, alice, "two")

	for _, r := range []struct {
		msg  int64
		user int64
		e    string
	}{{m1.ID, bob, "👍"}, {m1.ID, alice, "👍"}, {m1.ID, bob, "👍"}, {m1.ID, bob, "😂"}, {m2.ID, bob, "❤️"}} {
		if err := f.store.AddReaction(f.ctx, r.msg, r.user, r.e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := f.store.ReactionsFor(f.ctx, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatal(err)
	}
	r1 := got[m1.ID]
	if len(r1) != 2 || r1[0].Emoji != "👍" || len(r1[0].UserIDs) != 2 || r1[1].Emoji != "😂" {
		t.Errorf("message 1 reactions = %+v (adding 👍 twice must count once, oldest emoji first)", r1)
	}
	if len(got[m2.ID]) != 1 {
		t.Errorf("message 2 reactions = %+v", got[m2.ID])
	}

	if err := f.store.RemoveReaction(f.ctx, m1.ID, bob, "👍"); err != nil {
		t.Fatal(err)
	}
	got, _ = f.store.ReactionsFor(f.ctx, []int64{m1.ID})
	if len(got[m1.ID]) != 2 || len(got[m1.ID][0].UserIDs) != 1 || got[m1.ID][0].UserIDs[0] != alice {
		t.Errorf("after removing bob's 👍: %+v", got[m1.ID])
	}
}

func TestReplies_StoredAndLoadable(t *testing.T) {
	f := setup(t)
	alice, bob := f.user("alice"), f.user("bob")
	c, _, _ := f.store.FindOrCreateDirect(f.ctx, alice, bob)
	orig := f.send(c.ID, alice, "lunch?")

	reply, _, err := f.store.Send(f.ctx, c.ID, bob, "yes!", newUUID(), &orig.ID)
	if err != nil || reply.ReplyToID == nil || *reply.ReplyToID != orig.ID {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	got, err := f.store.MessagesByIDs(f.ctx, []int64{orig.ID, 999999})
	if err != nil || len(got) != 1 || got[orig.ID].Body != "lunch?" {
		t.Fatalf("MessagesByIDs = %+v, %v", got, err)
	}
}
