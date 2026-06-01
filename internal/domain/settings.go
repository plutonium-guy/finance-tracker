package domain

// AppSettings is the single-row application configuration.
type AppSettings struct {
	Currency        string `json:"currency"`          // "₹"
	Locale          string `json:"locale"`            // "en-IN"
	MonthFormat     string `json:"month_format"`      // "mmm yyyy"
	DateFormat      string `json:"date_format"`       // "dd-mmm-yyyy"
	FiscalYearStart string `json:"fiscal_year_start"` // "April" | "January"
	PayCycleEnabled bool   `json:"pay_cycle_enabled"`
	DarkMode        bool   `json:"dark_mode"`
}

// DefaultSettings returns the canonical defaults used to seed the settings row.
func DefaultSettings() AppSettings {
	return AppSettings{
		Currency:        "₹",
		Locale:          "en-IN",
		MonthFormat:     "mmm yyyy",
		DateFormat:      "dd-mmm-yyyy",
		FiscalYearStart: "April",
		PayCycleEnabled: true,
		DarkMode:        false,
	}
}

// DefaultCategories is the seeded category set (build spec §Data model).
var DefaultCategories = []string{
	"Groceries", "Food & Dining", "Travel", "Transport", "Shopping",
	"Utilities", "Rent", "Entertainment", "Healthcare", "Education",
	"Investment", "Insurance", "Salary", "Freelance", "Refund", "Gift",
	"Plantation Fund", "Loan", "Borrowed", "Lent", "Split", "Miscellaneous",
}

// Category is a dynamic spending category (validated against the DB, not an enum).
type Category struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}
