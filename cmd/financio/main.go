// Command financio is the Financio 2.0 Telegram expense-tracking bot.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rifi331/financio/internal/ai"
	"github.com/rifi331/financio/internal/auth"
	"github.com/rifi331/financio/internal/bot"
	"github.com/rifi331/financio/internal/config"
	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
	"github.com/rifi331/financio/internal/telegram"
	"github.com/rifi331/financio/migrations"
)

// Build info, injected via -ldflags "-X main.version=... etc".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// "healthcheck" subcommand: used by the Docker HEALTHCHECK — the distroless
	// image has no shell or wget, so the binary probes /healthz itself.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		port := os.Getenv("HEALTH_PORT")
		if port == "" {
			port = "7777"
		}
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://localhost:" + port + "/healthz")
		if err != nil {
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("financio exited with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(cfg.LogLevel),
	}))
	slog.SetDefault(logger)

	logger.Info("starting financio",
		"version", version, "commit", commit, "build_date", date,
		"config", cfg.String())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool, migrations.FS); err != nil {
		return err
	}

	tz, err := time.LoadLocation(cfg.DisplayTZ)
	if err != nil {
		return err
	}

	tg, err := telegram.New(cfg.TelegramBotToken)
	if err != nil {
		return err
	}
	aiClient := ai.NewClient("", cfg.OpenAIAPIKey, cfg.OpenAIModel)

	st := store.New(pool)
	authorizer := auth.New(st, cfg.AllowedUsers)
	dispatcher := bot.NewDispatcher(st, authorizer, aiClient, tg, tg, tz, cfg.DefaultCurrency, nil)

	// At most 4 messages processed concurrently, per-chat order preserved.
	pool4 := bot.NewPool(4)

	// Health endpoint.
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Addr: ":" + cfg.HealthPort, Handler: mux}

	errCh := make(chan error, 2)
	go func() {
		logger.Info("health server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	go func() {
		logger.Info("telegram polling started")
		err := tg.Run(ctx, func(u tgbotapi.Update) {
			if u.Message == nil {
				return
			}
			m := u.Message
			chatID, userID := m.Chat.ID, m.From.ID
			msg := botMessage(m)
			updateID := int64(u.UpdateID)
			first, last := m.From.FirstName, m.From.LastName
			pool4.Submit(chatID, func() {
				dispatcher.Handle(ctx, updateID, chatID, userID, first, last, msg)
			})
		})
		if err != nil {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, telegram.ErrConflict) {
			logger.Error("FATAL: "+err.Error(),
				"hint", "another replica is polling with this token; Financio must run as a single replica")
		}
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func botMessage(m *tgbotapi.Message) router.Message {
	msg := router.Message{
		Text:           m.Text,
		HasPhoto:       len(m.Photo) > 0,
		DocumentName:   m.Document.FileName,
		DocumentFileID: m.Document.FileID,
	}
	if len(m.Photo) > 0 {
		msg.PhotoFileID = m.Photo[len(m.Photo)-1].FileID // largest size last
	}
	if m.Caption != "" {
		msg.Text = m.Caption
	}
	return msg
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
