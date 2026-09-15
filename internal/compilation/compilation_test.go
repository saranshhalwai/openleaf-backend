package compilation_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"openleaf-backend/internal/compilation"
	"openleaf-backend/internal/database"
	"openleaf-backend/internal/files"
	"openleaf-backend/internal/projects"
)

func TestCompilationTarballAndJob(t *testing.T) {
	_ = godotenv.Load("../../.env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping integration test: DATABASE_URL not set")
	}

	ctx := context.Background()
	db, err := database.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	testUserID := "comp-user-" + uuid.New().String()[:8]
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO "user" (id, name, email, email_verified, created_at, updated_at)
		VALUES ($1, 'Comp Test User', $2, TRUE, NOW(), NOW())
	`, testUserID, testUserID+"@test.com")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	defer func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, testUserID)
	}()

	projSvc := projects.NewService(db.Pool)
	filesSvc := files.NewService(db.Pool)
	compSvc := compilation.NewService(ctx, db.Pool, filesSvc, compilation.Config{})
	defer compSvc.Close()

	// 1. Create a project
	proj, err := projSvc.CreateProject(ctx, testUserID, "LaTeX Test Project")
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	defer func() {
		_ = projSvc.DeleteProject(ctx, proj.ID, testUserID)
	}()

	// 2. Add an extra file (sections/intro.tex)
	introContent := "\\section{Introduction}\nThis is a test section."
	_, err = filesSvc.CreateFile(ctx, proj.ID, "sections/intro.tex", false, &introContent, nil)
	if err != nil {
		t.Fatalf("Failed to create sub-file: %v", err)
	}

	// 3. Test ExportProjectTarGz
	tarballBytes, err := compSvc.ExportProjectTarGz(ctx, proj.ID)
	if err != nil {
		t.Fatalf("Failed to export tarball: %v", err)
	}

	// Verify tarball contents
	gzr, err := gzip.NewReader(bytes.NewReader(tarballBytes))
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	foundFiles := make(map[string]string)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Tar read error: %v", err)
		}
		content, _ := io.ReadAll(tr)
		foundFiles[hdr.Name] = string(content)
	}

	if _, ok := foundFiles["main.tex"]; !ok {
		t.Errorf("main.tex missing from exported tarball")
	}
	if foundFiles["sections/intro.tex"] != introContent {
		t.Errorf("sections/intro.tex content mismatch. Expected %q, got %q", introContent, foundFiles["sections/intro.tex"])
	}

	// 4. Test CreateJob and UpdateJobStatus
	job, err := compSvc.CreateJob(ctx, proj.ID, testUserID)
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	if job.Status != "queued" {
		t.Errorf("Expected job status 'queued', got %s", job.Status)
	}

	err = compSvc.UpdateJobStatus(ctx, job.ID, "succeeded", "artifacts/pdf.pdf", "artifacts/log.log", "")
	if err != nil {
		t.Fatalf("UpdateJobStatus failed: %v", err)
	}

	latestJob, err := compSvc.GetLatestJob(ctx, proj.ID)
	if err != nil {
		t.Fatalf("GetLatestJob failed: %v", err)
	}
	if latestJob.Status != "succeeded" {
		t.Errorf("Expected latest job status 'succeeded', got %s", latestJob.Status)
	}
	if latestJob.PdfStorageKey == nil || *latestJob.PdfStorageKey != "artifacts/pdf.pdf" {
		t.Errorf("PdfStorageKey mismatch")
	}
}
