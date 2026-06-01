package domain

// TransactionType is the kind of a transaction.
type TransactionType string

const (
	Income   TransactionType = "Income"
	Expense  TransactionType = "Expense"
	Transfer TransactionType = "Transfer"
)

// PaymentMethod is how money moved.
type PaymentMethod string

const (
	UPI          PaymentMethod = "UPI"
	CreditCard   PaymentMethod = "Credit Card"
	DebitCard    PaymentMethod = "Debit Card"
	Cash         PaymentMethod = "Cash"
	BankTransfer PaymentMethod = "Bank Transfer"
	NetBanking   PaymentMethod = "NetBanking"
	Cheque       PaymentMethod = "Cheque"
	OtherMethod  PaymentMethod = "Other"
)

// ValidTransactionTypes / ValidPaymentMethods back server-side enum validation.
var ValidTransactionTypes = []TransactionType{Income, Expense, Transfer}

var ValidPaymentMethods = []PaymentMethod{
	UPI, CreditCard, DebitCard, Cash, BankTransfer, NetBanking, Cheque, OtherMethod,
}

// Transaction is a single ledger entry. Amount is always > 0; direction comes
// from Type (see SignedAmount).
type Transaction struct {
	ID            string          `json:"id"`
	Date          string          `json:"date"` // "2006-01-02"
	Description   string          `json:"description"`
	Amount        Money           `json:"amount"`
	Type          TransactionType `json:"type"`
	PaymentMethod PaymentMethod   `json:"payment_method"`
	Category      string          `json:"category"`
	Notes         *string         `json:"notes,omitempty"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

// Month returns the "YYYY-MM" month key for the transaction date.
func (t Transaction) Month() string {
	if len(t.Date) >= 7 {
		return t.Date[:7]
	}
	return t.Date
}

// SignedAmount yields +amount for Income, -amount for Expense, 0 for Transfer.
func (t Transaction) SignedAmount() Money {
	switch t.Type {
	case Income:
		return t.Amount
	case Expense:
		return -t.Amount
	default:
		return 0
	}
}

// IsValidType / IsValidPaymentMethod report enum membership.
func IsValidType(t TransactionType) bool {
	for _, v := range ValidTransactionTypes {
		if v == t {
			return true
		}
	}
	return false
}

func IsValidPaymentMethod(p PaymentMethod) bool {
	for _, v := range ValidPaymentMethods {
		if v == p {
			return true
		}
	}
	return false
}
