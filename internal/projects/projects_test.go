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

func TestProjectAndFileLifecycle(t *testing.T) {
	_ = godotenv.Load("../../.env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := database.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// 1. Create a test user in the "user" table
	testUserID := "test-user-" + uuid.New().String()[:8]
	testUserEmail := testUserID + "@example.com"
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO "user" (id, name, email, email_verified, created_at, updated_at)
		VALUES ($1, 'Test User', $2, TRUE, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING;
	`, testUserID, testUserEmail)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, testUserID)
	}()

	projSvc := projects.NewService(db.Pool)
	filesSvc := files.NewService(db.Pool)

	// 2. Create Project
	p, err := projSvc.CreateProject(ctx, testUserID, "Test LaTeX Project", "article")
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	if p.Name != "Test LaTeX Project" {
		t.Errorf("Expected name 'Test LaTeX Project', got '%s'", p.Name)
	}
	if p.Role != "owner" {
		t.Errorf("Expected role 'owner', got '%s'", p.Role)
	}
	if p.RootFileID == nil {
		t.Fatalf("Expected RootFileID to be populated")
	}

	// 3. Verify main.tex exists
	mainFile, err := filesSvc.GetFile(ctx, p.ID, *p.RootFileID)
	if err != nil {
		t.Fatalf("Failed to get root file: %v", err)
	}
	if mainFile.Path != "main.tex" {
		t.Errorf("Expected root file path 'main.tex', got '%s'", mainFile.Path)
	}
	if mainFile.Content == nil || len(*mainFile.Content) == 0 {
		t.Errorf("Expected default main.tex content to not be empty")
	}

	// 4. Create an additional file
	introContent := "\\section{Introduction}\nHere is some test content."
	introFile, err := filesSvc.CreateFile(ctx, p.ID, "chapters/intro.tex", false, &introContent, nil)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	if introFile.Path != "chapters/intro.tex" {
		t.Errorf("Expected path 'chapters/intro.tex', got '%s'", introFile.Path)
	}

	// 5. Update content
	updatedContent := introContent + "\nExtra line."
	if err := filesSvc.UpdateFileContent(ctx, p.ID, introFile.ID, updatedContent); err != nil {
		t.Fatalf("Failed to update file content: %v", err)
	}

	fetchedIntro, err := filesSvc.GetFile(ctx, p.ID, introFile.ID)
	if err != nil {
		t.Fatalf("Failed to fetch updated file: %v", err)
	}
	if fetchedIntro.Content == nil || *fetchedIntro.Content != updatedContent {
		t.Errorf("File content not updated correctly")
	}

	// 6. List files
	allFiles, err := filesSvc.ListFiles(ctx, p.ID)
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}
	if len(allFiles) != 2 {
		t.Errorf("Expected 2 files, found %d", len(allFiles))
	}

	// 7. Verify cannot delete root main.tex file
	if err := filesSvc.DeleteFile(ctx, p.ID, *p.RootFileID); err == nil {
		t.Errorf("Expected error when deleting root file, got nil")
	}

	// 8. Delete intro file
	if err := filesSvc.DeleteFile(ctx, p.ID, introFile.ID); err != nil {
		t.Fatalf("Failed to delete intro file: %v", err)
	}

	// 9. List user projects
	userProjects, err := projSvc.ListUserProjects(ctx, testUserID)
	if err != nil {
		t.Fatalf("Failed to list user projects: %v", err)
	}
	if len(userProjects) == 0 {
		t.Errorf("Expected at least 1 project in list")
	}

	// 10. Delete project
	if err := projSvc.DeleteProject(ctx, p.ID, testUserID); err != nil {
		t.Fatalf("Failed to delete project: %v", err)
	}
}
