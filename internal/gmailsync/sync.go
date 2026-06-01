package gmailsync

import (
	"context"
	"strings"
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
			s.db.MarkGmailProcessed(m.ID, "", nowISO)
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
		s.db.MarkGmailProcessed(m.ID, t.ID, nowISO)
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
