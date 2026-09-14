package projects

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

type Project struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	OwnerID    string    `json:"ownerId"`
	RootFileID *string   `json:"rootFileId,omitempty"`
	Role       string    `json:"role,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

const defaultMainTeX = `\documentclass{article}
\usepackage[utf8]{inputenc}

\title{My Openleaf Document}
\author{Openleaf Collaborator}
\date{\today}

\begin{document}

\maketitle

\section{Introduction}
Welcome to your new Openleaf LaTeX project! Start typing here to write your document.

\end{document}
`

// ListUserProjects returns all projects where the user is an owner, editor, or viewer
func (s *Service) ListUserProjects(ctx context.Context, userID string) ([]Project, error) {
	query := `
		SELECT p.id, p.name, p.owner_id, p.root_file_id, pm.role, p.created_at, p.updated_at
		FROM projects p
		JOIN project_members pm ON p.id = pm.project_id
		WHERE pm.user_id = $1
		ORDER BY p.updated_at DESC;
	`

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.OwnerID, &p.RootFileID, &p.Role, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		projects = append(projects, p)
	}

	if projects == nil {
		projects = []Project{}
	}

	return projects, nil
}

// CreateProject creates a project, associates the owner, and generates a default main.tex file
func (s *Service) CreateProject(ctx context.Context, userID, name string) (*Project, error) {
	if name == "" {
		name = "Untitled Project"
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	projectID := uuid.New().String()
	fileID := uuid.New().String()

	// 1. Insert project
	var p Project
	projectQuery := `
		INSERT INTO projects (id, name, owner_id)
		VALUES ($1, $2, $3)
		RETURNING id, name, owner_id, created_at, updated_at;
	`
	err = tx.QueryRow(ctx, projectQuery, projectID, name, userID).Scan(
		&p.ID, &p.Name, &p.OwnerID, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert project: %w", err)
	}

	// 2. Insert owner membership
	memberQuery := `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1, $2, 'owner');
	`
	if _, err := tx.Exec(ctx, memberQuery, projectID, userID); err != nil {
		return nil, fmt.Errorf("failed to insert project member: %w", err)
	}

	// 3. Create default main.tex file
	fileQuery := `
		INSERT INTO files (id, project_id, path, is_binary, content)
		VALUES ($1, $2, 'main.tex', FALSE, $3);
	`
	if _, err := tx.Exec(ctx, fileQuery, fileID, projectID, defaultMainTeX); err != nil {
		return nil, fmt.Errorf("failed to insert main.tex: %w", err)
	}

	// 4. Update root_file_id
	updateRootQuery := `UPDATE projects SET root_file_id = $1 WHERE id = $2;`
	if _, err := tx.Exec(ctx, updateRootQuery, fileID, projectID); err != nil {
		return nil, fmt.Errorf("failed to set root file: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	p.RootFileID = &fileID
	p.Role = "owner"
	return &p, nil
}

// GetProject retrieves a project by ID if the user is a member
func (s *Service) GetProject(ctx context.Context, projectID, userID string) (*Project, error) {
	query := `
		SELECT p.id, p.name, p.owner_id, p.root_file_id, pm.role, p.created_at, p.updated_at
		FROM projects p
		JOIN project_members pm ON p.id = pm.project_id
		WHERE p.id = $1 AND pm.user_id = $2;
	`

	var p Project
	err := s.db.QueryRow(ctx, query, projectID, userID).Scan(
		&p.ID, &p.Name, &p.OwnerID, &p.RootFileID, &p.Role, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("project not found or permission denied")
		}
		return nil, err
	}

	return &p, nil
}

// DeleteProject deletes a project (owner only)
func (s *Service) DeleteProject(ctx context.Context, projectID, userID string) error {
	role, err := s.GetUserRole(ctx, projectID, userID)
	if err != nil {
		return err
	}
	if role != RoleOwner {
		return errors.New("only the project owner can delete this project")
	}

	res, err := s.db.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	if res.RowsAffected() == 0 {
		return errors.New("project not found")
	}

	return nil
}

// GetUserRole fetches the user's role in a given project
func (s *Service) GetUserRole(ctx context.Context, projectID, userID string) (Role, error) {
	query := `SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2;`
	var role string
	err := s.db.QueryRow(ctx, query, projectID, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("not a member of this project")
		}
		return "", err
	}
	return Role(role), nil
}
