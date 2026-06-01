package domain

// Frequency is how often a recurring item repeats.
type Frequency string

const (
	Monthly    Frequency = "Monthly"
	Quarterly  Frequency = "Quarterly"
	HalfYearly Frequency = "Half-Yearly"
	Yearly     Frequency = "Yearly"
	Weekly     Frequency = "Weekly"
	OneTime    Frequency = "One-time"
)

// ValidFrequencies backs server-side enum validation.
var ValidFrequencies = []Frequency{Monthly, Quarterly, HalfYearly, Yearly, Weekly, OneTime}

func IsValidFrequency(f Frequency) bool {
	for _, v := range ValidFrequencies {
		if v == f {
			return true
		}
	}
	return false
}

// RecurringItem is a repeating income or expense commitment.
type RecurringItem struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Category      string          `json:"category"`
	Amount        Money           `json:"amount"`
	Frequency     Frequency       `json:"frequency"`
	Type          TransactionType `json:"type"` // Income | Expense
	StartDate     string          `json:"start_date"`
	EndDate       *string         `json:"end_date,omitempty"`
	PaymentMethod PaymentMethod   `json:"payment_method"`
	Active        bool            `json:"active"`
	Notes         *string         `json:"notes,omitempty"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}
