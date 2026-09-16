package collab

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 10 * 1024 * 1024 // 10 MB maximum Yjs message chunk
)

// Client represents a single connected collaborator in a room.
type Client struct {
	hub      *Hub
	room     *Room
	conn     *websocket.Conn
	send     chan []byte
	UserID   string
	UserName string
}

func NewClient(hub *Hub, room *Room, conn *websocket.Conn, userID, userName string) *Client {
	return &Client{
		hub:      hub,
		room:     room,
		conn:     conn,
		send:     make(chan []byte, 256),
		UserID:   userID,
		UserName: userName,
	}
}

// SendInit sends an initialization control message with the count of active peers in the room.
func (c *Client) SendInit(peerCount int) {
	if peerCount > 255 {
		peerCount = 255
	}
	msg := []byte{MessageTypeInit, byte(peerCount)}
	select {
	case c.send <- msg:
	default:
		log.Printf("[Collab Client %s] Unable to queue init message", c.UserID)
	}
}

// readPump pumps messages from the websocket connection to the room.
func (c *Client) readPump() {
	defer func() {
		c.room.RemoveClient(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[Collab Client %s] Read error: %v", c.UserID, err)
			}
			break
		}

		if messageType == websocket.BinaryMessage || messageType == websocket.TextMessage {
			c.room.Broadcast(c, message)
		}
	}
}

// writePump pumps messages from the send channel to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				return
			}

			// Add queued messages to the current websocket frame
			n := len(c.send)
			for i := 0; i < n; i++ {
				if _, err := w.Write(<-c.send); err != nil {
					return
				}
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
