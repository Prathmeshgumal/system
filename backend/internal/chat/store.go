// Package chat owns chats, members and messages. Rules about who may do what
// live in service.go; this file is only the SQL.
package chat

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")
	ErrDeleted  = errors.New("message was deleted")
)

type Chat struct {
	ID            int64      `json:"id"`
	Kind          string     `json:"kind"` // "direct" or "group"
	Name          string     `json:"name"`
	CreatedBy     int64      `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	LastSeq       int64      `json:"last_seq"`
	LastMessageAt *time.Time `json:"last_message_at"`
}

type Member struct {
	UserID      int64     `json:"user_id"`
	Role        string    `json:"role"` // "admin" or "member"
	JoinedAt    time.Time `json:"joined_at"`
	LastReadSeq int64     `json:"last_read_seq"`
}

type Message struct {
	ID        int64      `json:"id"`
	ChatID    int64      `json:"chat_id"`
	Seq       int64      `json:"seq"`
	SenderID  int64      `json:"sender_id"`
	Body      string     `json:"body"`
	ClientID  string     `json:"client_id"`
	CreatedAt time.Time  `json:"created_at"`
	EditedAt  *time.Time `json:"edited_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

// ListItem is one row of "my chats".
type ListItem struct {
	Chat
	MyRole      string
	LastReadSeq int64
	Unread      int64
	MemberCount int64
	OtherUserID int64 // direct chats: the other person
	LastMessage *Message
}

// Page asks for history. Before = 0 and After = 0 means "the newest messages".
type Page struct {
	Before, After int64
	Limit         int
}

// MessagePage is always ordered oldest → newest.
type MessagePage struct {
	Messages []Message
	HasMore  bool
}

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

const chatColumns = `c.id, c.kind, c.name, c.created_by, c.created_at, c.last_seq, c.last_message_at`

func scanChat(row pgx.Row) (Chat, error) {
	var c Chat
	err := row.Scan(&c.ID, &c.Kind, &c.Name, &c.CreatedBy, &c.CreatedAt, &c.LastSeq, &c.LastMessageAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Chat{}, ErrNotFound
	}
	return c, err
}

const messageColumns = `id, chat_id, seq, sender_id, body, client_id::text, created_at, edited_at, deleted_at`

func scanMessage(row pgx.Row) (Message, error) {
	var m Message
	err := row.Scan(&m.ID, &m.ChatID, &m.Seq, &m.SenderID, &m.Body, &m.ClientID, &m.CreatedAt, &m.EditedAt, &m.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	return m, err
}

// ---- chats ----

// FindOrCreateDirect returns the 1:1 chat between a and b, creating it if needed.
// direct_key is UNIQUE, so even two simultaneous calls end up with one chat.
func (s *Store) FindOrCreateDirect(ctx context.Context, a, b int64) (Chat, bool, error) {
	key := fmt.Sprintf("%d:%d", min(a, b), max(a, b))
	var c Chat
	created := false
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		c, err = scanChat(tx.QueryRow(ctx, `
			INSERT INTO chats AS c (kind, direct_key, created_by) VALUES ('direct', $1, $2)
			ON CONFLICT (direct_key) DO NOTHING
			RETURNING `+chatColumns, key, a))
		if errors.Is(err, ErrNotFound) { // conflict: it already exists
			c, err = scanChat(tx.QueryRow(ctx, `SELECT `+chatColumns+` FROM chats c WHERE direct_key = $1`, key))
			return err
		}
		if err != nil {
			return err
		}
		created = true
		_, err = tx.Exec(ctx, `
			INSERT INTO chat_members (chat_id, user_id) VALUES ($1, $2), ($1, $3)`, c.ID, a, b)
		return err
	})
	return c, created, err
}

