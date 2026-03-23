package ws

import (
	"log/slog"
	"sync"
	"testing"
)

// newTestHub creates a Hub with a discard logger for testing.
func newTestHub(t *testing.T) *Hub {
	t.Helper()
	return NewHub(slog.Default())
}

// newTestClient creates a Client with the given userID for testing.
// The conn field is nil — tests must not call methods that use it.
func newTestClient(userID int64, hub *Hub) *Client {
	return &Client{
		userID: userID,
		conn:   nil,
		hub:    hub,
		send:   make(chan Message, sendChannelSize),
	}
}

func TestHub_BroadcastToUser_SendsToCorrectClients(t *testing.T) {
	hub := newTestHub(t)

	c1 := newTestClient(1, hub)
	c2 := newTestClient(1, hub)
	c3 := newTestClient(2, hub) // different user

	hub.Register(c1)
	hub.Register(c2)
	hub.Register(c3)

	msg := Message{Type: "TEST", Payload: "hello"}
	hub.BroadcastToUser(1, msg)

	// c1 and c2 should receive the message.
	select {
	case got := <-c1.send:
		if got.Type != msg.Type {
			t.Errorf("c1: got type %q; want %q", got.Type, msg.Type)
		}
	default:
		t.Error("c1: expected message, got none")
	}

	select {
	case got := <-c2.send:
		if got.Type != msg.Type {
			t.Errorf("c2: got type %q; want %q", got.Type, msg.Type)
		}
	default:
		t.Error("c2: expected message, got none")
	}

	// c3 should NOT receive the message.
	select {
	case got := <-c3.send:
		t.Errorf("c3: unexpected message received: %v", got)
	default:
		// correct — no message
	}
}

func TestHub_BroadcastToAll_SendsToAllClients(t *testing.T) {
	hub := newTestHub(t)

	c1 := newTestClient(1, hub)
	c2 := newTestClient(2, hub)
	c3 := newTestClient(3, hub)

	hub.Register(c1)
	hub.Register(c2)
	hub.Register(c3)

	msg := Message{Type: "GLOBAL", Payload: nil}
	hub.BroadcastToAll(msg)

	for i, c := range []*Client{c1, c2, c3} {
		select {
		case got := <-c.send:
			if got.Type != msg.Type {
				t.Errorf("client %d: got type %q; want %q", i+1, got.Type, msg.Type)
			}
		default:
			t.Errorf("client %d: expected message, got none", i+1)
		}
	}
}

func TestHub_Unregister_ClientDoesNotReceiveMessages(t *testing.T) {
	hub := newTestHub(t)

	c := newTestClient(1, hub)
	hub.Register(c)
	hub.Unregister(c)

	hub.BroadcastToUser(1, Message{Type: "TEST", Payload: nil})

	select {
	case got := <-c.send:
		t.Errorf("unregistered client received message: %v", got)
	default:
		// correct — no message
	}
}

func TestHub_ConcurrentRegisterUnregister(t *testing.T) {
	hub := newTestHub(t)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := range goroutines {
		go func(id int64) {
			defer wg.Done()
			c := newTestClient(id, hub)
			hub.Register(c)
			hub.BroadcastToUser(id, Message{Type: "TEST", Payload: nil})
			hub.Unregister(c)
		}(int64(i))

		go func() {
			defer wg.Done()
			hub.BroadcastToAll(Message{Type: "GLOBAL", Payload: nil})
		}()
	}

	wg.Wait()
}

func TestHub_MultipleClientsPerUser(t *testing.T) {
	hub := newTestHub(t)

	const clientCount = 5
	clients := make([]*Client, clientCount)
	for i := range clientCount {
		clients[i] = newTestClient(42, hub)
		hub.Register(clients[i])
	}

	msg := Message{Type: "MULTI", Payload: "data"}
	hub.BroadcastToUser(42, msg)

	for i, c := range clients {
		select {
		case got := <-c.send:
			if got.Type != msg.Type {
				t.Errorf("client %d: got type %q; want %q", i, got.Type, msg.Type)
			}
		default:
			t.Errorf("client %d: expected message, got none", i)
		}
	}
}

func TestHub_UnregisterLastClient_CleansUpUserEntry(t *testing.T) {
	hub := newTestHub(t)

	c := newTestClient(99, hub)
	hub.Register(c)
	hub.Unregister(c)

	hub.mu.RLock()
	_, exists := hub.clients[99]
	hub.mu.RUnlock()

	if exists {
		t.Error("user entry should be removed after last client unregisters")
	}
}
