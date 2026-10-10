package chat

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"relay/internal/users"
)

var (
	ErrForbidden = errors.New("not allowed")
)

// InvalidError lists bad input fields; RuleError is a broken rule ("you can't
// do that to a direct chat"). The HTTP handler turns both into 400s.
type InvalidError map[string]string

func (e InvalidError) Error() string { return "invalid input" }

type RuleError struct{ Code, Message string }

func (e *RuleError) Error() string { return e.Message }

// UserLookup is what chat needs from the users module: profiles, in batches.
type UserLookup interface {
	GetMany(ctx context.Context, ids []int64) (map[int64]users.Profile, error)
}

// Publisher pushes an event to users who are online. The realtime module
// implements it (Phase 3: Redis pub/sub will, without chat changing).
type Publisher interface {
	Publish(userIDs []int64, event any)
}

// Event is what clients receive over the WebSocket. See ADR 0003.
type Event struct {
	Type    string       `json:"type"`
	ChatID  int64        `json:"chat_id"`
	Message *MessageView `json:"message,omitempty"`
	Seq     int64        `json:"seq,omitempty"`
}

const (
	EventMessageCreated = "message.created"
	EventMessageUpdated = "message.updated"
	EventMessageDeleted = "message.deleted"
	EventChatCreated    = "chat.created"
	EventChatUpdated    = "chat.updated"
	EventChatRemoved    = "chat.removed"
	EventChatRead       = "chat.read"
)

type MessageView struct {
	Message
	Sender *users.Profile `json:"sender"` // nil if the account no longer exists
}

type MemberView struct {
	Member
	User users.Profile `json:"user"`
}

type ChatView struct {
	Chat
	MyRole      string         `json:"my_role"`
	LastReadSeq int64          `json:"last_read_seq"`
	Unread      int64          `json:"unread"`
	MemberCount int64          `json:"member_count"`
	OtherUser   *users.Profile `json:"other_user,omitempty"` // direct chats
	LastMessage *MessageView   `json:"last_message"`
	Members     []MemberView   `json:"members,omitempty"` // only on GET /api/chats/{id}
}

const (
	maxBodyChars = 4000
	maxGroupSize = 256
	MaxPageSize  = 100
)

type Service struct {
	store *Store
	users UserLookup
	pub   Publisher
}

func NewService(s *Store, u UserLookup, p Publisher) *Service {
	return &Service{store: s, users: u, pub: p}
}

// ---- reading ----

func (s *Service) List(ctx context.Context, me int64) ([]ChatView, error) {
	items, err := s.store.ListForUser(ctx, me)
	if err != nil {
		return nil, err
	}
	// Collect every user id on the page, then load them in ONE call.
	var ids []int64
	for _, it := range items {
		if it.OtherUserID != 0 {
			ids = append(ids, it.OtherUserID)
		}
		if it.LastMessage != nil {
			ids = append(ids, it.LastMessage.SenderID)
		}
	}
	profiles, err := s.users.GetMany(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]ChatView, 0, len(items))
	for _, it := range items {
		v := ChatView{Chat: it.Chat, MyRole: it.MyRole, LastReadSeq: it.LastReadSeq, Unread: it.Unread, MemberCount: it.MemberCount}
		if p, ok := profiles[it.OtherUserID]; ok {
			v.OtherUser = &p
		}
		if it.LastMessage != nil {
			v.LastMessage = view(*it.LastMessage, profiles)
		}
		out = append(out, v)
	}
	return out, nil
}

