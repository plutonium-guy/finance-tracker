package domain

// AccountType categorizes where money sits.
type AccountType string

const (
	BankAccount   AccountType = "Bank"
	CashAccount   AccountType = "Cash"
	WalletAccount AccountType = "Wallet"
	OtherAccount  AccountType = "Other"
)

// ValidAccountTypes backs server-side validation.
var ValidAccountTypes = []AccountType{BankAccount, CashAccount, WalletAccount, OtherAccount}

// IsValidAccountType reports enum membership.
func IsValidAccountType(t AccountType) bool {
	for _, v := range ValidAccountTypes {
		if v == t {
			return true
		}
	}
	return false
}

// Account is a place money lives (bank, cash, wallet). Its balance is the
// opening balance plus the signed amounts of transactions linked to it.
type Account struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Type           AccountType `json:"type"`
	OpeningBalance Money       `json:"opening_balance"` // may be negative
	CreatedAt      string      `json:"created_at"`
	UpdatedAt      string      `json:"updated_at"`
}