// CreateGroup makes a group with creator as admin. Duplicate ids are ignored.
func (s *Store) CreateGroup(ctx context.Context, creator int64, name string, memberIDs []int64) (Chat, error) {
	var c Chat
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		c, err = scanChat(tx.QueryRow(ctx, `
			INSERT INTO chats AS c (kind, name, created_by) VALUES ('group', $1, $2)
			RETURNING `+chatColumns, name, creator))
		if err != nil {
			return err
		}
		others := slices.DeleteFunc(slices.Clone(memberIDs), func(id int64) bool { return id == creator })
		_, err = tx.Exec(ctx, `
			INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, 'admin');
		`, c.ID, creator)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO chat_members (chat_id, user_id)
			SELECT $1, id FROM unnest($2::bigint[]) AS id
			ON CONFLICT DO NOTHING`, c.ID, others)
		return err
	})
	return c, err
}

func (s *Store) Get(ctx context.Context, chatID int64) (Chat, error) {
	return scanChat(s.db.QueryRow(ctx, `SELECT `+chatColumns+` FROM chats c WHERE c.id = $1`, chatID))
}

func (s *Store) Rename(ctx context.Context, chatID int64, name string) error {
	_, err := s.db.Exec(ctx, `UPDATE chats SET name = $2 WHERE id = $1 AND kind = 'group'`, chatID, name)
	return err
}

// ListForUser returns every chat userID is in, most recent activity first,
// in ONE query: unread is arithmetic (last_seq − last_read_seq), and the last
// message is a single index lookup on (chat_id, seq) per chat.
func (s *Store) ListForUser(ctx context.Context, userID int64) ([]ListItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+chatColumns+`,
		       m.role, m.last_read_seq,
		       c.last_seq - m.last_read_seq AS unread,
		       (SELECT count(*) FROM chat_members x WHERE x.chat_id = c.id) AS member_count,
		       COALESCE((SELECT x.user_id FROM chat_members x
		                 WHERE c.kind = 'direct' AND x.chat_id = c.id AND x.user_id <> $1 LIMIT 1), 0),
		       lm.id, lm.seq, lm.sender_id, lm.body, lm.client_id::text, lm.created_at, lm.edited_at, lm.deleted_at
		FROM chat_members m
		JOIN chats c ON c.id = m.chat_id
		LEFT JOIN messages lm ON lm.chat_id = c.id AND lm.seq = c.last_seq
		WHERE m.user_id = $1
		ORDER BY COALESCE(c.last_message_at, c.created_at) DESC, c.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ListItem
	for rows.Next() {
		var it ListItem
		var lm struct {
			ID, Seq, SenderID          *int64
			Body, ClientID             *string
			CreatedAt, EditedAt, DelAt *time.Time
		}
		err := rows.Scan(&it.ID, &it.Kind, &it.Name, &it.CreatedBy, &it.CreatedAt, &it.LastSeq, &it.LastMessageAt,
			&it.MyRole, &it.LastReadSeq, &it.Unread, &it.MemberCount, &it.OtherUserID,
			&lm.ID, &lm.Seq, &lm.SenderID, &lm.Body, &lm.ClientID, &lm.CreatedAt, &lm.EditedAt, &lm.DelAt)
		if err != nil {
			return nil, err
		}
		if lm.ID != nil { // LEFT JOIN found a message
			it.LastMessage = &Message{ID: *lm.ID, ChatID: it.ID, Seq: *lm.Seq, SenderID: *lm.SenderID, Body: *lm.Body,
				ClientID: *lm.ClientID, CreatedAt: *lm.CreatedAt, EditedAt: lm.EditedAt, DeletedAt: lm.DelAt}
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ---- members ----

func (s *Store) Member(ctx context.Context, chatID, userID int64) (Member, error) {
	var m Member
	err := s.db.QueryRow(ctx, `
		SELECT user_id, role, joined_at, last_read_seq FROM chat_members
		WHERE chat_id = $1 AND user_id = $2`, chatID, userID).Scan(&m.UserID, &m.Role, &m.JoinedAt, &m.LastReadSeq)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	return m, err
}

func (s *Store) Members(ctx context.Context, chatID int64) ([]Member, error) {
	rows, err := s.db.Query(ctx, `
		SELECT user_id, role, joined_at, last_read_seq FROM chat_members
		WHERE chat_id = $1 ORDER BY joined_at, user_id`, chatID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Member])
}

func (s *Store) MemberIDs(ctx context.Context, chatID int64) ([]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT user_id FROM chat_members WHERE chat_id = $1`, chatID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// AddMembers adds users who aren't members yet and returns who was added.
// New members start "caught up": history is visible, but not unread.
func (s *Store) AddMembers(ctx context.Context, chatID int64, userIDs []int64) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
		INSERT INTO chat_members (chat_id, user_id, last_read_seq)
		SELECT c.id, u.id, c.last_seq FROM chats c, unnest($2::bigint[]) AS u(id)
		WHERE c.id = $1
		ON CONFLICT DO NOTHING
		RETURNING user_id`, chatID, userIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// RemoveMember removes a user (kicked or left). If that leaves a group with
// members but no admin, the longest-standing member becomes admin.
func (s *Store) RemoveMember(ctx context.Context, chatID, userID int64) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM chat_members WHERE chat_id = $1 AND user_id = $2`, chatID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, `
			UPDATE chat_members SET role = 'admin'
			WHERE chat_id = $1
			  AND NOT EXISTS (SELECT 1 FROM chat_members WHERE chat_id = $1 AND role = 'admin')
			  AND user_id = (SELECT user_id FROM chat_members WHERE chat_id = $1
			                 ORDER BY joined_at, user_id LIMIT 1)`, chatID)
		return err
	})
}

func (s *Store) SetRole(ctx context.Context, chatID, userID int64, role string) error {
	tag, err := s.db.Exec(ctx, `UPDATE chat_members SET role = $3 WHERE chat_id = $1 AND user_id = $2`,
		chatID, userID, role)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ---- messages ----

// Send stores a message and returns it. created is false when this client_id
// was already used — a retry — and the original message is returned instead.
//
// Ordering: bumping chats.last_seq takes a row lock on the chat, held until
// COMMIT. So sends to one chat run one at a time through that line and commit
// in seq order — which is what makes "give me everything after seq N" safe.
func (s *Store) Send(ctx context.Context, chatID, senderID int64, body, clientID string) (Message, bool, error) {
	if m, err := s.byClientID(ctx, senderID, clientID); err == nil {
		return m, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Message{}, false, err
	}

	var m Message
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var seq int64
		err := tx.QueryRow(ctx, `
			UPDATE chats SET last_seq = last_seq + 1, last_message_at = now()
			WHERE id = $1
			  AND EXISTS (SELECT 1 FROM chat_members WHERE chat_id = $1 AND user_id = $2)
			RETURNING last_seq`, chatID, senderID).Scan(&seq)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound // no such chat, or not a member
		}
		if err != nil {
			return err
		}
		m, err = scanMessage(tx.QueryRow(ctx, `
			INSERT INTO messages (chat_id, seq, sender_id, body, client_id)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING `+messageColumns, chatID, seq, senderID, body, clientID))
		if err != nil {
			return err
		}
		// My own message is never unread for me.
		_, err = tx.Exec(ctx, `UPDATE chat_members SET last_read_seq = $3 WHERE chat_id = $1 AND user_id = $2`,
			chatID, senderID, seq)
		return err
	})

	// Two retries raced past the check above; the UNIQUE constraint stopped the
	// second one (its transaction rolled back, so no seq was used up).
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "messages_sender_id_client_id_key" {
		m, err = s.byClientID(ctx, senderID, clientID)
		return m, false, err
	}
	return m, err == nil, err
}

func (s *Store) byClientID(ctx context.Context, senderID int64, clientID string) (Message, error) {
	return scanMessage(s.db.QueryRow(ctx, `
		SELECT `+messageColumns+` FROM messages WHERE sender_id = $1 AND client_id = $2`, senderID, clientID))
}

func (s *Store) Message(ctx context.Context, id int64) (Message, error) {
	return scanMessage(s.db.QueryRow(ctx, `SELECT `+messageColumns+` FROM messages WHERE id = $1`, id))
}

func (s *Store) Edit(ctx context.Context, id int64, body string) (Message, error) {
	m, err := scanMessage(s.db.QueryRow(ctx, `
		UPDATE messages SET body = $2, edited_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+messageColumns, id, body))
	if errors.Is(err, ErrNotFound) {
		if _, err2 := s.Message(ctx, id); err2 == nil {
			return Message{}, ErrDeleted
		}
	}
	return m, err
}

