package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"openleaf-backend/internal/auth"
	"openleaf-backend/internal/projects"
)

// handleCompileProject packages project files and triggers an asynchronous compile job.
func (api *API) handleCompileProject(w http.ResponseWriter, r *http.Request) {
	role := getProjectRole(r.Context())
	if role == projects.RoleViewer {
		respondError(w, http.StatusForbidden, "read-only members cannot trigger compilation")
		return
	}

	projectID := chi.URLParam(r, "projectId")
	user, _ := auth.GetUser(r.Context())

	// 1. Create compile_jobs record
	job, err := api.compilation.CreateJob(r.Context(), projectID, user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 2. Package project files into in-memory .tar.gz
	tarball, err := api.compilation.ExportProjectTarGz(r.Context(), projectID)
	if err != nil {
		_ = api.compilation.UpdateJobStatus(r.Context(), job.ID, "failed", "", "", "failed to package project: "+err.Error())
		respondError(w, http.StatusInternalServerError, "failed to package project source: "+err.Error())
		return
	}

	// 3. Upload tarball to GCS
	tarballKey, err := api.compilation.UploadTarball(r.Context(), job.ID, tarball)
	if err != nil {
		_ = api.compilation.UpdateJobStatus(r.Context(), job.ID, "failed", "", "", "failed to upload tarball: "+err.Error())
		respondError(w, http.StatusInternalServerError, "failed to upload source tarball: "+err.Error())
		return
	}

	// 4. Resolve root file (defaults to main.tex)
	rootFile := "main.tex"
	proj, err := api.projects.GetProject(r.Context(), projectID, user.ID)
	if err == nil && proj.RootFileID != nil && *proj.RootFileID != "" {
		rf, err := api.files.GetFile(r.Context(), projectID, *proj.RootFileID)
		if err == nil && rf.Path != "" {
			rootFile = rf.Path
		}
	}

	// 5. Dispatch task to Compiler service
	if err := api.compilation.DispatchCompileJob(r.Context(), job, tarballKey, rootFile); err != nil {
		_ = api.compilation.UpdateJobStatus(r.Context(), job.ID, "failed", "", "", "failed to dispatch compile task: "+err.Error())
		respondError(w, http.StatusInternalServerError, "failed to dispatch compile task: "+err.Error())
		return
	}

	respondJSON(w, http.StatusAccepted, job)
}

// handleGetCompileStatus returns the latest compile job status for a project.
func (api *API) handleGetCompileStatus(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	job, err := api.compilation.GetLatestJob(r.Context(), projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if job == nil {
		respondJSON(w, http.StatusOK, map[string]any{"status": "none"})
		return
	}

	respondJSON(w, http.StatusOK, job)
}

// handleGetCompilePDF streams the latest compiled PDF to the browser.
func (api *API) handleGetCompilePDF(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	job, err := api.compilation.GetLatestJob(r.Context(), projectID)
	if err != nil || job == nil || job.Status != "succeeded" || job.PdfStorageKey == nil {
		respondError(w, http.StatusNotFound, "no compiled PDF found")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\"document.pdf\"")

	if err := api.compilation.StreamArtifact(r.Context(), *job.PdfStorageKey, w); err != nil {
		log.Printf("[Compile PDF] Error streaming PDF: %v", err)
	}
}

// handleGetCompileLog streams the latest compilation log to the client.
func (api *API) handleGetCompileLog(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	job, err := api.compilation.GetLatestJob(r.Context(), projectID)
	if err != nil || job == nil || job.LogStorageKey == nil {
		respondError(w, http.StatusNotFound, "no compilation log found")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := api.compilation.StreamArtifact(r.Context(), *job.LogStorageKey, w); err != nil {
		log.Printf("[Compile Log] Error streaming log: %v", err)
	}
}

type compileCallbackReq struct {
	JobID        string `json:"job_id"`
	ProjectID    string `json:"project_id"`
	Status       string `json:"status"`
	PdfKey       string `json:"pdf_key"`
	LogKey       string `json:"log_key"`
	ErrorMessage string `json:"error_message"`
}

// handleCompileCallback receives the async webhook completion signal from the compiler.
func (api *API) handleCompileCallback(w http.ResponseWriter, r *http.Request) {
	var req compileCallbackReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid callback payload")
		return
	}

	if req.JobID == "" || req.Status == "" {
		respondError(w, http.StatusBadRequest, "job_id and status are required")
		return
	}

	if err := api.compilation.UpdateJobStatus(r.Context(), req.JobID, req.Status, req.PdfKey, req.LogKey, req.ErrorMessage); err != nil {
		log.Printf("[Callback Error] Failed updating job status: %v", err)
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("[Callback] Successfully updated job %s status to %s", req.JobID, req.Status)
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
