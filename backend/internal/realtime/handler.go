package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"relay/internal/httpx"
)

// Tickets is what the WebSocket endpoint needs from auth.
type Tickets interface {
	IssueTicket(userID int64) (string, error)
	ParseTicket(token string) (int64, error)
}

// Inbound handles the few things clients send over the socket. Everything
// that must not be lost (messages, reads, reactions) is a REST call instead;
// the socket only carries temporary events (ADR 0003).
type Inbound interface {
	Typing(ctx context.Context, userID, chatID int64) error
}

type Handler struct {
	hub       *Hub
	tickets   Tickets
	inbound   Inbound
	pingEvery time.Duration // how often we check the client is still there
	pongWait  time.Duration // how long it has to answer
	minGap    time.Duration // per connection: client frames closer together than this are dropped
}

func NewHandler(hub *Hub, t Tickets, in Inbound) *Handler {
	return &Handler{hub: hub, tickets: t, inbound: in,
		pingEvery: 25 * time.Second, pongWait: 10 * time.Second, minGap: 500 * time.Millisecond}
}

// clientFrame is the only shape clients may send: {"type":"typing","chat_id":7}.
type clientFrame struct {
	Type   string `json:"type"`
	ChatID int64  `json:"chat_id"`
}

// RegisterPublic adds GET /ws. It's outside the bearer-token middleware:
// it authenticates with the ticket in the URL instead.
func (h *Handler) RegisterPublic(mux *http.ServeMux) {
	mux.HandleFunc("GET /ws", h.serve)
}

// RegisterPrivate adds POST /api/ws/ticket to the authenticated mux.
func (h *Handler) RegisterPrivate(mux *http.ServeMux) {
	mux.Handle("POST /api/ws/ticket", httpx.Handle(h.ticket))
}

func (h *Handler) ticket(w http.ResponseWriter, r *http.Request) error {
	t, err := h.tickets.IssueTicket(httpx.UserID(r.Context()))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"ticket": t})
	return nil
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	userID, err := h.tickets.ParseTicket(r.URL.Query().Get("ticket"))
	if err != nil {
		httpx.WriteError(w, r, httpx.Unauthorized("invalid or expired ticket"))
		return
	}

	// The server's 15 s Read/WriteTimeout doesn't apply from here on: Accept
	// hijacks the connection, and net/http clears its deadlines on hijack
	// (TestRealtime_ConnectionOutlivesServerWriteTimeout keeps us honest).
	// Ping/pong is what detects dead connections instead.
	//
	// Accept checks Origin: a page on another site can't open a socket to us
	// with our user's cookies (cross-site WebSocket hijacking).
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept already wrote the HTTP error
	}
	defer conn.CloseNow()

	client := h.hub.Register(userID)
	defer h.hub.Unregister(client)

	// Clients send tiny frames (typing). A frame bigger than this is a bug or
	// an attack, and closes the connection.
	conn.SetReadLimit(1024)

	// The reader runs in its own goroutine: it handles client frames, and
	// (inside conn.Read) pongs and close frames. When the connection ends it
	// cancels ctx, which stops the writer loop below.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		defer cancel()
		h.read(ctx, conn, userID)
	}()

	ping := time.NewTicker(h.pingEvery)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-client.Kicked():
			code := websocket.StatusTryAgainLater
			if client.Reason() == ReasonShutdown {
				code = websocket.StatusGoingAway
			}
			conn.Close(code, client.Reason())
			return
		case b := <-client.send:
			if err := write(ctx, conn, b); err != nil {
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, h.pongWait)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				slog.Info("websocket ping failed, closing", "user_id", userID, "err", err)
				return
			}
		}
	}
}

// read handles frames from one client until the connection ends. Frames
// arriving faster than minGap are dropped: a typing indicator doesn't need
// more, and a misbehaving client can't make us query the database in a loop.
func (h *Handler) read(ctx context.Context, conn *websocket.Conn, userID int64) {
	var last time.Time
	for {
		typ, b, err := conn.Read(ctx)
		if err != nil {
			return // closed, or read limit exceeded
		}
		if typ != websocket.MessageText || time.Since(last) < h.minGap {
			continue
		}
		last = time.Now()
		var f clientFrame
		if json.Unmarshal(b, &f) != nil {
			continue
		}
		switch f.Type {
		case "typing":
			// Not a member → error, silently ignored: there's no one to tell.
			_ = h.inbound.Typing(ctx, userID, f.ChatID)
		}
	}
}

func write(ctx context.Context, conn *websocket.Conn, b []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, b)
}
