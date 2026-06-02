// Package pdfimport is a best-effort importer for bank/card statement PDFs.
// PDF statement layouts are bank-specific, so extraction is heuristic: it pulls
// text lines, then for each line looks for a date and a money amount and treats
// the rest as the description. Review imported rows afterwards.
package pdfimport

import (
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"rsc.io/pdf"

	"finance-tracker/internal/domain"
)

// Candidate is a parsed statement line, ready to become a Transaction.
type Candidate struct {
	Date        string // YYYY-MM-DD
	Description string
	Amount      float64
	Type        domain.TransactionType
}

var (
	dateRe   = regexp.MustCompile(`\b(\d{1,2}[/-]\d{1,2}[/-]\d{2,4}|\d{1,2}[ -][A-Za-z]{3}[ -]\d{2,4})\b`)
	moneyRe  = regexp.MustCompile(`\d[\d,]*\.\d{2}\b`)
	crRe     = regexp.MustCompile(`(?i)\b(cr|credit)\b`)
	drRe     = regexp.MustCompile(`(?i)\b(dr|debit)\b`)
	dateFmts = []string{"02/01/2006", "02-01-2006", "02/01/06", "02-01-06", "02-Jan-2006", "02-Jan-06", "02 Jan 2006", "02 Jan 06", "2/1/2006", "2-1-2006"}
)

// ExtractLines reads a PDF and returns its text reconstructed into lines.
func ExtractLines(r io.ReaderAt, size int64) ([]string, error) {
	doc, err := pdf.NewReader(r, size)
	if err != nil {
		return nil, err
	}
	var lines []string
	for i := 1; i <= doc.NumPage(); i++ {
		p := doc.Page(i)
		if p.V.IsNull() {
			continue
		}
		// Group runes into lines by their rounded Y position, ordered by X.
		type tok struct {
			x float64
			s string
		}
		rows := map[int][]tok{}
		for _, t := range p.Content().Text {
			key := int(math.Round(t.Y))
			rows[key] = append(rows[key], tok{x: t.X, s: t.S})
		}
		var keys []int
		for k := range rows {
			keys = append(keys, k)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(keys))) // top of page first (higher Y)
		for _, k := range keys {
			toks := rows[k]
			sort.Slice(toks, func(a, b int) bool { return toks[a].x < toks[b].x })
			var sb strings.Builder
			for _, t := range toks {
				sb.WriteString(t.s)
			}
			if line := strings.TrimSpace(sb.String()); line != "" {
				lines = append(lines, line)
			}
		}
	}
	return lines, nil
}

// ParseLines turns reconstructed statement lines into transaction candidates.
// A line must contain a parseable date and a 2-decimal money amount.
func ParseLines(lines []string) []Candidate {
	var out []Candidate
	for _, line := range lines {
		dm := dateRe.FindString(line)
		if dm == "" {
			continue
		}
		date, ok := normalizeDate(dm)
		if !ok {
			continue
		}
		amounts := moneyRe.FindAllString(line, -1)
		if len(amounts) == 0 {
			continue
		}
		// The first money amount is usually the transaction value (a running
		// balance, if present, tends to come after).
		amt, err := strconv.ParseFloat(strings.ReplaceAll(amounts[0], ",", ""), 64)
		if err != nil || amt <= 0 {
			continue
		}
		typ := domain.Expense
		if crRe.MatchString(line) && !drRe.MatchString(line) {
			typ = domain.Income
		}
		desc := cleanDescription(line, dm, amounts)
		if desc == "" {
			desc = "Statement entry"
		}
		out = append(out, Candidate{Date: date, Description: desc, Amount: amt, Type: typ})
	}
	return out
}

// cleanDescription strips the date and money tokens out of a line.
func cleanDescription(line, date string, amounts []string) string {
	line = strings.Replace(line, date, " ", 1)
	for _, a := range amounts {
		line = strings.Replace(line, a, " ", 1)
	}
	line = strings.Map(func(r rune) rune {
		if r == '|' {
			return ' '
		}
		return r
	}, line)
	desc := strings.TrimSpace(strings.Join(strings.Fields(line), " "))
	if len(desc) > 200 {
		desc = strings.TrimSpace(desc[:200])
	}
	return desc
}

func normalizeDate(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for _, f := range dateFmts {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}
