// Package navsync fetches mutual-fund NAVs from AMFI's daily file and updates
// holdings' last price by scheme code.
package navsync

import (
	"bufio"
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AMFIURL is AMFI's public daily NAV dump (semicolon-delimited, no auth).
const AMFIURL = "https://www.amfiindia.com/spages/NAVAll.txt"

// Fetcher returns a map of AMFI scheme code → NAV in rupees.
type Fetcher interface {
	Fetch(ctx context.Context) (map[string]float64, error)
}

// HTTPFetcher downloads and parses the AMFI NAV file.
type HTTPFetcher struct {
	URL    string
	Client *http.Client
}

// NewHTTPFetcher builds a fetcher with sane defaults.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{URL: AMFIURL, Client: &http.Client{Timeout: 30 * time.Second}}
}

func (f *HTTPFetcher) Fetch(ctx context.Context) (map[string]float64, error) {
	url := f.URL
	if url == "" {
		url = AMFIURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ParseNAVAll(resp.Body)
}

// ParseNAVAll parses the AMFI NAVAll.txt format. Each data line is:
//
//	Scheme Code;ISIN Div Payout/Growth;ISIN Div Reinvest;Scheme Name;Net Asset Value;Date
//
// Header rows, AMC-name rows, and blank lines (which lack the fields or have a
// non-numeric code/NAV) are skipped.
func ParseNAVAll(r interface{ Read([]byte) (int, error) }) (map[string]float64, error) {
	out := map[string]float64{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, ";") {
			continue
		}
		parts := strings.Split(line, ";")
		if len(parts) < 6 {
			continue
		}
		code := strings.TrimSpace(parts[0])
		if code == "" || !isDigits(code) {
			continue // header ("Scheme Code") or AMC name row
		}
		nav, err := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
		if err != nil || nav <= 0 {
			continue
		}
		out[code] = nav
	}
	return out, sc.Err()
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}
