#!/usr/bin/env bash
#
# add-transaction.sh — push a transaction to Finance Tracker's JSON API.
#
#   POST /api/transactions  (Bearer $API_PUSH_TOKEN)
#
# Env:
#   FT_HOST          base URL          (default: http://localhost:8080)
#   API_PUSH_TOKEN   bearer token      (required)
#
set -euo pipefail

HOST="${FT_HOST:-http://localhost:8080}"
TOKEN="${API_PUSH_TOKEN:-}"

# Defaults (server applies its own if these are empty, but we send explicit ones).
TYPE="Expense"
METHOD="UPI"
CATEGORY=""
DATE=""
NOTES=""
TAGS=""
CARD_ID=""
AMOUNT=""
DESC=""

usage() {
  cat <<'EOF'
Add a transaction to Finance Tracker.

Usage:
  add-transaction.sh -a AMOUNT -d DESCRIPTION [options]

Required:
  -a AMOUNT        amount in rupees (e.g. 250 or 1234.50)
  -d DESCRIPTION   description text

Options:
  -t TYPE          Income | Expense | Transfer            (default: Expense)
  -m METHOD        UPI | Credit Card | Debit Card | Cash |
                   Bank Transfer | NetBanking | Cheque | Other  (default: UPI)
  -c CATEGORY      category name (must already exist)      (default: server's Miscellaneous)
  -D DATE          YYYY-MM-DD                              (default: today)
  -n NOTES         free-text note
  -g TAGS          comma-separated tags (e.g. work,reimbursable)
  -k CARD_ID       link to a credit card by id (only used when method is "Credit Card")
  -h               show this help

Environment:
  FT_HOST          base URL        (default: http://localhost:8080)
  API_PUSH_TOKEN   bearer token    (required)

Examples:
  export API_PUSH_TOKEN=xxxxxxxx
  export FT_HOST=http://192.168.0.100:8080

  add-transaction.sh -a 250 -d "Coffee" -c "Food & Dining"
  add-transaction.sh -a 85000 -d "Salary" -t Income -c "Salary" -m "Bank Transfer"
  add-transaction.sh -a 1299 -d "Amazon" -m "Credit Card" -k <card-id> -g "shopping"
EOF
}

while getopts ":a:d:t:m:c:D:n:g:k:h" opt; do
  case "$opt" in
    a) AMOUNT="$OPTARG" ;;
    d) DESC="$OPTARG" ;;
    t) TYPE="$OPTARG" ;;
    m) METHOD="$OPTARG" ;;
    c) CATEGORY="$OPTARG" ;;
    D) DATE="$OPTARG" ;;
    n) NOTES="$OPTARG" ;;
    g) TAGS="$OPTARG" ;;
    k) CARD_ID="$OPTARG" ;;
    h) usage; exit 0 ;;
    \?) echo "unknown option: -$OPTARG" >&2; usage; exit 2 ;;
    :)  echo "option -$OPTARG needs a value" >&2; exit 2 ;;
  esac
done

# --- validation ---
[ -n "$TOKEN" ] || { echo "error: API_PUSH_TOKEN is not set" >&2; exit 2; }
[ -n "$AMOUNT" ] || { echo "error: -a AMOUNT is required" >&2; usage; exit 2; }
[ -n "$DESC" ]   || { echo "error: -d DESCRIPTION is required" >&2; usage; exit 2; }
case "$AMOUNT" in
  ''|*[!0-9.]*) echo "error: amount must be a number (rupees), got '$AMOUNT'" >&2; exit 2 ;;
esac

# JSON string escaping (handles backslash and double-quote).
json_str() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }

# Build a JSON array from a comma-separated list: "a,b" -> ["a","b"]
json_tags() {
  local IFS=','; local out="" first=1 t
  for t in $1; do
    t="$(printf '%s' "$t" | sed -e 's/^ *//' -e 's/ *$//')"
    [ -n "$t" ] || continue
    [ $first -eq 1 ] && first=0 || out="$out,"
    out="$out\"$(json_str "$t")\""
  done
  printf '[%s]' "$out"
}

# Assemble the JSON payload.
payload="{\"amount\":$AMOUNT"
payload="$payload,\"description\":\"$(json_str "$DESC")\""
payload="$payload,\"type\":\"$(json_str "$TYPE")\""
payload="$payload,\"payment_method\":\"$(json_str "$METHOD")\""
[ -n "$CATEGORY" ] && payload="$payload,\"category\":\"$(json_str "$CATEGORY")\""
[ -n "$DATE" ]     && payload="$payload,\"date\":\"$(json_str "$DATE")\""
[ -n "$NOTES" ]    && payload="$payload,\"notes\":\"$(json_str "$NOTES")\""
[ -n "$TAGS" ]     && payload="$payload,\"tags\":$(json_tags "$TAGS")"
[ -n "$CARD_ID" ]  && payload="$payload,\"card_id\":\"$(json_str "$CARD_ID")\""
payload="$payload}"

# Send it. Capture body + HTTP status, exit non-zero on >=400.
resp="$(curl -sS -w $'\n%{http_code}' -X POST "$HOST/api/transactions" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$payload")"
code="${resp##*$'\n'}"
body="${resp%$'\n'*}"

echo "$body"
if [ "$code" -ge 400 ]; then
  echo "request failed (HTTP $code)" >&2
  exit 1
fi
