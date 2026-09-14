package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"openleaf-backend/internal/auth"
	"openleaf-backend/internal/projects"
)

type projectRoleKey struct{}

func (api *API) projectMemberMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID := chi.URLParam(r, "projectId")
		user, ok := auth.GetUser(r.Context())
		if !ok || user == nil {
			respondError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		role, err := api.projects.GetUserRole(r.Context(), projectID, user.ID)
		if err != nil {
			respondError(w, http.StatusNotFound, "project not found or permission denied")
			return
		}

		ctx := context.WithValue(r.Context(), projectRoleKey{}, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getProjectRole(ctx context.Context) projects.Role {
	if role, ok := ctx.Value(projectRoleKey{}).(projects.Role); ok {
		return role
	}
	return ""
}

func (api *API) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.GetUser(r.Context())
	respondJSON(w, http.StatusOK, user)
}

// Projects handlers
func (api *API) handleListProjects(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.GetUser(r.Context())
	list, err := api.projects.ListUserProjects(r.Context(), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, list)
}

type createProjectReq struct {
	Name string `json:"name"`
}

func (api *API) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.GetUser(r.Context())
	var req createProjectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	p, err := api.projects.CreateProject(r.Context(), user.ID, req.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, p)
}

func (api *API) handleGetProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	user, _ := auth.GetUser(r.Context())

	p, err := api.projects.GetProject(r.Context(), projectID, user.ID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, p)
}

func (api *API) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	user, _ := auth.GetUser(r.Context())

	if err := api.projects.DeleteProject(r.Context(), projectID, user.ID); err != nil {
		respondError(w, http.StatusForbidden, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "project deleted"})
}

// Files handlers
func (api *API) handleListFiles(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	fileList, err := api.files.ListFiles(r.Context(), projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, fileList)
}

type createFileReq struct {
	Path     string  `json:"path"`
	Content  *string `json:"content,omitempty"`
	IsBinary bool    `json:"isBinary"`
}

func (api *API) handleCreateFile(w http.ResponseWriter, r *http.Request) {
	role := getProjectRole(r.Context())
	if role == projects.RoleViewer {
		respondError(w, http.StatusForbidden, "read-only access")
		return
	}

	projectID := chi.URLParam(r, "projectId")
	var req createFileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	defaultContent := ""
	if req.Content == nil && !req.IsBinary {
		req.Content = &defaultContent
	}

	f, err := api.files.CreateFile(r.Context(), projectID, req.Path, req.IsBinary, req.Content, nil)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, f)
}

func (api *API) handleGetFile(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	fileID := chi.URLParam(r, "fileId")

	f, err := api.files.GetFile(r.Context(), projectID, fileID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, f)
}

type updateFileReq struct {
	Content string `json:"content"`
}

func (api *API) handleUpdateFile(w http.ResponseWriter, r *http.Request) {
	role := getProjectRole(r.Context())
	if role == projects.RoleViewer {
		respondError(w, http.StatusForbidden, "read-only access")
		return
	}

	projectID := chi.URLParam(r, "projectId")
	fileID := chi.URLParam(r, "fileId")

	var req updateFileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := api.files.UpdateFileContent(r.Context(), projectID, fileID, req.Content); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "file updated"})
}

func (api *API) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	role := getProjectRole(r.Context())
	if role == projects.RoleViewer {
		respondError(w, http.StatusForbidden, "read-only access")
		return
	}

	projectID := chi.URLParam(r, "projectId")
	fileID := chi.URLParam(r, "fileId")

	if err := api.files.DeleteFile(r.Context(), projectID, fileID); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "file deleted"})
}