// Delete is a soft delete: the row stays (keeping seq gapless and giving other
// clients something to replace), the text is gone.
func (s *Store) Delete(ctx context.Context, id int64) (Message, error) {
	return scanMessage(s.db.QueryRow(ctx, `
		UPDATE messages SET body = '', deleted_at = COALESCE(deleted_at, now())
		WHERE id = $1
		RETURNING `+messageColumns, id))
}

// History returns one page of messages using a seq cursor — never OFFSET.
// We fetch one extra row to learn whether there's more without a COUNT.
func (s *Store) History(ctx context.Context, chatID int64, p Page) (MessagePage, error) {
	var rows pgx.Rows
	var err error
	if p.After > 0 {
		rows, err = s.db.Query(ctx, `
			SELECT `+messageColumns+` FROM messages
			WHERE chat_id = $1 AND seq > $2 ORDER BY seq ASC LIMIT $3`, chatID, p.After, p.Limit+1)
	} else {
		before := p.Before
		if before <= 0 {
			before = 1<<63 - 1
		}
		rows, err = s.db.Query(ctx, `
			SELECT `+messageColumns+` FROM messages
			WHERE chat_id = $1 AND seq < $2 ORDER BY seq DESC LIMIT $3`, chatID, before, p.Limit+1)
	}
	if err != nil {
		return MessagePage{}, err
	}
	msgs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Message, error) { return scanMessage(r) })
	if err != nil {
		return MessagePage{}, err
	}

	page := MessagePage{Messages: msgs, HasMore: len(msgs) > p.Limit}
	if page.HasMore {
		page.Messages = msgs[:p.Limit]
	}
	if p.After <= 0 {
		slices.Reverse(page.Messages) // fetched newest-first; pages are oldest-first
	}
	if page.Messages == nil {
		page.Messages = []Message{}
	}
	return page, nil
}

// MarkRead moves my read marker forward to seq (never backwards, never past
// the newest message) and returns where it ended up.
func (s *Store) MarkRead(ctx context.Context, chatID, userID, seq int64) (int64, error) {
	var got int64
	err := s.db.QueryRow(ctx, `
		UPDATE chat_members m SET last_read_seq = GREATEST(m.last_read_seq, LEAST($3, c.last_seq))
		FROM chats c
		WHERE m.chat_id = $1 AND m.user_id = $2 AND c.id = m.chat_id
		RETURNING m.last_read_seq`, chatID, userID, seq).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return got, err
}
