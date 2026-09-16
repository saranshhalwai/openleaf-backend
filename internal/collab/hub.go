package collab

import (
	"fmt"
	"log"
	"sync"
)

// Hub maintains the set of active rooms and handles room lifecycle.
type Hub struct {
	rooms map[string]*Room
	mu    sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]*Room),
	}
}

// GetOrCreateRoom returns an existing room for (projectID, fileID) or creates a new one.
func (h *Hub) GetOrCreateRoom(projectID, fileID string) *Room {
	roomID := fmt.Sprintf("%s:%s", projectID, fileID)

	h.mu.Lock()
	defer h.mu.Unlock()

	room, exists := h.rooms[roomID]
	if !exists {
		room = NewRoom(roomID, projectID, fileID, h)
		h.rooms[roomID] = room
		log.Printf("[Collab Hub] Created new room: %s", roomID)
	}

	return room
}

// CloseRoom deletes an empty room from the hub.
func (h *Hub) CloseRoom(roomID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, exists := h.rooms[roomID]
	if exists && room.Count() == 0 {
		delete(h.rooms, roomID)
		log.Printf("[Collab Hub] Closed inactive room: %s", roomID)
	}
}

// Stats returns summary statistics of active rooms and connections.
func (h *Hub) Stats() (roomsCount int, totalClients int) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	roomsCount = len(h.rooms)
	for _, room := range h.rooms {
		totalClients += room.Count()
	}
	return
}
