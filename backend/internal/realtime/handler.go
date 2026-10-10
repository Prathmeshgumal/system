package realtime

import (
	"context"
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

type Handler struct {
	hub       *Hub
	tickets   Tickets
	pingEvery time.Duration // how often we check the client is still there
	pongWait  time.Duration // how long it has to answer
}

func NewHandler(hub *Hub, t Tickets) *Handler {
	return &Handler{hub: hub, tickets: t, pingEvery: 25 * time.Second, pongWait: 10 * time.Second}
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

	// Clients never send us data messages (sending is REST). CloseRead keeps
	// reading in the background so pongs and close frames are handled, and
	// cancels ctx when the connection ends.
	ctx := conn.CloseRead(r.Context())

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

func write(ctx context.Context, conn *websocket.Conn, b []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, b)
}
