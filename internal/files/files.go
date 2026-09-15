package files

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type File struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"projectId"`
	Path       string    `json:"path"`
	IsBinary   bool      `json:"isBinary"`
	Content    *string   `json:"content,omitempty"`
	StorageKey *string   `json:"storageKey,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

// CleanPath validates and sanitizes a relative file path
func CleanPath(p string) (string, error) {
	cleaned := path.Clean(strings.TrimSpace(p))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", errors.New("invalid file path")
	}
	return cleaned, nil
}

// ListFiles returns all files in a project without loading heavy contents
func (s *Service) ListFiles(ctx context.Context, projectID string) ([]File, error) {
	query := `
		SELECT id, project_id, path, is_binary, storage_key, created_at, updated_at
		FROM files
		WHERE project_id = $1
		ORDER BY path ASC;
	`

	rows, err := s.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	defer rows.Close()

	var fileList []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Path, &f.IsBinary, &f.StorageKey, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		fileList = append(fileList, f)
	}

	if fileList == nil {
		fileList = []File{}
	}

	return fileList, nil
}

// ListFilesWithContent returns all files in a project including text content (used for compilation tarball packaging).
func (s *Service) ListFilesWithContent(ctx context.Context, projectID string) ([]File, error) {
	query := `
		SELECT id, project_id, path, is_binary, content, storage_key, created_at, updated_at
		FROM files
		WHERE project_id = $1
		ORDER BY path ASC;
	`

	rows, err := s.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list files with content: %w", err)
	}
	defer rows.Close()

	var fileList []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Path, &f.IsBinary, &f.Content, &f.StorageKey, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		fileList = append(fileList, f)
	}

	if fileList == nil {
		fileList = []File{}
	}

	return fileList, nil
}

// GetFile retrieves a specific file including content
func (s *Service) GetFile(ctx context.Context, projectID, fileID string) (*File, error) {
	query := `
		SELECT id, project_id, path, is_binary, content, storage_key, created_at, updated_at
		FROM files
		WHERE id = $1 AND project_id = $2;
	`

	var f File
	err := s.db.QueryRow(ctx, query, fileID, projectID).Scan(
		&f.ID, &f.ProjectID, &f.Path, &f.IsBinary, &f.Content, &f.StorageKey, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("file not found")
		}
		return nil, err
	}

	return &f, nil
}

// CreateFile adds a new file to the project
func (s *Service) CreateFile(ctx context.Context, projectID, rawPath string, isBinary bool, content *string, storageKey *string) (*File, error) {
	filePath, err := CleanPath(rawPath)
	if err != nil {
		return nil, err
	}

	fileID := uuid.New().String()
	var f File
	query := `
		INSERT INTO files (id, project_id, path, is_binary, content, storage_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, project_id, path, is_binary, content, storage_key, created_at, updated_at;
	`

	err = s.db.QueryRow(ctx, query, fileID, projectID, filePath, isBinary, content, storageKey).Scan(
		&f.ID, &f.ProjectID, &f.Path, &f.IsBinary, &f.Content, &f.StorageKey, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "files_project_id_path_key") || strings.Contains(err.Error(), "duplicate key") {
			return nil, errors.New("a file with this path already exists")
		}
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	// Update project's updated_at
	_, _ = s.db.Exec(ctx, "UPDATE projects SET updated_at = NOW() WHERE id = $1", projectID)

	return &f, nil
}

// UpdateFileContent updates the plaintext content of a file
func (s *Service) UpdateFileContent(ctx context.Context, projectID, fileID, content string) error {
	query := `
		UPDATE files
		SET content = $1, updated_at = NOW()
		WHERE id = $2 AND project_id = $3 AND is_binary = FALSE;
	`

	res, err := s.db.Exec(ctx, query, content, fileID, projectID)
	if err != nil {
		return fmt.Errorf("failed to update file content: %w", err)
	}
	if res.RowsAffected() == 0 {
		return errors.New("file not found or is a binary file")
	}

	// Update project's updated_at
	_, _ = s.db.Exec(ctx, "UPDATE projects SET updated_at = NOW() WHERE id = $1", projectID)

	return nil
}

// DeleteFile removes a file, disallowing deletion of the root file
func (s *Service) DeleteFile(ctx context.Context, projectID, fileID string) error {
	// Check if this is the root file
	var rootFileID *string
	err := s.db.QueryRow(ctx, "SELECT root_file_id FROM projects WHERE id = $1", projectID).Scan(&rootFileID)
	if err != nil {
		return fmt.Errorf("failed to check root file: %w", err)
	}

	if rootFileID != nil && *rootFileID == fileID {
		return errors.New("cannot delete the root main.tex file")
	}

	res, err := s.db.Exec(ctx, "DELETE FROM files WHERE id = $1 AND project_id = $2", fileID, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	if res.RowsAffected() == 0 {
		return errors.New("file not found")
	}

	// Update project's updated_at
	_, _ = s.db.Exec(ctx, "UPDATE projects SET updated_at = NOW() WHERE id = $1", projectID)

	return nil
}
