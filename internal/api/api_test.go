package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"openleaf-backend/internal/api"
	"openleaf-backend/internal/auth"
	"openleaf-backend/internal/compilation"
	"openleaf-backend/internal/database"
	"openleaf-backend/internal/files"
	"openleaf-backend/internal/projects"
)

func TestAPIEndpoints(t *testing.T) {
	_ = godotenv.Load("../../.env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	db, err := database.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	// 1. Create a test user and a test session in Neon
	testUserID := "api-test-user-" + uuid.New().String()[:8]
	testSessionToken := "test-session-token-" + uuid.New().String()
	testSessionID := uuid.New().String()

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO "user" (id, name, email, email_verified, created_at, updated_at)
		VALUES ($1, 'API Test User', $1 || '@example.com', TRUE, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING;
	`, testUserID)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO session (id, user_id, token, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, NOW() + INTERVAL '1 hour', NOW(), NOW());
	`, testSessionID, testUserID, testSessionToken)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	defer func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM session WHERE id = $1`, testSessionID)
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, testUserID)
	}()

	authSvc := auth.NewAuthenticator(db.Pool)
	projSvc := projects.NewService(db.Pool)
	filesSvc := files.NewService(db.Pool)
	compSvc := compilation.NewService(context.Background(), db.Pool, filesSvc, compilation.Config{})
	defer compSvc.Close()
	server := api.New(authSvc, projSvc, filesSvc, compSvc)
	router := server.Router([]string{"http://localhost:5173"})

	// Test 1: Health check (public)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected health 200, got %d", w.Code)
	}

	// Test 2: Unauthorized request to protected endpoint
	req = httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized without token, got %d", w.Code)
	}

	// Test 3: Authenticated /api/me via Authorization header
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+testSessionToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for /api/me, got %d: %s", w.Code, w.Body.String())
	}

	// Test 4: Create Project via POST /api/projects
	body, _ := json.Marshal(map[string]string{"name": "Paper on AI"})
	req = httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testSessionToken)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for project, got %d: %s", w.Code, w.Body.String())
	}

	var createdProj struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		RootFileID *string `json:"rootFileId"`
	}
	_ = json.NewDecoder(w.Body).Decode(&createdProj)
	if createdProj.ID == "" || createdProj.RootFileID == nil {
		t.Fatalf("Expected project ID and RootFileID to be populated")
	}

	// Test 5: List files in project
	req = httptest.NewRequest(http.MethodGet, "/api/projects/"+createdProj.ID+"/files", nil)
	req.Header.Set("Authorization", "Bearer "+testSessionToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for listing files, got %d", w.Code)
	}

	// Test 6: Create new file in project
	fileBody, _ := json.Marshal(map[string]any{
		"path":     "references.bib",
		"content":  "@article{latex, title={Openleaf Article}}",
		"isBinary": false,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/projects/"+createdProj.ID+"/files", bytes.NewReader(fileBody))
	req.Header.Set("Authorization", "Bearer "+testSessionToken)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 for creating file, got %d: %s", w.Code, w.Body.String())
	}

	// Test 7: Clean up project
	req = httptest.NewRequest(http.MethodDelete, "/api/projects/"+createdProj.ID, nil)
	req.Header.Set("Authorization", "Bearer "+testSessionToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for deleting project, got %d", w.Code)
	}
}
