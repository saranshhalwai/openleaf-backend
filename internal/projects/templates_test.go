package projects_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"openleaf-backend/internal/database"
	"openleaf-backend/internal/files"
	"openleaf-backend/internal/projects"
)

func TestTemplatesListingAndCreation(t *testing.T) {
	// Test template catalog
	tpls := projects.ListTemplates()
	if len(tpls) < 4 {
		t.Fatalf("Expected at least 4 templates, found %d", len(tpls))
	}

	ieee := projects.GetTemplate("ieee")
	if ieee.ID != "ieee" || len(ieee.Files) != 2 {
		t.Errorf("Expected IEEE template with 2 files, got %d files", len(ieee.Files))
	}

	resume := projects.GetTemplate("resume")
	if resume.ID != "resume" {
		t.Errorf("Expected resume template, got %s", resume.ID)
	}

	beamer := projects.GetTemplate("beamer")
	if beamer.ID != "beamer" {
		t.Errorf("Expected beamer template, got %s", beamer.ID)
	}

	// Test fallback
	fallback := projects.GetTemplate("nonexistent-template")
	if fallback.ID != "article" {
		t.Errorf("Expected fallback to article template, got %s", fallback.ID)
	}

	// Test multi-file template creation in database if DATABASE_URL is available
	_ = godotenv.Load("../../.env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping DB template creation test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := database.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	testUserID := "tpl-user-" + uuid.New().String()[:8]
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO "user" (id, name, email, email_verified, created_at, updated_at)
		VALUES ($1, 'Tpl User', $2, TRUE, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING;
	`, testUserID, testUserID+"@test.com")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, testUserID)
	}()

	projSvc := projects.NewService(db.Pool)
	filesSvc := files.NewService(db.Pool)

	// Create project using IEEE template (has main.tex and references.bib)
	p, err := projSvc.CreateProject(ctx, testUserID, "IEEE Paper Test", "ieee")
	if err != nil {
		t.Fatalf("Failed to create IEEE project: %v", err)
	}
	defer func() {
		_ = projSvc.DeleteProject(context.Background(), p.ID, testUserID)
	}()

	createdFiles, err := filesSvc.ListFiles(ctx, p.ID)
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}
	if len(createdFiles) != 2 {
		t.Fatalf("Expected 2 files created for IEEE template, got %d", len(createdFiles))
	}

	var hasMain, hasBib bool
	for _, f := range createdFiles {
		if f.Path == "main.tex" {
			hasMain = true
		}
		if f.Path == "references.bib" {
			hasBib = true
		}
	}
	if !hasMain || !hasBib {
		t.Errorf("Expected main.tex and references.bib, got: %+v", createdFiles)
	}
}
