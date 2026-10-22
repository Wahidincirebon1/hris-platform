package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"hris-platform/backend/internal/database"
	"hris-platform/backend/internal/httpapi"
	"hris-platform/backend/internal/security"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool); err != nil {
		log.Fatalf("apply database migrations: %v", err)
	}
	if len(os.Args) > 1 && os.Args[1] == "bootstrap-superadmin" {
		if err := bootstrapSuperadmin(ctx, pool); err != nil {
			log.Fatal(err)
		}
		return
	}

	api, err := httpapi.New(pool)
	if err != nil {
		log.Fatalf("configure API: %v", err)
	}

	server := &http.Server{
		Addr:              envOr("HTTP_ADDR", ":8081"),
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("HRIS API listening on %s", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server: %v", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown: %v", err)
		}
	}
}

func runHealthcheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := database.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	return pool.Ping(ctx)
}

func bootstrapSuperadmin(ctx context.Context, pool *pgxpool.Pool) error {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("BOOTSTRAP_SUPERADMIN_EMAIL")))
	password := os.Getenv("BOOTSTRAP_SUPERADMIN_PASSWORD")
	branchCode := strings.ToUpper(strings.TrimSpace(os.Getenv("BOOTSTRAP_BRANCH_CODE")))
	if email == "" || password == "" || branchCode == "" {
		return fmt.Errorf("BOOTSTRAP_SUPERADMIN_EMAIL, BOOTSTRAP_SUPERADMIN_PASSWORD, and BOOTSTRAP_BRANCH_CODE must be provided")
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return fmt.Errorf("bootstrap email is invalid")
	}
	if len([]byte(password)) < 16 || len([]byte(password)) > security.MaxPasswordBytes {
		return fmt.Errorf("bootstrap password must contain 16 to 72 bytes")
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin superadmin bootstrap: %w", err)
	}
	defer tx.Rollback(ctx)
	var existingSuperadmin bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE role = 'superadmin')").Scan(&existingSuperadmin); err != nil {
		return fmt.Errorf("check existing superadmin: %w", err)
	}
	if existingSuperadmin {
		return fmt.Errorf("a superadmin already exists; bootstrap can only run once")
	}
	var branchID uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT id FROM branches WHERE code = $1 AND active", branchCode).Scan(&branchID); err != nil {
		return fmt.Errorf("active bootstrap branch %q not found", branchCode)
	}
	var userID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role, branch_id, active, must_change_password)
		VALUES ($1, $2, 'superadmin', $3, TRUE, TRUE)
		RETURNING id
	`, email, hash, branchID).Scan(&userID); err != nil {
		return fmt.Errorf("create superadmin: %w", err)
	}
	employeeNumber := "SYS-" + strings.ToUpper(strings.ReplaceAll(userID.String()[:8], "-", ""))
	if _, err := tx.Exec(ctx, `
		INSERT INTO employees (user_id, employee_number, full_name, branch_id, employment_status)
		VALUES ($1, $2, 'Administrator Sistem', $3, 'active')
	`, userID, employeeNumber, branchID); err != nil {
		return fmt.Errorf("create superadmin profile: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, details)
		VALUES ($1, 'BOOTSTRAP_SUPERADMIN', 'user', $2, '{}'::jsonb)
	`, userID, userID.String()); err != nil {
		return fmt.Errorf("audit superadmin bootstrap: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit superadmin bootstrap: %w", err)
	}
	log.Printf("Initial superadmin created for %s; sign in and change the temporary password immediately.", email)
	return nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
