package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"aether_ops/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(dbc config.DatabaseConfig) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = dbc.DSN()
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	if dbc.MaxConns > 0 {
		cfg.MaxConns = dbc.MaxConns
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return pool, nil
}
