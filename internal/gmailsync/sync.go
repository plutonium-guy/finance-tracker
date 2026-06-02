package gmailsync

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"finance-tracker/internal/domain"
)

// Fetcher returns recent emails from the given senders since a cutoff time.
type Fetcher interface {
	Fetch(ctx context.Context, senders []string, since time.Time) ([]Message, error)
}

// DB is the subset of the store the syncer needs.
type DB interface {
	WasGmailProcessed(messageID string) (bool, error)
	MarkGmailProcessed(messageID, txID, processedAt string) error
	CreateTransaction(t domain.Transaction) error
	CardIDByLast4(last4 string) (string, bool, error)
	SetTransactionCard(txID, cardID string) error
}

// Config tunes the sync.
type Config struct {
	Senders         []string             // From-address substrings to match
	LookbackDays    int                  // how far back to search (default 7)
	DefaultCategory string               // category to file spends under
	DefaultMethod   domain.PaymentMethod // payment method to record
}

// Syncer imports card-spend emails into the store.
type Syncer struct {
	fetcher Fetcher
	db      DB
	cfg     Config
	now     func() time.Time
	mu      sync.Mutex // serializes Run so the scheduled ticker and a manual
	// /api/gmail/sync trigger can't process the same email concurrently and
	// double-insert it.
}

// Result summarizes a sync run.
type Result struct {
	Scanned  int `json:"scanned"`
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

// NewSyncer builds a Syncer. Defaults are applied for unset config.
func NewSyncer(fetcher Fetcher, db DB, cfg Config, now func() time.Time) *Syncer {
	if now == nil {
		now = time.Now
	}
	if cfg.LookbackDays <= 0 {
		cfg.LookbackDays = 7
	}
	if cfg.DefaultCategory == "" {
		cfg.DefaultCategory = "Miscellaneous"
	}
	if cfg.DefaultMethod == "" {
		cfg.DefaultMethod = domain.CreditCard
	}
	return &Syncer{fetcher: fetcher, db: db, cfg: cfg, now: now}
}

// Run fetches, parses, dedupes, and inserts. Each email is marked processed
// exactly once (spend or not) so subsequent runs skip it.
func (s *Syncer) Run(ctx context.Context) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	since := s.now().AddDate(0, 0, -s.cfg.LookbackDays)
	msgs, err := s.fetcher.Fetch(ctx, s.cfg.Senders, since)
	if err != nil {
		return Result{}, err
	}
	var res Result
	for _, m := range msgs {
		res.Scanned++
		if m.ID == "" {
			res.Skipped++
			continue
		}
		if done, _ := s.db.WasGmailProcessed(m.ID); done {
			res.Skipped++
			continue
		}
		parsed, ok := ParseCardEmail(m.Subject, m.Text)
		nowISO := s.now().UTC().Format(time.RFC3339)
		if !ok {
			// Remember it so we don't reparse the same non-spend every run.
			if err := s.db.MarkGmailProcessed(m.ID, "", nowISO); err != nil {
				slog.Error("gmail: mark non-spend processed", "id", m.ID, "err", err)
			}
			res.Skipped++
			continue
		}

		t := domain.Transaction{
			ID:            uuid.NewString(),
			Date:          m.Date.Format("2006-01-02"),
			Description:   describe(parsed.Merchant, m.Subject),
			Amount:        domain.ToPaise(parsed.Amount),
			Type:          domain.Expense,
			PaymentMethod: s.cfg.DefaultMethod,
			Category:      s.cfg.DefaultCategory,
			CreatedAt:     nowISO,
			UpdatedAt:     nowISO,
		}
		note := "imported from Gmail: " + truncate(m.Subject, 140)
		t.Notes = &note
		if err := s.db.CreateTransaction(t); err != nil {
			res.Skipped++
			continue // leave unmarked so a later run can retry
		}
		// Link the spend to a known card when the alert stated a last-4.
		if parsed.Last4 != "" {
			if cardID, ok, _ := s.db.CardIDByLast4(parsed.Last4); ok {
				if err := s.db.SetTransactionCard(t.ID, cardID); err != nil {
					slog.Error("gmail: link transaction to card", "tx", t.ID, "last4", parsed.Last4, "err", err)
				}
			}
		}
		if err := s.db.MarkGmailProcessed(m.ID, t.ID, nowISO); err != nil {
			// Transaction is already saved; if we fail to record it as
			// processed, a later run could re-import it. Surface it loudly.
			slog.Error("gmail: imported but failed to mark processed (may re-import)", "id", m.ID, "tx", t.ID, "err", err)
		}
		res.Imported++
	}
	return res, nil
}

// describe picks a transaction description: the merchant, else a cleaned subject.
func describe(merchant, subject string) string {
	if merchant != "" {
		return merchant
	}
	s := strings.TrimSpace(subject)
	for _, p := range []string{"Alert:", "Transaction Alert:", "Txn Alert:", "Fwd:", "Re:"} {
		s = strings.TrimSpace(strings.TrimPrefix(s, p))
	}
	if s == "" {
		return "Card transaction"
	}
	return truncate(s, 100)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return strings.TrimSpace(s[:n])
	}
	return s
}
