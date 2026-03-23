package ws

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
)

const (
	// sendChannelSize is the buffer size for the client send channel.
	sendChannelSize = 32
	// writeTimeout is the maximum time to wait for a write to complete.
	writeTimeout = 10 * time.Second
)

// Handler handles WebSocket upgrade requests.
type Handler struct {
	hub    *Hub
	secret string
	logger *slog.Logger
}

// NewHandler creates a new WebSocket Handler.
func NewHandler(hub *Hub, secret string, logger *slog.Logger) *Handler {
	return &Handler{
		hub:    hub,
		secret: secret,
		logger: logger,
	}
}

// Routes registers the WebSocket endpoint on the given router.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/ws", h.ServeWS)
}

// ServeWS upgrades an HTTP connection to WebSocket and manages the client lifecycle.
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	// Extract token from query param or Authorization header.
	tokenString := r.URL.Query().Get("token")
	if tokenString == "" {
		authHeader := r.Header.Get("Authorization")
		tokenString, _ = strings.CutPrefix(authHeader, "Bearer ")
	}

	if tokenString == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	principal, err := auth.ValidateAccessToken(tokenString, h.secret)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Allow connections from any origin in dev; in production the
		// same-origin policy applies via the reverse proxy.
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.logger.Error("websocket upgrade failed", "error", err)
		return
	}

	client := &Client{
		userID: principal.UserID,
		conn:   conn,
		hub:    h.hub,
		send:   make(chan Message, sendChannelSize),
	}

	h.hub.Register(client)
	defer h.hub.Unregister(client)

	h.logger.Debug("websocket client connected", "user_id", principal.UserID)

	// Run the write goroutine.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	writeErrCh := make(chan error, 1)
	go func() {
		writeErrCh <- h.writePump(ctx, client)
	}()

	// Read loop: handle PING messages, detect disconnect.
	h.readPump(ctx, client, cancel)

	// Wait for write goroutine to finish.
	<-writeErrCh

	h.logger.Debug("websocket client disconnected", "user_id", principal.UserID)
}

// readPump reads messages from the WebSocket connection.
// It handles PING messages and cancels ctx on disconnect.
func (h *Handler) readPump(ctx context.Context, client *Client, cancel context.CancelFunc) {
	defer cancel()

	for {
		var msg struct {
			Type string `json:"type"`
		}
		err := wsjson.Read(ctx, client.conn, &msg)
		if err != nil {
			// Connection closed or context cancelled — normal exit.
			return
		}

		if msg.Type == "PING" {
			select {
			case client.send <- Message{Type: "PONG", Payload: nil}:
			default:
			}
		}
	}
}

// writePump reads from the client's send channel and writes to the WebSocket.
func (h *Handler) writePump(ctx context.Context, client *Client) error {
	for {
		select {
		case <-ctx.Done():
			// Close the connection gracefully.
			_ = client.conn.Close(websocket.StatusNormalClosure, "")
			return nil
		case msg, ok := <-client.send:
			if !ok {
				_ = client.conn.Close(websocket.StatusNormalClosure, "")
				return nil
			}

			writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := wsjson.Write(writeCtx, client.conn, msg)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}
