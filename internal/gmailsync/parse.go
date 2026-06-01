// Package gmailsync imports credit-card spend alerts from Gmail (over IMAP) and
// inserts them as expense transactions, deduplicated by email Message-ID.
//
// Bank alert formats vary, so parsing is heuristic and tunable: it extracts an
// amount and a best-effort merchant, and only accepts messages that look like a
// card *spend* (not payments, statements, OTPs, or rewards).
package gmailsync

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Message is a fetched email reduced to the fields the parser needs.
type Message struct {
	ID      string // Message-ID (dedupe key)
	From    string
	Subject string
	Date    time.Time
	Text    string // plain-text body (HTML stripped)
}

// Parsed is the outcome of parsing a card-alert email.
type Parsed struct {
	Amount   float64
	Merchant string
}

var (
	// amount after a currency token: Rs. / Rs / INR / ₹. The transaction amount
	// is stated before any "available/total limit", so the first match wins.
	amountRe = regexp.MustCompile(`(?i)(?:rs\.?|inr|₹)\s*([0-9][0-9,]*(?:\.[0-9]{1,2})?)`)
	// "at MERCHANT" up to "on"/punctuation/end (only used when no Info: field).
	atRe = regexp.MustCompile(`(?i)\b(?:at|towards|to)\s+([A-Za-z0-9][A-Za-z0-9 .&'_/*-]{1,40}?)(?:\s+on\b|\s+using\b|[.,;\n]|$)`)
	// ICICI/HDFC put the payee in an "Info:" field, e.g. "Info: UPI-12345-Mr Aasi".
	infoRe = regexp.MustCompile(`(?i)\bInfo:\s*([^.\n]+)`)
	// A spend signal: explicit verbs, or a "transaction alert/of" phrasing such
	// as ICICI's subject "Transaction alert for your ICICI Bank Credit Card".
	spendRe = regexp.MustCompile(`(?i)\b(spent|debited|charged|purchase|paid|was used|used (?:at|on|for)|transaction (?:of|alert)|txn)\b`)
	// things that mean it is NOT a card spend we want to record. (Note: the
	// "available limit" line that appears in every spend alert is intentionally
	// NOT excluded.)
	excludeRe = regexp.MustCompile(`(?i)\b(credited to|statement|e-?statement|reward points|refund|revers(?:ed|al)|payment received|payment of .* received|total amount due|minimum (?:amount )?due|\botp\b|one[ -]time password|due date|declined|failed)\b`)
	// a bare time like 09:00 or 09:00:24 (so "at 09:00:24" isn't read as a merchant)
	timeRe = regexp.MustCompile(`^\d{1,2}:\d{2}(?::\d{2})?$`)
)

// ParseCardEmail extracts a spend from an email's subject + body. The second
// return value is false when the message is not a recordable card spend.
func ParseCardEmail(subject, body string) (Parsed, bool) {
	text := subject + "\n" + body
	if !spendRe.MatchString(text) || excludeOnlyContext(text) {
		return Parsed{}, false
	}
	m := amountRe.FindStringSubmatch(text)
	if m == nil {
		return Parsed{}, false
	}
	amt, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil || amt <= 0 {
		return Parsed{}, false
	}
	return Parsed{Amount: amt, Merchant: extractMerchant(text)}, true
}

// excludeOnlyContext reports whether the message is a non-spend (payment,
// statement, OTP, reward, etc.). The "available limit" phrasing that legitimately
// appears in spend alerts is handled by the precise excludeRe above.
func excludeOnlyContext(text string) bool {
	return excludeRe.MatchString(text)
}

// extractMerchant prefers an "Info:" payee field (ICICI/UPI), then "at MERCHANT",
// ignoring time-of-day and all-numeric matches.
func extractMerchant(text string) string {
	if m := infoRe.FindStringSubmatch(text); m != nil {
		if name := merchantFromInfo(m[1]); name != "" {
			return name
		}
	}
	for _, m := range atRe.FindAllStringSubmatch(text, -1) {
		cand := cleanMerchant(m[1])
		if cand != "" && !timeRe.MatchString(cand) && hasLetters(cand) {
			return cand
		}
	}
	return ""
}

// merchantFromInfo turns "UPI-651816620443-Mr Aasi" into "Mr Aasi" and
// "POS-AMAZON" into "AMAZON"; a plain "AMAZON" passes through.
func merchantFromInfo(info string) string {
	info = strings.TrimSpace(info)
	if parts := strings.Split(info, "-"); len(parts) >= 2 {
		last := strings.TrimSpace(parts[len(parts)-1])
		if hasLetters(last) {
			return cleanMerchant(last)
		}
	}
	return cleanMerchant(info)
}

func cleanMerchant(s string) string {
	s = strings.Trim(strings.TrimSpace(s), ".,;:")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 60 {
		s = strings.TrimSpace(s[:60])
	}
	return s
}

func hasLetters(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// stripHTML removes tags and decodes a few common entities so the parser sees
// readable text when only an HTML body is present.
var tagRe = regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</(?:script|style)>|<[^>]+>`)

func stripHTML(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	r := strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&#39;", "'", "&quot;", `"`, "&rsquo;", "'")
	s = r.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}
