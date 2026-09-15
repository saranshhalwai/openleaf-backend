package compilation

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"openleaf-backend/internal/files"
)

type CompileJob struct {
	ID            string     `json:"id"`
	ProjectID     string     `json:"projectId"`
	UserID        *string    `json:"userId,omitempty"`
	VersionID     int        `json:"versionId"`
	Status        string     `json:"status"`
	PdfStorageKey *string    `json:"pdfStorageKey,omitempty"`
	LogStorageKey *string    `json:"logStorageKey,omitempty"`
	ErrorMessage  *string    `json:"errorMessage,omitempty"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type Config struct {
	GCSBucket      string
	TasksQueuePath string
	CompilerURL    string
	BackendURL     string
	InternalSecret string
}

type Service struct {
	db          *pgxpool.Pool
	files       *files.Service
	cfg         Config
	storageCl   *storage.Client
	tasksCl     *cloudtasks.Client
}

func NewService(ctx context.Context, db *pgxpool.Pool, filesService *files.Service, cfg Config) *Service {
	s := &Service{
		db:    db,
		files: filesService,
		cfg:   cfg,
	}

	stg, err := storage.NewClient(ctx)
	if err != nil {
		log.Printf("[Compilation] GCS client init warning: %v", err)
	} else {
		s.storageCl = stg
	}

	tasks, err := cloudtasks.NewClient(ctx)
	if err != nil {
		log.Printf("[Compilation] Cloud Tasks client init warning: %v", err)
	} else {
		s.tasksCl = tasks
	}

	return s
}

func (s *Service) Close() {
	if s.storageCl != nil {
		_ = s.storageCl.Close()
	}
	if s.tasksCl != nil {
		_ = s.tasksCl.Close()
	}
}

// CreateJob records a new compile_jobs entry in database.
func (s *Service) CreateJob(ctx context.Context, projectID, userID string) (*CompileJob, error) {
	jobID := uuid.New().String()
	query := `
		INSERT INTO compile_jobs (id, project_id, user_id, status)
		VALUES ($1, $2, $3, 'queued')
		RETURNING id, project_id, user_id, version_id, status, pdf_storage_key, log_storage_key, error_message, started_at, finished_at, created_at
	`

	job := &CompileJob{}
	err := s.db.QueryRow(ctx, query, jobID, projectID, userID).Scan(
		&job.ID,
		&job.ProjectID,
		&job.UserID,
		&job.VersionID,
		&job.Status,
		&job.PdfStorageKey,
		&job.LogStorageKey,
		&job.ErrorMessage,
		&job.StartedAt,
		&job.FinishedAt,
		&job.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create compile job: %w", err)
	}

	return job, nil
}

// ExportProjectTarGz packages all files for a project into a tar.gz byte buffer.
func (s *Service) ExportProjectTarGz(ctx context.Context, projectID string) ([]byte, error) {
	fileList, err := s.files.ListFilesWithContent(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list project files: %w", err)
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, f := range fileList {
		var data []byte
		if f.IsBinary {
			if f.StorageKey != nil && *f.StorageKey != "" && s.storageCl != nil {
				rc, err := s.storageCl.Bucket(s.cfg.GCSBucket).Object(*f.StorageKey).NewReader(ctx)
				if err == nil {
					data, _ = io.ReadAll(rc)
					_ = rc.Close()
				}
			}
		} else if f.Content != nil {
			data = []byte(*f.Content)
		}

		hdr := &tar.Header{
			Name:    f.Path,
			Mode:    0644,
			Size:    int64(len(data)),
			ModTime: f.UpdatedAt,
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("failed writing tar header for %s: %w", f.Path, err)
		}

		if _, err := tw.Write(data); err != nil {
			return nil, fmt.Errorf("failed writing tar content for %s: %w", f.Path, err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// UploadTarball uploads project tarball to GCS.
func (s *Service) UploadTarball(ctx context.Context, jobID string, tarball []byte) (string, error) {
	if s.storageCl == nil {
		return "", errors.New("GCS client not initialized")
	}

	key := fmt.Sprintf("tarballs/%s.tar.gz", jobID)
	wc := s.storageCl.Bucket(s.cfg.GCSBucket).Object(key).NewWriter(ctx)
	wc.ContentType = "application/gzip"

	if _, err := wc.Write(tarball); err != nil {
		_ = wc.Close()
		return "", fmt.Errorf("failed to write tarball to GCS: %w", err)
	}

	if err := wc.Close(); err != nil {
		return "", fmt.Errorf("failed to close GCS tarball writer: %w", err)
	}

	return key, nil
}

// DispatchCompileJob dispatches the job to Compiler service via Cloud Tasks (or direct HTTP fallback).
func (s *Service) DispatchCompileJob(ctx context.Context, job *CompileJob, tarballKey string, rootFile string) error {
	if rootFile == "" {
		rootFile = "main.tex"
	}

	callbackURL := fmt.Sprintf("%s/api/internal/compile/callback", s.cfg.BackendURL)

	payload := map[string]any{
		"job_id":         job.ID,
		"project_id":     job.ProjectID,
		"bucket":         s.cfg.GCSBucket,
		"tarball_key":    tarballKey,
		"root_file":      rootFile,
		"callback_url":   callbackURL,
		"callback_token": s.cfg.InternalSecret,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// 1. If Cloud Tasks is configured and available, dispatch via Cloud Tasks
	if s.tasksCl != nil && s.cfg.TasksQueuePath != "" {
		taskReq := &taskspb.CreateTaskRequest{
			Parent: s.cfg.TasksQueuePath,
			Task: &taskspb.Task{
				MessageType: &taskspb.Task_HttpRequest{
					HttpRequest: &taskspb.HttpRequest{
						HttpMethod: taskspb.HttpMethod_POST,
						Url:        fmt.Sprintf("%s/compile", s.cfg.CompilerURL),
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						Body: bodyBytes,
					},
				},
			},
		}

		_, err := s.tasksCl.CreateTask(ctx, taskReq)
		if err == nil {
			log.Printf("[Compilation] Enqueued task for job %s to %s", job.ID, s.cfg.TasksQueuePath)
			return nil
		}
		log.Printf("[Compilation] Cloud Tasks enqueue failed (%v), attempting direct dispatch fallback", err)
	}

	// 2. Direct HTTP dispatch fallback
	go func() {
		client := &http.Client{Timeout: 180 * time.Second}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("%s/compile", s.cfg.CompilerURL), bytes.NewReader(bodyBytes))
		if err != nil {
			log.Printf("[Compilation] Failed to create direct compiler request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[Compilation] Direct compiler request failed: %v", err)
			return
		}
		_ = resp.Body.Close()
	}()

	return nil
}

// UpdateJobStatus updates compile_jobs with result from compiler webhook.
func (s *Service) UpdateJobStatus(ctx context.Context, jobID string, status string, pdfKey, logKey, errMsg string) error {
	now := time.Now()
	query := `
		UPDATE compile_jobs
		SET status = $2,
		    pdf_storage_key = NULLIF($3, ''),
		    log_storage_key = NULLIF($4, ''),
		    error_message = NULLIF($5, ''),
		    finished_at = $6
		WHERE id = $1
	`
	_, err := s.db.Exec(ctx, query, jobID, status, pdfKey, logKey, errMsg, now)
	return err
}

// GetLatestJob returns the latest compile job for a project.
func (s *Service) GetLatestJob(ctx context.Context, projectID string) (*CompileJob, error) {
	query := `
		SELECT id, project_id, user_id, version_id, status, pdf_storage_key, log_storage_key, error_message, started_at, finished_at, created_at
		FROM compile_jobs
		WHERE project_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	job := &CompileJob{}
	err := s.db.QueryRow(ctx, query, projectID).Scan(
		&job.ID,
		&job.ProjectID,
		&job.UserID,
		&job.VersionID,
		&job.Status,
		&job.PdfStorageKey,
		&job.LogStorageKey,
		&job.ErrorMessage,
		&job.StartedAt,
		&job.FinishedAt,
		&job.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return job, nil
}

// StreamArtifact streams a file from GCS to the given writer.
func (s *Service) StreamArtifact(ctx context.Context, key string, w io.Writer) error {
	if s.storageCl == nil {
		return errors.New("GCS client not initialized")
	}

	rc, err := s.storageCl.Bucket(s.cfg.GCSBucket).Object(key).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to open GCS object: %w", err)
	}
	defer rc.Close()

	_, err = io.Copy(w, rc)
	return err
}
