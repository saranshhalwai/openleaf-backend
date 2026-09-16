package collab

import (
	"errors"
	"log"
	"sync"
)

const MaxClientsPerRoom = 6

const (
	MessageTypeSync      = 0
	MessageTypeAwareness = 1
	MessageTypeInit      = 2
)

// Room represents an active collaborative editing session for a specific file.
type Room struct {
	ID            string
	ProjectID     string
	FileID        string
	clients       map[*Client]bool
	cachedUpdates [][]byte
	mu            sync.RWMutex
	hub           *Hub
}

func NewRoom(id, projectID, fileID string, hub *Hub) *Room {
	return &Room{
		ID:            id,
		ProjectID:     projectID,
		FileID:        fileID,
		clients:       make(map[*Client]bool),
		cachedUpdates: make([][]byte, 0, 16),
		hub:           hub,
	}
}

func (r *Room) AddClient(c *Client) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.clients) >= MaxClientsPerRoom {
		return errors.New("room is full (maximum 6 concurrent collaborators)")
	}

	r.clients[c] = true
	log.Printf("[Collab Room %s] User %s joined (total: %d)", r.ID, c.UserID, len(r.clients))

	// Replay all cached document updates to the newly joined client
	for _, update := range r.cachedUpdates {
		select {
		case c.send <- update:
		default:
			log.Printf("[Collab Room %s] Buffer full for user %s during replay", r.ID, c.UserID)
		}
	}

	return nil
}

func (r *Room) RemoveClient(c *Client) {
	r.mu.Lock()
	delete(r.clients, c)
	count := len(r.clients)
	r.mu.Unlock()

	log.Printf("[Collab Room %s] User %s left (remaining: %d)", r.ID, c.UserID, count)

	if count == 0 {
		r.hub.CloseRoom(r.ID)
	}
}

func (r *Room) Broadcast(sender *Client, message []byte) {
	// If message is a sync update or step2, cache it for future joining clients
	if len(message) > 1 && message[0] == MessageTypeSync && (message[1] == 1 || message[1] == 2) {
		r.mu.Lock()
		msgCopy := make([]byte, len(message))
		copy(msgCopy, message)
		// Cap cached updates to prevent unbound memory growth
		if len(r.cachedUpdates) < 5000 {
			r.cachedUpdates = append(r.cachedUpdates, msgCopy)
		}
		r.mu.Unlock()
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for client := range r.clients {
		if client != sender {
			select {
			case client.send <- message:
			default:
				log.Printf("[Collab Room %s] Client buffer full for user %s, dropping message", r.ID, client.UserID)
			}
		}
	}
}

func (r *Room) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}
