// Package store provides persistence for the finance tracker over SQLite.
package store

import (
	"errors"

	"finance-tracker/internal/domain"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// ErrCategoryInUse is returned when deleting a category referenced by rows.
var ErrCategoryInUse = errors.New("category in use")

// TxFilter narrows a transactions query. Zero values mean "no constraint".
type TxFilter struct {
	Month         string // "YYYY-MM"
	Category      string
	Type          string
	PaymentMethod string
	Query         string // substring match on description
	Sort          string // date|amount|description|category|type
	Order         string // asc|desc
	Page          int    // 1-based; 0 => 1
	PageSize      int    // 0 => no limit
}

// Backup is the full exportable dataset.
type Backup struct {
	Transactions []domain.Transaction   `json:"transactions"`
	Recurring    []domain.RecurringItem `json:"recurring"`
	Settings     domain.AppSettings     `json:"settings"`
	Categories   []domain.Category      `json:"categories"`
	Goals        []domain.Goal          `json:"goals,omitempty"`
	Budgets      []domain.Budget        `json:"budgets,omitempty"`
	Tags         []domain.Tag           `json:"tags,omitempty"`
	Cards             []domain.Card             `json:"cards,omitempty"`
	StatementPayments []domain.StatementPayment `json:"statement_payments,omitempty"`
	TransactionCards  map[string]string         `json:"transaction_cards,omitempty"` // txID -> cardID
}

// Store is the persistence interface used by the service and web layers.
type Store interface {
	// lifecycle
	Migrate() error
	EnsureSettings() error
	SeedDefaultCategories() error
	Close() error

	// categories
	ListCategories() ([]domain.Category, error)
	AddCategory(name string) error
	RenameCategory(oldName, newName string) error // cascades to rows
	DeleteCategory(name string, reassignTo string) error
	CategoryInUse(name string) (bool, error)

	// transactions
	Transactions(f TxFilter) ([]domain.Transaction, int, error) // rows, totalCount
	AllTransactions() ([]domain.Transaction, error)
	GetTransaction(id string) (domain.Transaction, error)
	CreateTransaction(t domain.Transaction) error
	UpdateTransaction(t domain.Transaction) error
	DeleteTransaction(id string) error

	// recurring
	ListRecurring() ([]domain.RecurringItem, error)
	GetRecurring(id string) (domain.RecurringItem, error)
	CreateRecurring(r domain.RecurringItem) error
	UpdateRecurring(r domain.RecurringItem) error
	DeleteRecurring(id string) error
	ToggleRecurring(id string) (domain.RecurringItem, error)

	// settings
	GetSettings() (domain.AppSettings, error)
	SaveSettings(s domain.AppSettings) error

	// goals
	ListGoals() ([]domain.Goal, error)
	GetGoal(id string) (domain.Goal, error)
	CreateGoal(g domain.Goal) error
	UpdateGoal(g domain.Goal) error
	DeleteGoal(id string) error

	// budgets
	ListBudgets() ([]domain.Budget, error)
	SetBudget(b domain.Budget) error // upsert
	DeleteBudget(category string) error

	// tags
	ListTags() ([]domain.Tag, error)
	SetTransactionTags(txID string, tags []string) error // replaces the set
	TagsFor(txID string) ([]string, error)
	TagsForMany(txIDs []string) (map[string][]string, error)
	TransactionIDsWithTag(tag string) ([]string, error)

	// recurring auto-fire bookkeeping
	WasPosted(recurringID, month string) (bool, error)
	MarkPosted(recurringID, month, txID, postedAt string) error

	// gmail import dedupe
	WasGmailProcessed(messageID string) (bool, error)
	MarkGmailProcessed(messageID, txID, processedAt string) error

	// credit cards + billing cycle
	ListCards() ([]domain.Card, error)
	GetCard(id string) (domain.Card, error)
	CreateCard(c domain.Card) error
	UpdateCard(c domain.Card) error
	DeleteCard(id string) error
	CardIDByLast4(last4 string) (string, bool, error)
	SetTransactionCard(txID, cardID string) error // cardID "" clears the link
	CardOfTransaction(txID string) (string, error) // "" when none
	TransactionCardMap() (map[string]string, error)
	RecordStatementPayment(p domain.StatementPayment) (bool, error) // false if already paid
	ListStatementPayments(cardID string) ([]domain.StatementPayment, error)

	// backup
	Export() (Backup, error)
	Import(b Backup) error // atomic replace
	Reset() error          // wipe + re-seed defaults

	// helpers
	IsEmpty() (bool, error) // no transactions and no recurring
}
