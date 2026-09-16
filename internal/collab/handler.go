package collab

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"openleaf-backend/internal/auth"
	"openleaf-backend/internal/projects"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  64 * 1024,
	WriteBufferSize: 64 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow WebSocket upgrade from configured client origins
		return true
	},
}

type Handler struct {
	hub      *Hub
	auth     *auth.Authenticator
	projects *projects.Service
}

func NewHandler(hub *Hub, auth *auth.Authenticator, projects *projects.Service) *Handler {
	return &Handler{
		hub:      hub,
		auth:     auth,
		projects: projects,
	}
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	fileID := chi.URLParam(r, "fileId")

	if projectID == "" || fileID == "" {
		http.Error(w, `{"error":"projectId and fileId are required"}`, http.StatusBadRequest)
		return
	}

	// 1. Authenticate session token from query param or Authorization header
	token := r.URL.Query().Get("token")
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}
	}

	if token == "" {
		http.Error(w, `{"error":"unauthorized: session token required"}`, http.StatusUnauthorized)
		return
	}

	session, err := h.auth.ValidateSession(r.Context(), token)
	if err != nil || session == nil {
		http.Error(w, `{"error":"invalid or expired session token"}`, http.StatusUnauthorized)
		return
	}

	// 2. Check project membership role
	role, err := h.projects.GetUserRole(r.Context(), projectID, session.UserID)
	if err != nil {
		http.Error(w, `{"error":"project not found or permission denied"}`, http.StatusForbidden)
		return
	}

	if role == projects.RoleViewer {
		// Viewers cannot participate in collaborative editing (read-only)
		http.Error(w, `{"error":"viewers have read-only access"}`, http.StatusForbidden)
		return
	}

	// 3. Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Collab Upgrade Error] %v", err)
		return
	}

	// 4. Register client in room
	room := h.hub.GetOrCreateRoom(projectID, fileID)
	existingPeers := room.Count()
	client := NewClient(h.hub, room, conn, session.UserID, session.User.Name)
	client.SendInit(existingPeers)

	if err := room.AddClient(client); err != nil {
		_ = conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(4001, "Room is full (max 6 concurrent collaborators)"),
		)
		_ = conn.Close()
		return
	}

	// 5. Start read and write pumps
	go client.writePump()
	client.readPump()
}
