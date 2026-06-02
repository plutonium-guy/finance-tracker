package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"finance-tracker/internal/domain"
)

func reqAuth(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func getAuth(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	return reqAuth(t, h, "GET", path, token)
}

func deleteAuth(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	return reqAuth(t, h, "DELETE", path, token)
}

func TestAPIv1AuthAndCRUD(t *testing.T) {
	router, db := newTestServerWithToken(t, "secret")

	// Auth gate.
	if rec := get(t, router, "/api/v1/accounts"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token list: status %d, want 401", rec.Code)
	}

	// Create an account (paise).
	rec := postJSON(t, router, "/api/v1/accounts", "secret", `{"name":"HDFC","type":"Bank","opening_balance":100000}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account: status %d body %s", rec.Code, rec.Body.String())
	}
	var acc domain.Account
	json.Unmarshal(rec.Body.Bytes(), &acc)
	if acc.ID == "" || acc.OpeningBalance != 100000 {
		t.Fatalf("created account wrong: %+v", acc)
	}

	// List includes it.
	if rec := getAuth(t, router, "/api/v1/accounts", "secret"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "HDFC") {
		t.Fatalf("list: status %d body %s", rec.Code, rec.Body.String())
	}

	// Get by id.
	if rec := getAuth(t, router, "/api/v1/accounts/"+acc.ID, "secret"); rec.Code != 200 {
		t.Fatalf("get: status %d", rec.Code)
	}

	// Create a transaction via the API (paise), linked to the account.
	rec = postJSON(t, router, "/api/v1/transactions", "secret",
		`{"amount":50000,"description":"Salary","type":"Income","category":"Miscellaneous","account_id":"`+acc.ID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create txn: status %d body %s", rec.Code, rec.Body.String())
	}

	// Net worth reflects opening 100000 + income 50000 = 150000.
	rec = getAuth(t, router, "/api/v1/net-worth", "secret")
	var nw struct{ Total int64 `json:"total"` }
	json.Unmarshal(rec.Body.Bytes(), &nw)
	if nw.Total != 150000 {
		t.Errorf("net worth total = %d paise, want 150000", nw.Total)
	}

	// Delete the account → 204.
	if rec := deleteAuth(t, router, "/api/v1/accounts/"+acc.ID, "secret"); rec.Code != http.StatusNoContent {
		t.Errorf("delete: status %d, want 204", rec.Code)
	}
	_ = db
}

func TestAPIDocsAndSpec(t *testing.T) {
	router, _ := newTestServer(t) // no token needed for docs/spec
	if rec := get(t, router, "/api/openapi.json"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "\"openapi\"") {
		t.Errorf("openapi.json: status %d", rec.Code)
	}
	if rec := get(t, router, "/api/docs"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "swagger-ui") {
		t.Errorf("docs: status %d", rec.Code)
	}
}
