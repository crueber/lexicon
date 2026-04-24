package ws

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/coder/websocket"
)

// Message is a WebSocket message sent to clients.
type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// Client represents a single WebSocket connection.
type Client struct {
	userID int64
	conn   *websocket.Conn
	hub    *Hub
	send   chan Message
}

// Hub manages WebSocket connections and broadcasts messages.
type Hub struct {
	mu      sync.RWMutex
	clients map[int64]map[*Client]struct{} // userID → set of clients
	logger  *slog.Logger
}

// NewHub creates a new Hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients: make(map[int64]map[*Client]struct{}),
		logger:  logger,
	}
}

// Register adds a client to the hub.
func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[client.userID] == nil {
		h.clients[client.userID] = make(map[*Client]struct{})
	}
	h.clients[client.userID][client] = struct{}{}

	h.logger.Debug("websocket client registered",
		"user_id", client.userID,
		"total_clients", len(h.clients[client.userID]),
	)
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.clients[client.userID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.clients, client.userID)
		}
	}

	h.logger.Debug("websocket client unregistered", "user_id", client.userID)
}

// BroadcastToUser sends a message to all connections for a specific user.
func (h *Hub) BroadcastToUser(userID int64, msg Message) {
	h.mu.RLock()
	clients := h.clients[userID]
	// Copy the set to avoid holding the lock while sending.
	targets := make([]*Client, 0, len(clients))
	for c := range clients {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.send <- msg:
		default:
			// Channel full — drop the message rather than blocking.
			h.logger.Warn("websocket send channel full, dropping message",
				"user_id", userID,
				"type", msg.Type,
			)
		}
	}
}

// BroadcastToAll sends a message to all connected clients.
func (h *Hub) BroadcastToAll(msg Message) {
	h.mu.RLock()
	// Collect all clients while holding the read lock.
	var targets []*Client
	for _, clients := range h.clients {
		for c := range clients {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.send <- msg:
		default:
			h.logger.Warn("websocket send channel full, dropping message",
				"user_id", c.userID,
				"type", msg.Type,
			)
		}
	}
}

// BroadcastBookUpdated broadcasts a BOOK_UPDATED message to all clients.
func (h *Hub) BroadcastBookUpdated(bookID int64) {
	h.BroadcastToAll(Message{Type: "BOOK_UPDATED", Payload: fmt.Sprintf("%d", bookID)})
}
