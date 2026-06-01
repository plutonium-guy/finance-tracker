package domain

// Card is a credit card with a billing cycle. Statements close on StatementDay
// of each month (1..28, kept ≤28 so it exists in every month) and payment is due
// DueOffsetDays later. Limit is the total credit limit.
type Card struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Last4         string `json:"last4"` // last 4 digits, for matching bank alerts
	Limit         Money  `json:"limit"`
	StatementDay  int    `json:"statement_day"`   // 1..28
	DueOffsetDays int    `json:"due_offset_days"` // due = statement date + N days
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// StatementPayment records that a card's statement (identified by its closing
// date) was paid, along with the Transfer transaction created for it.
type StatementPayment struct {
	CardID    string `json:"card_id"`
	PeriodEnd string `json:"period_end"` // "YYYY-MM-DD" statement closing date
	Amount    Money  `json:"amount"`
	TxID      string `json:"transaction_id"`
	PaidAt    string `json:"paid_at"`
}
