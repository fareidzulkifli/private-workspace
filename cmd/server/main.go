package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"private-workspace/internal/ai"
	"private-workspace/internal/auth"
	"private-workspace/internal/config"
	"private-workspace/internal/db"
	"private-workspace/internal/gitnote"
	"private-workspace/internal/holidays"
	"private-workspace/internal/security"
	"private-workspace/internal/server"
	"private-workspace/internal/uploads"
	"private-workspace/internal/web"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, db.Config{
		Path:          cfg.SQLitePath,
		MigrationsDir: cfg.MigrationsDir,
		AppEnv:        cfg.AppEnv,
	})
	if err != nil {
		logger.Fatalf("database error: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Printf("database close error: %v", err)
		}
	}()

	store := auth.NewStore(database, cfg.SessionTTL)
	if _, err := store.BootstrapAdmin(ctx, cfg.AdminEmail, cfg.AdminPasswordHash); err != nil {
		logger.Fatalf("admin bootstrap error: %v", err)
	}
	if err := store.DeleteExpired(ctx); err != nil {
		logger.Printf("expired session cleanup error: %v", err)
	}

	authService := auth.NewService(store, auth.Options{
		CookieName:     cfg.SessionCookie,
		CookieSecure:   cfg.CookieSecure,
		CSRFHeaderName: cfg.CSRFHeader,
		Limiter:        security.NewLoginLimiter(5, 15*time.Minute),
		Logger:         logger,
	})

	var r2Client *uploads.Client
	if cfg.R2AccessKeyID != "" || cfg.R2SecretAccessKey != "" || cfg.R2Endpoint != "" || cfg.R2BucketName != "" {
		r2Client, err = uploads.NewClient(uploads.Config{
			AccessKeyID:     cfg.R2AccessKeyID,
			SecretAccessKey: cfg.R2SecretAccessKey,
			Endpoint:        cfg.R2Endpoint,
			BucketName:      cfg.R2BucketName,
			Region:          cfg.R2Region,
		})
		if err != nil {
			logger.Printf("R2 disabled: %v", err)
		}
	}

	gitNoteClient := gitnote.NewGitHubClient(gitnote.GitHubConfig{
		Token:  cfg.GitHubPAT,
		Owner:  cfg.GitNoteOwner,
		Repo:   cfg.GitNoteRepo,
		Branch: cfg.GitHubBranch,
	})
	holidayClient := holidays.NewHTTPClient(cfg.MalaysiaState)
	aiClient := ai.NewXAIClient(cfg.GrokAPIKey)
	var r2Router server.R2Client
	if r2Client != nil {
		r2Router = r2Client
	}

	handler := server.NewRouter(server.Config{
		DB:           database,
		Auth:         authService,
		Logger:       logger,
		Web:          web.New(web.Options{}),
		R2:           r2Router,
		GitNote:      gitNoteClient,
		Holidays:     holidayClient,
		HolidayState: cfg.MalaysiaState,
		AI:           aiClient,
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Printf("server listening addr=%s env=%s", cfg.HTTPAddr, cfg.AppEnv)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Printf("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("server error: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Fatalf("graceful shutdown failed: %v", err)
	}
	logger.Printf("server stopped")
}
