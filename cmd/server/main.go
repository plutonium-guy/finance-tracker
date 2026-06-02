// Command server runs the Finance Tracker as a single self-contained binary:
// it migrates the database, seeds defaults, and serves the server-rendered UI.
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"finance-tracker/internal/gmailsync"
	"finance-tracker/internal/service"
	"finance-tracker/internal/store"
	"finance-tracker/internal/telegrambot"
	"finance-tracker/internal/web"
)

func main() {
	// `-health` performs an HTTP self-check against the running server and exits.
	// Used by the container HEALTHCHECK (distroless has no shell/curl).
	healthFlag := flag.Bool("health", false, "probe the running server's /healthz and exit 0/1")
	flag.Parse()

	_ = godotenv.Load() // .env optional (dev convenience)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	port := envOr("PORT", "8080")

	if *healthFlag {
		os.Exit(healthProbe(port))
	}
	dbPath := envOr("DB_PATH", "./finance.db")
	seedDemo := os.Getenv("SEED_DEMO") == "true"

	db, err := store.OpenSQLite(dbPath)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	for _, step := range []struct {
		name string
		fn   func() error
	}{
		{"migrate", db.Migrate},
		{"ensure settings", db.EnsureSettings},
		{"seed categories", db.SeedDefaultCategories},
	} {
		if err := step.fn(); err != nil {
			slog.Error("startup", "step", step.name, "err", err)
			os.Exit(1)
		}
	}

	svc := service.New(db, time.Now)
	if seedDemo {
		if err := svc.DemoSeed(); err != nil {
			slog.Error("demo seed", "err", err)
			os.Exit(1)
		}
		slog.Info("demo data seeded (SEED_DEMO=true)")
	}

	// Optionally post this month's due recurring items on startup.
	if os.Getenv("RECURRING_AUTOFIRE") == "true" {
		if n, err := svc.PostDue(svc.CurrentMonth()); err != nil {
			slog.Error("recurring auto-fire", "err", err)
		} else if n > 0 {
			slog.Info("recurring auto-fire", "posted", n, "month", svc.CurrentMonth())
		}
	}

	// Optional Telegram bot: add transactions by messaging the bot.
	botCtx, stopBot := context.WithCancel(context.Background())
	defer stopBot()
	botClient, botChat := startTelegramBot(botCtx, svc, dbPath)

	rdr, err := web.NewRenderer()
	if err != nil {
		slog.Error("parse templates", "err", err)
		os.Exit(1)
	}
	h := web.NewHandler(svc, rdr)

	// API token gates the protected /api/* endpoints (push, gmail sync).
	apiToken := os.Getenv("API_PUSH_TOKEN")
	if apiToken != "" {
		h.SetAPIToken(apiToken)
	}

	// Outbound push API (POST /api/push) — needs a bot client + API token.
	if botClient != nil {
		h.EnablePush(botClient, botChat)
		if apiToken != "" {
			slog.Info("push API enabled", "endpoint", "POST /api/push", "default_chat", botChat != 0)
		} else {
			slog.Info("push API disabled (set API_PUSH_TOKEN to enable)")
		}
	}

	// Gmail credit-card import (scheduled + POST /gmail/sync, /api/gmail/sync).
	startGmailSync(botCtx, svc, h)

	// Optional scheduled proactive alerts (budgets, card dues, recurring) via Telegram.
	startAlerts(botCtx, h, botClient != nil)

	handler := h.Routes(http.FileServer(http.FS(web.StaticFS())))

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("serving", "addr", "http://localhost:"+port, "db", dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// healthProbe GETs the local /healthz and returns an exit code (0 healthy).
func healthProbe(port string) int {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// startTelegramBot launches the Telegram add-transaction bot when
// TELEGRAM_BOT_TOKEN is set. TELEGRAM_ALLOWED_CHAT (optional) restricts who may
// add. The poll offset is persisted next to the database.
// startTelegramBot starts the inbound bot (if a token is set) and returns the
// client and the configured chat for reuse by the outbound push API. Returns
// (nil, 0) when disabled or the token is bad.
func startTelegramBot(ctx context.Context, svc *service.Service, dbPath string) (*telegrambot.Client, int64) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		slog.Info("telegram bot disabled (set TELEGRAM_BOT_TOKEN to enable)")
		return nil, 0
	}
	var allowedChat int64
	if v := os.Getenv("TELEGRAM_ALLOWED_CHAT"); v != "" {
		allowedChat, _ = strconv.ParseInt(v, 10, 64)
	}

	client := telegrambot.NewClient(token)
	verifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	me, err := client.GetMe(verifyCtx)
	cancel()
	if err != nil {
		slog.Error("telegram bot: bad token, not starting", "err", err)
		return nil, 0
	}
	slog.Info("telegram bot enabled", "bot", "@"+me.Username, "restricted", allowedChat != 0)

	offsetPath := filepath.Join(filepath.Dir(dbPath), "telegram_offset")
	bot := telegrambot.NewBot(svc, client, allowedChat, offsetPath)
	go bot.Run(ctx, client)
	return client, allowedChat
}

// gmailRunner adapts a *gmailsync.Syncer to the web.GmailSyncer interface.
type gmailRunner struct{ s *gmailsync.Syncer }

func (g gmailRunner) Run(ctx context.Context) (web.GmailResult, error) {
	r, err := g.s.Run(ctx)
	return web.GmailResult{Scanned: r.Scanned, Imported: r.Imported, Skipped: r.Skipped}, err
}

// startGmailSync wires the Gmail importer (if configured) and runs it on a
// schedule: once shortly after boot, then every GMAIL_SYNC_INTERVAL.
func startGmailSync(ctx context.Context, svc *service.Service, h *web.Handler) {
	user := os.Getenv("GMAIL_IMAP_USER")
	pass := os.Getenv("GMAIL_IMAP_PASSWORD")
	if user == "" || pass == "" {
		slog.Info("gmail import disabled (set GMAIL_IMAP_USER + GMAIL_IMAP_PASSWORD to enable)")
		return
	}

	senders := gmailsync.DefaultCardSenders
	if v := os.Getenv("GMAIL_CC_SENDERS"); v != "" {
		senders = splitCSV(v)
	}
	lookback := 7
	if n, err := strconv.Atoi(os.Getenv("GMAIL_LOOKBACK_DAYS")); err == nil && n > 0 {
		lookback = n
	}
	interval := 6 * time.Hour
	if d, err := time.ParseDuration(os.Getenv("GMAIL_SYNC_INTERVAL")); err == nil && d > 0 {
		interval = d
	}

	fetcher := gmailsync.NewIMAPFetcher(gmailsync.IMAPConfig{
		Host:     envOr("GMAIL_IMAP_HOST", "imap.gmail.com:993"),
		Username: user,
		Password: pass,
		Mailbox:  envOr("GMAIL_MAILBOX", "INBOX"),
	})
	syncer := gmailsync.NewSyncer(fetcher, svc.Store, gmailsync.Config{
		Senders:         senders,
		LookbackDays:    lookback,
		DefaultCategory: envOr("GMAIL_DEFAULT_CATEGORY", "Miscellaneous"),
	}, time.Now)
	h.EnableGmail(gmailRunner{syncer})
	slog.Info("gmail import enabled", "user", user, "interval", interval.String(), "senders", len(senders))

	go func() {
		run := func() {
			res, err := syncer.Run(ctx)
			if err != nil {
				slog.Error("gmail sync", "err", err)
				return
			}
			slog.Info("gmail sync", "scanned", res.Scanned, "imported", res.Imported, "skipped", res.Skipped)
		}
		// First run a little after boot, then on the interval.
		first := time.NewTimer(30 * time.Second)
		defer first.Stop()
		select {
		case <-ctx.Done():
			return
		case <-first.C:
			run()
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

// startAlerts schedules proactive Telegram alerts when ALERTS_INTERVAL is set
// and the bot is available. The /api/alerts/run endpoint works regardless (for cron).
func startAlerts(ctx context.Context, h *web.Handler, botEnabled bool) {
	v := os.Getenv("ALERTS_INTERVAL")
	if v == "" {
		return
	}
	interval, err := time.ParseDuration(v)
	if err != nil || interval <= 0 {
		slog.Warn("alerts: invalid ALERTS_INTERVAL, scheduler disabled", "value", v)
		return
	}
	if !botEnabled {
		slog.Info("alerts: ALERTS_INTERVAL set but Telegram bot disabled; use POST /api/alerts/run instead")
		return
	}
	slog.Info("scheduled alerts enabled", "interval", interval.String())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				alerts, sent, err := h.RunAlerts(ctx)
				if err != nil {
					slog.Error("alerts run", "err", err)
					continue
				}
				slog.Info("alerts run", "count", len(alerts), "sent", sent)
			}
		}
	}()
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
