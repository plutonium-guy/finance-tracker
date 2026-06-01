// Package telegrambot lets a user add transactions to the tracker by messaging
// a Telegram bot. It long-polls the Bot API, parses each message into a
// transaction, inserts it via the service, and replies with a confirmation.
package telegrambot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"finance-tracker/internal/domain"
)

// Parsed is a transaction parsed from a chat message (before category/store
// validation). Category may be empty (meaning "use the default").
type Parsed struct {
	Amount      float64
	Description string
	Category    string
	Method      domain.PaymentMethod
	Type        domain.TransactionType
	Date        string // "YYYY-MM-DD"
}

// methodAliases maps lowercase shorthands to canonical payment methods.
var methodAliases = map[string]domain.PaymentMethod{
	"upi": domain.UPI, "cash": domain.Cash,
	"cc": domain.CreditCard, "credit": domain.CreditCard, "creditcard": domain.CreditCard,
	"dc": domain.DebitCard, "debit": domain.DebitCard, "debitcard": domain.DebitCard,
	"bank": domain.BankTransfer, "banktransfer": domain.BankTransfer, "transfer": domain.BankTransfer,
	"netbanking": domain.NetBanking, "nb": domain.NetBanking,
	"cheque": domain.Cheque, "check": domain.Cheque, "other": domain.OtherMethod,
}

// typeAliases maps lowercase shorthands to transaction types.
var typeAliases = map[string]domain.TransactionType{
	"expense": domain.Expense, "exp": domain.Expense, "spend": domain.Expense,
	"income": domain.Income, "in": domain.Income, "credit": domain.Income, "salary": domain.Income,
	"transfer": domain.Transfer, "xfer": domain.Transfer,
}

// ParseMessage parses a free-form add message into a Parsed transaction.
//
// Format: "<amount> <description> [key:value ...]"
// keys: cat|category|c, method|m|pm, type|t, date|d
// e.g. "250 Coffee cat:Food method:upi"  ·  "1,234.50 Salary type:income"
func ParseMessage(text string, now time.Time) (Parsed, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "/add")
	text = strings.TrimSpace(text)
	if text == "" {
		return Parsed{}, fmt.Errorf("empty message")
	}

	fields := strings.Fields(text)
	p := Parsed{
		Method: domain.UPI,
		Type:   domain.Expense,
		Date:   now.Format("2006-01-02"),
	}

	// First field is the amount.
	amt, err := parseAmount(fields[0])
	if err != nil {
		return p, fmt.Errorf("first word must be an amount, e.g. 250 or 1,234.50")
	}
	p.Amount = amt

	var descWords []string
	for _, f := range fields[1:] {
		key, val, ok := splitKV(f)
		if !ok {
			descWords = append(descWords, f)
			continue
		}
		switch key {
		case "cat", "category", "c":
			p.Category = val
		case "method", "m", "pm":
			if mm, ok := methodAliases[strings.ToLower(val)]; ok {
				p.Method = mm
			} else if domain.IsValidPaymentMethod(domain.PaymentMethod(val)) {
				p.Method = domain.PaymentMethod(val)
			} else {
				return p, fmt.Errorf("unknown method %q", val)
			}
		case "type", "t":
			if tt, ok := typeAliases[strings.ToLower(val)]; ok {
				p.Type = tt
			} else {
				return p, fmt.Errorf("unknown type %q (use expense|income|transfer)", val)
			}
		case "date", "d":
			if _, err := time.Parse("2006-01-02", val); err != nil {
				return p, fmt.Errorf("date must be YYYY-MM-DD")
			}
			p.Date = val
		default:
			// Unknown key:value — treat as part of the description.
			descWords = append(descWords, f)
		}
	}

	p.Description = strings.TrimSpace(strings.Join(descWords, " "))
	if p.Description == "" {
		return p, fmt.Errorf("description is required, e.g. \"250 Coffee\"")
	}
	if len(p.Description) > 200 {
		return p, fmt.Errorf("description too long (max 200 chars)")
	}
	return p, nil
}

// parseAmount accepts plain numbers, ₹ prefixes, and Indian comma grouping.
func parseAmount(s string) (float64, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "₹")
	s = strings.ReplaceAll(s, ",", "")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if v <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}
	return v, nil
}

// splitKV splits "key:value" (value may itself be empty-checked by caller).
func splitKV(token string) (key, val string, ok bool) {
	i := strings.IndexByte(token, ':')
	if i <= 0 || i == len(token)-1 {
		return "", "", false
	}
	return strings.ToLower(token[:i]), token[i+1:], true
}
