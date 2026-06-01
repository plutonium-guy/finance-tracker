package telegrambot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/service"
)

const pollTimeout = 50 // seconds

// Sender sends a reply to a chat (satisfied by *Client; faked in tests).
type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

// Bot turns chat messages into transactions via the service.
type Bot struct {
	svc         *service.Service
	send        Sender
	allowedChat int64  // 0 => any chat may add
	offsetPath  string // file to persist the poll offset (avoids reprocessing)
}

// NewBot builds a Bot. allowedChat of 0 allows any chat.
func NewBot(svc *service.Service, send Sender, allowedChat int64, offsetPath string) *Bot {
	return &Bot{svc: svc, send: send, allowedChat: allowedChat, offsetPath: offsetPath}
}

const helpText = `🧾 Finance Tracker bot

Add a transaction:
  250 Coffee
  1,234.50 Groceries cat:Groceries method:upi
  85000 Salary type:income cat:Salary

Keys (all optional): cat:  method:  type:  date:YYYY-MM-DD
Defaults: type=expense, method=UPI, category=Miscellaneous, date=today.

Other commands:
  /balance — this month's credits, debits, net
  /help — this message`

// HandleMessage processes one incoming message and replies. Errors are reported
// back to the user; transport errors are logged.
func (b *Bot) HandleMessage(ctx context.Context, m *Message) {
	if m == nil {
		return
	}
	if b.allowedChat != 0 && m.Chat.ID != b.allowedChat {
		slog.Warn("telegram: ignoring message from unauthorized chat", "chat", m.Chat.ID)
		return
	}
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return
	}

	switch {
	case text == "/start" || text == "/help" || text == "help":
		b.reply(ctx, m.Chat.ID, helpText)
		return
	case text == "/balance" || text == "balance":
		b.reply(ctx, m.Chat.ID, b.balance())
		return
	}

	p, err := ParseMessage(text, b.svc.Now())
	if err != nil {
		b.reply(ctx, m.Chat.ID, "⚠️ "+err.Error()+"\n\nSend /help for the format.")
		return
	}

	category, err := b.resolveCategory(p.Category)
	if err != nil {
		b.reply(ctx, m.Chat.ID, "⚠️ "+err.Error())
		return
	}

	now := b.svc.Now().UTC().Format(time.RFC3339)
	t := domain.Transaction{
		ID:            uuid.NewString(),
		Date:          p.Date,
		Description:   p.Description,
		Amount:        domain.ToPaise(p.Amount),
		Type:          p.Type,
		PaymentMethod: p.Method,
		Category:      category,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	note := "added via Telegram"
	t.Notes = &note

	if err := b.svc.Store.CreateTransaction(t); err != nil {
		b.reply(ctx, m.Chat.ID, "❌ Could not save: "+err.Error())
		return
	}
	b.reply(ctx, m.Chat.ID, fmt.Sprintf("✅ Added %s — %s\n%s · %s · %s · %s",
		t.Amount.FormatINR(), t.Description, t.Category, t.Type, t.PaymentMethod, fmtDate(t.Date)))
}

// resolveCategory returns the canonical category name, defaulting to
// Miscellaneous when none is given. A provided-but-unknown category is an error.
func (b *Bot) resolveCategory(want string) (string, error) {
	cats, err := b.svc.Store.ListCategories()
	if err != nil {
		return "", err
	}
	if want == "" {
		for _, c := range cats {
			if c.Name == "Miscellaneous" {
				return c.Name, nil
			}
		}
		if len(cats) > 0 {
			return cats[0].Name, nil
		}
		return "", fmt.Errorf("no categories exist")
	}
	// 1) exact (case-insensitive) match.
	for _, c := range cats {
		if strings.EqualFold(c.Name, want) {
			return c.Name, nil
		}
	}
	// 2) unique prefix, then unique substring match (so "food" -> "Food & Dining").
	lw := strings.ToLower(want)
	for _, match := range []func(name string) bool{
		func(name string) bool { return strings.HasPrefix(name, lw) },
		func(name string) bool { return strings.Contains(name, lw) },
	} {
		var hits []string
		for _, c := range cats {
			if match(strings.ToLower(c.Name)) {
				hits = append(hits, c.Name)
			}
		}
		if len(hits) == 1 {
			return hits[0], nil
		}
	}
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		names = append(names, c.Name)
	}
	return "", fmt.Errorf("unknown category %q. Valid: %s", want, strings.Join(names, ", "))
}

// balance renders the current month's headline figures.
func (b *Bot) balance() string {
	txs, err := b.svc.Store.AllTransactions()
	if err != nil {
		return "❌ " + err.Error()
	}
	month := b.svc.CurrentMonth()
	k := b.svc.KPIsFor(txs, month)
	rec, _ := b.svc.Store.ListRecurring()
	pc := b.svc.PayCycleFor(txs, rec, month)
	return fmt.Sprintf("📊 %s\nCredits: %s\nDebits:  %s\nNet:     %s\nPay-cycle balance: %s",
		service.MonthKey(b.svc.Now()), k.MonthCredits.FormatINR(), k.MonthDebits.FormatINR(),
		k.MonthNet.FormatINR(), pc.Balance.FormatINR())
}

func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	if err := b.send.SendMessage(ctx, chatID, text); err != nil {
		slog.Error("telegram: send failed", "chat", chatID, "err", err)
	}
}

// Run long-polls Telegram and dispatches messages until ctx is cancelled.
func (b *Bot) Run(ctx context.Context, client *Client) {
	offset := b.loadOffset()
	if offset > 0 {
		slog.Info("telegram: resuming", "offset", offset)
	}
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		updates, err := client.GetUpdates(ctx, offset, pollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("telegram: getUpdates", "err", err, "retry_in", backoff.String())
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		for _, u := range updates {
			b.HandleMessage(ctx, u.Message)
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
				// Persist after each handled update, not once per batch, so a
				// crash reprocesses at most the single in-flight message rather
				// than re-adding every transaction in the batch.
				b.saveOffset(offset)
			}
		}
	}
}

func (b *Bot) loadOffset() int64 {
	if b.offsetPath == "" {
		return 0
	}
	data, err := os.ReadFile(b.offsetPath)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		// Don't silently fall back to 0 — that would reprocess every pending
		// update and create duplicate transactions. Surface it instead.
		slog.Error("telegram: corrupt offset file, resuming from 0 (may reprocess)", "path", b.offsetPath, "err", err)
		return 0
	}
	return n
}

func (b *Bot) saveOffset(offset int64) {
	if b.offsetPath == "" {
		return
	}
	if err := os.WriteFile(b.offsetPath, []byte(strconv.FormatInt(offset, 10)), 0o644); err != nil {
		slog.Error("telegram: save offset", "err", err)
	}
}

func fmtDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02-Jan-2006")
}
