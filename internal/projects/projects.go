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

// CreateProject creates a project, associates the owner, and generates starter files from the selected template
func (s *Service) CreateProject(ctx context.Context, userID, name, templateID string) (*Project, error) {
	if name == "" {
		name = "Untitled Project"
	}

	tpl := GetTemplate(templateID)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	projectID := uuid.New().String()

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

	// 3. Create starter template files
	var rootFileID string
	fileQuery := `
		INSERT INTO files (id, project_id, path, is_binary, content)
		VALUES ($1, $2, $3, FALSE, $4);
	`
	for _, tf := range tpl.Files {
		newFileID := uuid.New().String()
		if _, err := tx.Exec(ctx, fileQuery, newFileID, projectID, tf.Path, tf.Content); err != nil {
			return nil, fmt.Errorf("failed to insert template file %s: %w", tf.Path, err)
		}
		if tf.Path == tpl.RootFile || rootFileID == "" {
			rootFileID = newFileID
		}
	}

	// 4. Update root_file_id
	if rootFileID != "" {
		updateRootQuery := `UPDATE projects SET root_file_id = $1 WHERE id = $2;`
		if _, err := tx.Exec(ctx, updateRootQuery, rootFileID, projectID); err != nil {
			return nil, fmt.Errorf("failed to set root file: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	p.RootFileID = &rootFileID
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

type ProjectMember struct {
	UserID    string    `json:"userId"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// ListProjectMembers returns all members of a project if the requesting user is a member
func (s *Service) ListProjectMembers(ctx context.Context, projectID, userID string) ([]ProjectMember, error) {
	_, err := s.GetUserRole(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT pm.user_id, u.email, u.name, pm.role, pm.created_at
		FROM project_members pm
		JOIN "user" u ON pm.user_id = u.id
		WHERE pm.project_id = $1
		ORDER BY 
			CASE pm.role 
				WHEN 'owner' THEN 1 
				WHEN 'editor' THEN 2 
				ELSE 3 
			END,
			pm.created_at ASC;
	`
	rows, err := s.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query project members: %w", err)
	}
	defer rows.Close()

	var members []ProjectMember
	for rows.Next() {
		var m ProjectMember
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan project member: %w", err)
		}
		members = append(members, m)
	}
	if members == nil {
		members = []ProjectMember{}
	}
	return members, nil
}

// AddProjectMemberByEmail adds or updates a user's role in a project (owner or editor only)
func (s *Service) AddProjectMemberByEmail(ctx context.Context, projectID, requesterUserID, targetEmail, role string) (*ProjectMember, error) {
	reqRole, err := s.GetUserRole(ctx, projectID, requesterUserID)
	if err != nil {
		return nil, err
	}
	if reqRole != RoleOwner && reqRole != RoleEditor {
		return nil, errors.New("only project owners and editors can add collaborators")
	}

	if role != string(RoleEditor) && role != string(RoleViewer) {
		role = string(RoleEditor)
	}

	// Look up target user in "user" table
	var targetID, targetName, email string
	findUserQuery := `SELECT id, name, email FROM "user" WHERE LOWER(email) = LOWER($1);`
	err = s.db.QueryRow(ctx, findUserQuery, targetEmail).Scan(&targetID, &targetName, &email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user with email %q not found. Please ask them to sign up for Openleaf first", targetEmail)
		}
		return nil, fmt.Errorf("failed to search user: %w", err)
	}

	// Cannot change owner role
	var currentRole string
	err = s.db.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2`, projectID, targetID).Scan(&currentRole)
	if err == nil && currentRole == string(RoleOwner) {
		return nil, errors.New("cannot change role of the project owner")
	}

	// Insert or update role
	upsertQuery := `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id, user_id)
		DO UPDATE SET role = EXCLUDED.role
		RETURNING created_at;
	`
	var createdAt time.Time
	err = s.db.QueryRow(ctx, upsertQuery, projectID, targetID, role).Scan(&createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to add collaborator: %w", err)
	}

	return &ProjectMember{
		UserID:    targetID,
		Email:     email,
		Name:      targetName,
		Role:      role,
		CreatedAt: createdAt,
	}, nil
}

// RemoveProjectMember removes a collaborator from the project (owner only)
func (s *Service) RemoveProjectMember(ctx context.Context, projectID, requesterUserID, targetUserID string) error {
	reqRole, err := s.GetUserRole(ctx, projectID, requesterUserID)
	if err != nil {
		return err
	}
	if reqRole != RoleOwner {
		return errors.New("only the project owner can remove collaborators")
	}

	if requesterUserID == targetUserID {
		return errors.New("cannot remove the owner from their own project")
	}

	res, err := s.db.Exec(ctx, "DELETE FROM project_members WHERE project_id = $1 AND user_id = $2", projectID, targetUserID)
	if err != nil {
		return fmt.Errorf("failed to remove collaborator: %w", err)
	}
	if res.RowsAffected() == 0 {
		return errors.New("member not found in project")
	}

	return nil
}
