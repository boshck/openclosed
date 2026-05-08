package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"openclosed/internal/guard"
	"openclosed/internal/store"
	"openclosed/internal/telegram"
)

func main() {
	if err := run(); err != nil {
		slog.Error("bot stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := loadConfig()

	client, err := telegram.NewClient(cfg.token, telegram.WithBaseURL(cfg.apiBase))
	if err != nil {
		return err
	}
	me, err := client.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("get bot identity: %w", err)
	}

	repo, err := store.Open(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer repo.Close()

	service := guard.NewService(client, repo, me.ID, logger)
	offset, ok, err := repo.LoadUpdateOffset(ctx)
	if err != nil {
		return err
	}
	if !ok {
		offset = 0
	}

	logger.Info("bot started", "bot_id", me.ID, "bot_username", me.Username)

	removalTicker := time.NewTicker(cfg.removalInterval)
	defer removalTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-removalTicker.C:
			if _, err := service.RunRemovalOnce(ctx, 100); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("removal worker failed", "error", err)
			}
		default:
		}

		updates, err := client.GetUpdates(ctx, offset, int(cfg.pollTimeout.Seconds()), []string{"message", "chat_member"})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			logger.Error("get updates failed", "error", err)
			sleepOrDone(ctx, 5*time.Second)
			continue
		}

		for _, update := range updates {
			if err := service.HandleUpdate(ctx, update); err != nil {
				logger.Error("handle update failed", "update_id", update.UpdateID, "error", err)
			}
			offset = update.UpdateID + 1
			if err := repo.SaveUpdateOffset(ctx, offset); err != nil {
				return err
			}
		}

		if _, err := service.RunRemovalOnce(ctx, 100); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("removal worker failed", "error", err)
		}
	}
}

type config struct {
	token           string
	databaseURL     string
	apiBase         string
	pollTimeout     time.Duration
	removalInterval time.Duration
}

func loadConfig() config {
	return config{
		token:           os.Getenv("TELEGRAM_BOT_TOKEN"),
		databaseURL:     envString("DATABASE_URL", os.Getenv("OPENCLOSED_DATABASE_URL")),
		apiBase:         envString("OPENCLOSED_API_BASE", "https://api.telegram.org"),
		pollTimeout:     time.Duration(envInt("OPENCLOSED_POLL_TIMEOUT_SECONDS", 30)) * time.Second,
		removalInterval: time.Duration(envInt("OPENCLOSED_REMOVAL_INTERVAL_SECONDS", 5)) * time.Second,
	}
}

func envString(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func sleepOrDone(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
