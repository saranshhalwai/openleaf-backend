package database

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// DB wraps the pgxpool.Pool
type DB struct {
	Pool *pgxpool.Pool
}

// Connect establishes a connection pool to the database
func Connect(ctx context.Context, databaseURL string) (*DB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database config: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 2
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Ping database
	pingCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	log.Println("Connected to PostgreSQL database")
	return &DB{Pool: pool}, nil
}

// Migrate executes the initialization schema
func (db *DB) Migrate(ctx context.Context) error {
	log.Println("Applying database migrations...")
	_, err := db.Pool.Exec(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	log.Println("Database migrations applied successfully")
	return nil
}

// Close closes the pool
func (db *DB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}
