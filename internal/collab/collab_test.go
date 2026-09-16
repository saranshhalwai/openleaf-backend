package collab

import (
	"testing"
)

func TestHubAndRoomLifecycle(t *testing.T) {
	hub := NewHub()

	room1 := hub.GetOrCreateRoom("proj-1", "file-1")
	if room1 == nil {
		t.Fatal("Expected room to be created")
	}

	if hub.GetOrCreateRoom("proj-1", "file-1") != room1 {
		t.Errorf("Expected identical room instance for same project/file")
	}

	room2 := hub.GetOrCreateRoom("proj-1", "file-2")
	if room2 == room1 {
		t.Errorf("Expected different rooms for different files")
	}

	roomsCount, clientsCount := hub.Stats()
	if roomsCount != 2 || clientsCount != 0 {
		t.Errorf("Expected 2 rooms and 0 clients, got %d rooms, %d clients", roomsCount, clientsCount)
	}
}

func TestRoomClientLimit(t *testing.T) {
	hub := NewHub()
	room := hub.GetOrCreateRoom("proj-1", "file-1")

	var clients []*Client
	for i := 0; i < MaxClientsPerRoom; i++ {
		c := NewClient(hub, room, nil, "user-"+string(rune('A'+i)), "User")
		if err := room.AddClient(c); err != nil {
			t.Fatalf("Failed to add client %d: %v", i, err)
		}
		clients = append(clients, c)
	}

	// 7th client should be rejected
	cExtra := NewClient(hub, room, nil, "user-extra", "Extra User")
	if err := room.AddClient(cExtra); err == nil {
		t.Errorf("Expected 7th client to be rejected, but succeeded")
	}

	// Remove one client
	room.RemoveClient(clients[0])
	if room.Count() != MaxClientsPerRoom-1 {
		t.Errorf("Expected %d clients, got %d", MaxClientsPerRoom-1, room.Count())
	}

	// Now 7th client can join
	if err := room.AddClient(cExtra); err != nil {
		t.Errorf("Expected extra client to join after removal, got %v", err)
	}
}

func TestRoomBroadcast(t *testing.T) {
	hub := NewHub()
	room := hub.GetOrCreateRoom("proj-1", "file-1")

	c1 := NewClient(hub, room, nil, "user-1", "User 1")
	c2 := NewClient(hub, room, nil, "user-2", "User 2")
	c3 := NewClient(hub, room, nil, "user-3", "User 3")

	_ = room.AddClient(c1)
	_ = room.AddClient(c2)
	_ = room.AddClient(c3)

	payload := []byte("test-yjs-update-binary")
	room.Broadcast(c1, payload)

	// c2 and c3 should receive message; c1 should not
	select {
	case msg := <-c2.send:
		if string(msg) != string(payload) {
			t.Errorf("c2 received unexpected payload")
		}
	default:
		t.Errorf("c2 did not receive broadcast message")
	}

	select {
	case msg := <-c3.send:
		if string(msg) != string(payload) {
			t.Errorf("c3 received unexpected payload")
		}
	default:
		t.Errorf("c3 did not receive broadcast message")
	}

	select {
	case <-c1.send:
		t.Errorf("Sender c1 should not receive its own broadcast")
	default:
		// expected
	}
}