// Get returns one chat with its members. Non-members get ErrNotFound — we don't
// confirm that a chat they can't see exists.
func (s *Service) Get(ctx context.Context, me, chatID int64) (ChatView, error) {
	c, err := s.store.Get(ctx, chatID)
	if err != nil {
		return ChatView{}, err
	}
	members, err := s.store.Members(ctx, chatID)
	if err != nil {
		return ChatView{}, err
	}
	i := slices.IndexFunc(members, func(m Member) bool { return m.UserID == me })
	if i < 0 {
		return ChatView{}, ErrNotFound
	}
	mine := members[i]

	ids := make([]int64, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	var last *Message
	if c.LastSeq > 0 {
		page, err := s.store.History(ctx, chatID, Page{Limit: 1})
		if err != nil {
			return ChatView{}, err
		}
		if len(page.Messages) == 1 {
			last = &page.Messages[0]
			ids = append(ids, last.SenderID)
		}
	}
	profiles, err := s.users.GetMany(ctx, ids)
	if err != nil {
		return ChatView{}, err
	}

	v := ChatView{Chat: c, MyRole: mine.Role, LastReadSeq: mine.LastReadSeq, Unread: c.LastSeq - mine.LastReadSeq,
		MemberCount: int64(len(members))}
	for _, m := range members {
		v.Members = append(v.Members, MemberView{Member: m, User: profiles[m.UserID]})
		if c.Kind == "direct" && m.UserID != me {
			p := profiles[m.UserID]
			v.OtherUser = &p
		}
	}
	if last != nil {
		v.LastMessage = view(*last, profiles)
	}
	return v, nil
}

func (s *Service) History(ctx context.Context, me, chatID int64, p Page) ([]MessageView, bool, error) {
	if _, err := s.store.Member(ctx, chatID, me); err != nil {
		return nil, false, err
	}
	if p.Limit <= 0 || p.Limit > MaxPageSize {
		p.Limit = 50
	}
	page, err := s.store.History(ctx, chatID, p)
	if err != nil {
		return nil, false, err
	}
	views, err := s.views(ctx, page.Messages)
	return views, page.HasMore, err
}

// ---- chats & members ----

func (s *Service) StartDirect(ctx context.Context, me, other int64) (ChatView, bool, error) {
	if other == me {
		return ChatView{}, false, &RuleError{"self_chat", "you can't start a chat with yourself"}
	}
	if err := s.requireUsers(ctx, []int64{other}); err != nil {
		return ChatView{}, false, err
	}
	c, created, err := s.store.FindOrCreateDirect(ctx, me, other)
	if err != nil {
		return ChatView{}, false, err
	}
	if created {
		s.pub.Publish([]int64{me, other}, Event{Type: EventChatCreated, ChatID: c.ID})
	}
	v, err := s.Get(ctx, me, c.ID)
	return v, created, err
}

func (s *Service) CreateGroup(ctx context.Context, me int64, name string, memberIDs []int64) (ChatView, error) {
	name, msg := normalizeGroupName(name)
	if msg != "" {
		return ChatView{}, InvalidError{"name": msg}
	}
	ids := slices.DeleteFunc(uniq(memberIDs), func(id int64) bool { return id == me })
	if len(ids)+1 > maxGroupSize {
		return ChatView{}, InvalidError{"member_ids": "a group can have at most 256 members"}
	}
	if err := s.requireUsers(ctx, ids); err != nil {
		return ChatView{}, err
	}
	c, err := s.store.CreateGroup(ctx, me, name, ids)
	if err != nil {
		return ChatView{}, err
	}
	s.pub.Publish(append(ids, me), Event{Type: EventChatCreated, ChatID: c.ID})
	return s.Get(ctx, me, c.ID)
}

func (s *Service) Rename(ctx context.Context, me, chatID int64, name string) (ChatView, error) {
	if _, err := s.requireGroupAdmin(ctx, me, chatID); err != nil {
		return ChatView{}, err
	}
	name, msg := normalizeGroupName(name)
	if msg != "" {
		return ChatView{}, InvalidError{"name": msg}
	}
	if err := s.store.Rename(ctx, chatID, name); err != nil {
		return ChatView{}, err
	}
	s.publishToMembers(ctx, chatID, Event{Type: EventChatUpdated, ChatID: chatID})
	return s.Get(ctx, me, chatID)
}

func (s *Service) AddMembers(ctx context.Context, me, chatID int64, userIDs []int64) (ChatView, error) {
	if _, err := s.requireGroupAdmin(ctx, me, chatID); err != nil {
		return ChatView{}, err
	}
	ids := uniq(userIDs)
	if err := s.requireUsers(ctx, ids); err != nil {
		return ChatView{}, err
	}
	current, err := s.store.MemberIDs(ctx, chatID)
	if err != nil {
		return ChatView{}, err
	}
	if len(current)+len(ids) > maxGroupSize {
		return ChatView{}, InvalidError{"user_ids": "a group can have at most 256 members"}
	}
	added, err := s.store.AddMembers(ctx, chatID, ids)
	if err != nil {
		return ChatView{}, err
	}
	if len(added) > 0 {
		s.pub.Publish(added, Event{Type: EventChatCreated, ChatID: chatID}) // new to them
		s.pub.Publish(current, Event{Type: EventChatUpdated, ChatID: chatID})
	}
	return s.Get(ctx, me, chatID)
}

// RemoveMember kicks someone (admins only) or, when userID is me, leaves.
func (s *Service) RemoveMember(ctx context.Context, me, chatID, userID int64) error {
	c, err := s.requireMember(ctx, me, chatID)
	if err != nil {
		return err
	}
	if c.Kind != "group" {
		return &RuleError{"direct_chat", "direct chats have no member management"}
	}
	if userID != me {
		if _, err := s.requireGroupAdmin(ctx, me, chatID); err != nil {
			return err
		}
	}
	if err := s.store.RemoveMember(ctx, chatID, userID); err != nil {
		return err
	}
	s.pub.Publish([]int64{userID}, Event{Type: EventChatRemoved, ChatID: chatID})
	s.publishToMembers(ctx, chatID, Event{Type: EventChatUpdated, ChatID: chatID})
	return nil
}

func (s *Service) SetRole(ctx context.Context, me, chatID, userID int64, role string) (ChatView, error) {
	if role != "admin" && role != "member" {
		return ChatView{}, InvalidError{"role": `must be "admin" or "member"`}
	}
	if _, err := s.requireGroupAdmin(ctx, me, chatID); err != nil {
		return ChatView{}, err
	}
	if role == "member" {
		members, err := s.store.Members(ctx, chatID)
		if err != nil {
			return ChatView{}, err
		}
		admins := 0
		for _, m := range members {
			if m.Role == "admin" && m.UserID != userID {
				admins++
			}
		}
		if admins == 0 {
			return ChatView{}, &RuleError{"last_admin", "a group needs at least one admin"}
		}
	}
	if err := s.store.SetRole(ctx, chatID, userID, role); err != nil {
		return ChatView{}, err
	}
	s.publishToMembers(ctx, chatID, Event{Type: EventChatUpdated, ChatID: chatID})
	return s.Get(ctx, me, chatID)
}

// ---- messages ----

func (s *Service) Send(ctx context.Context, me, chatID int64, body, clientID string) (MessageView, bool, error) {
	body, msg := normalizeBody(body)
	fields := InvalidError{}
	if msg != "" {
		fields["body"] = msg
	}
	if !validUUID(clientID) {
		fields["client_id"] = "must be a UUID"
	}
	if len(fields) > 0 {
		return MessageView{}, false, fields
	}

	m, created, err := s.store.Send(ctx, chatID, me, body, clientID)
	if err != nil {
		return MessageView{}, false, err
	}
	if m.ChatID != chatID { // client_id reused across chats: a client bug
		return MessageView{}, false, &RuleError{"client_id_reused", "client_id was already used for another message"}
	}
	v, err := s.view(ctx, m)
	if err != nil {
		return MessageView{}, false, err
	}
	// Only after COMMIT (store.Send has returned): never announce a message
	// that could still roll back. A retry doesn't announce it twice.
	if created {
		s.publishToMembers(ctx, chatID, Event{Type: EventMessageCreated, ChatID: chatID, Message: &v})
	}
	return v, created, nil
}

func (s *Service) Edit(ctx context.Context, me, messageID int64, body string) (MessageView, error) {
	if _, err := s.ownMessage(ctx, me, messageID); err != nil {
		return MessageView{}, err
	}
	body, msg := normalizeBody(body)
	if msg != "" {
		return MessageView{}, InvalidError{"body": msg}
	}
	m, err := s.store.Edit(ctx, messageID, body)
	if err != nil {
		return MessageView{}, err
	}
	v, err := s.view(ctx, m)
	if err != nil {
		return MessageView{}, err
	}
	s.publishToMembers(ctx, m.ChatID, Event{Type: EventMessageUpdated, ChatID: m.ChatID, Message: &v})
	return v, nil
}

func (s *Service) Delete(ctx context.Context, me, messageID int64) (MessageView, error) {
	if _, err := s.ownMessage(ctx, me, messageID); err != nil {
		return MessageView{}, err
	}
	m, err := s.store.Delete(ctx, messageID)
	if err != nil {
		return MessageView{}, err
	}
	v, err := s.view(ctx, m)
	if err != nil {
		return MessageView{}, err
	}
	s.publishToMembers(ctx, m.ChatID, Event{Type: EventMessageDeleted, ChatID: m.ChatID, Message: &v})
	return v, nil
}

// MarkRead also tells my OTHER tabs, so their unread badges clear too.
func (s *Service) MarkRead(ctx context.Context, me, chatID, seq int64) (int64, error) {
	got, err := s.store.MarkRead(ctx, chatID, me, seq)
	if err != nil {
		return 0, err
	}
	s.pub.Publish([]int64{me}, Event{Type: EventChatRead, ChatID: chatID, Seq: got})
	return got, nil
}

// ---- helpers ----

func (s *Service) requireMember(ctx context.Context, me, chatID int64) (Chat, error) {
	if _, err := s.store.Member(ctx, chatID, me); err != nil {
		return Chat{}, err
	}
	return s.store.Get(ctx, chatID)
}

func (s *Service) requireGroupAdmin(ctx context.Context, me, chatID int64) (Chat, error) {
	m, err := s.store.Member(ctx, chatID, me)
	if err != nil {
		return Chat{}, err
	}
	c, err := s.store.Get(ctx, chatID)
	if err != nil {
		return Chat{}, err
	}
	if c.Kind != "group" {
		return Chat{}, &RuleError{"direct_chat", "direct chats have no member management"}
	}
	if m.Role != "admin" {
		return Chat{}, ErrForbidden
	}
	return c, nil
}

// ownMessage: members who aren't the sender get ErrForbidden; non-members ErrNotFound.
func (s *Service) ownMessage(ctx context.Context, me, messageID int64) (Message, error) {
	m, err := s.store.Message(ctx, messageID)
	if err != nil {
		return Message{}, err
	}
	if _, err := s.store.Member(ctx, m.ChatID, me); err != nil {
		return Message{}, err
	}
	if m.SenderID != me {
		return Message{}, ErrForbidden
	}
	return m, nil
}

func (s *Service) requireUsers(ctx context.Context, ids []int64) error {
	found, err := s.users.GetMany(ctx, ids)
	if err != nil {
		return err
	}
	if len(found) != len(uniq(ids)) {
		return &RuleError{"unknown_user", "one or more users don't exist"}
	}
	return nil
}

func (s *Service) publishToMembers(ctx context.Context, chatID int64, ev Event) {
	ids, err := s.store.MemberIDs(ctx, chatID)
	if err != nil {
		// The change is already committed; clients will catch up on reconnect.
		return
	}
	s.pub.Publish(ids, ev)
}

func (s *Service) view(ctx context.Context, m Message) (MessageView, error) {
	vs, err := s.views(ctx, []Message{m})
	if err != nil {
		return MessageView{}, err
	}
	return vs[0], nil
}

func (s *Service) views(ctx context.Context, msgs []Message) ([]MessageView, error) {
	ids := make([]int64, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.SenderID)
	}
	profiles, err := s.users.GetMany(ctx, uniq(ids))
	if err != nil {
		return nil, err
	}
	out := make([]MessageView, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, *view(m, profiles))
	}
	return out, nil
}

func view(m Message, profiles map[int64]users.Profile) *MessageView {
	v := &MessageView{Message: m}
	if p, ok := profiles[m.SenderID]; ok {
		v.Sender = &p
	}
	return v
}

func normalizeBody(s string) (string, string) {
	if strings.TrimSpace(s) == "" {
		return "", "can't be empty"
	}
	if utf8.RuneCountInString(s) > maxBodyChars {
		return "", "must be at most 4000 characters"
	}
	return s, ""
}

func normalizeGroupName(s string) (string, string) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n < 1 || n > 64 {
		return "", "must be 1–64 characters"
	}
	return s, ""
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if r != '-' {
				return false
			}
		case !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'):
			return false
		}
	}
	return true
}

func uniq(ids []int64) []int64 {
	set := map[int64]struct{}{}
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return slices.Sorted(maps.Keys(set))
}
