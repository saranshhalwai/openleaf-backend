package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"openleaf-backend/internal/auth"
	"openleaf-backend/internal/collab"
	"openleaf-backend/internal/compilation"
	"openleaf-backend/internal/files"
	"openleaf-backend/internal/projects"
)

type API struct {
	auth        *auth.Authenticator
	projects    *projects.Service
	files       *files.Service
	compilation *compilation.Service
	collab      *collab.Handler
}

func New(a *auth.Authenticator, p *projects.Service, f *files.Service, c *compilation.Service, cl *collab.Handler) *API {
	return &API{
		auth:        a,
		projects:    p,
		files:       f,
		compilation: c,
		collab:      cl,
	}
}

// Router sets up middleware and routes
func (api *API) Router(allowedOrigins []string) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS configuration
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Internal-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Public health check
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Real-time collaborative editing WebSocket endpoint
	// Authenticates token parameter and checks project membership within handler
	r.Get("/api/projects/{projectId}/files/{fileId}/collaborate", api.collab.HandleWebSocket)

	// Webhook callback endpoint for compiler service (unauthenticated by Better-Auth)
	r.Post("/api/internal/compile/callback", api.handleCompileCallback)

	// Protected routes
	r.Route("/api", func(r chi.Router) {
		r.Use(api.auth.Middleware)

		// Current authenticated user info
		r.Get("/me", api.handleMe)

		// Project routes
		r.Route("/projects", func(r chi.Router) {
			r.Get("/", api.handleListProjects)
			r.Post("/", api.handleCreateProject)

			r.Route("/{projectId}", func(r chi.Router) {
				r.Use(api.projectMemberMiddleware)

				r.Get("/", api.handleGetProject)
				r.Delete("/", api.handleDeleteProject)

				// Compilation routes
				r.Route("/compile", func(r chi.Router) {
					r.Post("/", api.handleCompileProject)
					r.Get("/status", api.handleGetCompileStatus)
					r.Get("/pdf", api.handleGetCompilePDF)
					r.Get("/log", api.handleGetCompileLog)
				})

				// File routes within a project
				r.Route("/files", func(r chi.Router) {
					r.Get("/", api.handleListFiles)
					r.Post("/", api.handleCreateFile)

					r.Route("/{fileId}", func(r chi.Router) {
						r.Get("/", api.handleGetFile)
						r.Put("/", api.handleUpdateFile)
						r.Delete("/", api.handleDeleteFile)
					})
				})
			})
		})
	})

	return r
}

// Response helpers
func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
