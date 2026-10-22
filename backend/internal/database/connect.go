package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context) (*pgxpool.Pool, error) {
	portValue := os.Getenv("DB_PORT")
	if portValue == "" {
		portValue = "5432"
	}
	port, err := strconv.Atoi(portValue)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("DB_PORT must be a valid TCP port")
	}

	config, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("create database configuration: %w", err)
	}
	config.ConnConfig.Host = os.Getenv("DB_HOST")
	config.ConnConfig.Port = uint16(port)
	config.ConnConfig.Database = os.Getenv("DB_NAME")
	config.ConnConfig.User = os.Getenv("DB_USER")
	config.ConnConfig.Password = os.Getenv("DB_PASSWORD")
	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute

	if config.ConnConfig.Host == "" || config.ConnConfig.Database == "" ||
		config.ConnConfig.User == "" || config.ConnConfig.Password == "" {
		return nil, fmt.Errorf("DB_HOST, DB_NAME, DB_USER, and DB_PASSWORD must be configured")
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
