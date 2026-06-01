package domain

// Goal is a savings target with optional deadline.
type Goal struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Target     Money   `json:"target"`
	Saved      Money   `json:"saved"`
	TargetDate *string `json:"target_date,omitempty"` // "YYYY-MM-DD"
	Notes      *string `json:"notes,omitempty"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// Budget is a per-category monthly spending limit.
type Budget struct {
	Category string `json:"category"`
	Limit    Money  `json:"limit"`
}

// Tag is a free-form label that can be attached to transactions.
type Tag struct {
	Name string `json:"name"`
}
