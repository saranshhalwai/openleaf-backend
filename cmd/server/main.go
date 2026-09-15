package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"openleaf-backend/internal/api"
	"openleaf-backend/internal/auth"
	"openleaf-backend/internal/compilation"
	"openleaf-backend/internal/database"
	"openleaf-backend/internal/files"
	"openleaf-backend/internal/projects"
)

func main() {
	// Load local .env if available
	_ = godotenv.Load(".env")

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	gcsBucket := os.Getenv("GCS_BUCKET")
	if gcsBucket == "" {
		gcsBucket = "openleaf-artifacts-openleaf-508614"
	}

	tasksQueue := os.Getenv("TASKS_QUEUE")
	if tasksQueue == "" {
		tasksQueue = "projects/openleaf-508614/locations/asia-southeast1/queues/latex-compiles"
	}

	compilerURL := os.Getenv("COMPILER_URL")
	if compilerURL == "" {
		compilerURL = "https://openleaf-compiler-17671319887.asia-southeast1.run.app"
	}

	backendURL := os.Getenv("BACKEND_URL")
	if backendURL == "" {
		backendURL = "https://openleaf-backend-17671319887.asia-southeast1.run.app"
	}

	internalSecret := os.Getenv("INTERNAL_SECRET")

	originsEnv := os.Getenv("ALLOWED_ORIGINS")
	var allowedOrigins []string
	if originsEnv != "" {
		for _, o := range strings.Split(originsEnv, ",") {
			trimmed := strings.TrimSpace(o)
			if trimmed != "" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	}
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:5173", "http://localhost:4173"}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to PostgreSQL database
	db, err := database.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	// Apply database schema migrations
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("Failed to run schema migrations: %v", err)
	}

	// Initialize core services
	authenticator := auth.NewAuthenticator(db.Pool)
	projectsService := projects.NewService(db.Pool)
	filesService := files.NewService(db.Pool)
	compilationService := compilation.NewService(ctx, db.Pool, filesService, compilation.Config{
		GCSBucket:      gcsBucket,
		TasksQueuePath: tasksQueue,
		CompilerURL:    compilerURL,
		BackendURL:     backendURL,
		InternalSecret: internalSecret,
	})
	defer compilationService.Close()

	// Build HTTP router
	apiServer := api.New(authenticator, projectsService, filesService, compilationService)
	router := apiServer.Router(allowedOrigins)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown handling
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Openleaf Go backend listening on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stopChan
	log.Println("Shutting down Openleaf backend gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	log.Println("Backend terminated cleanly.")
}
