# Finance Tracker

A self-hosted personal finance tracker for India — one Go binary, no external
services required. Track income and spending, recurring bills, savings goals,
budgets, and credit-card billing cycles; add transactions from a web UI, a
Telegram bot, or by auto-importing credit-card alert emails from Gmail.

Built as a single statically-linked binary: the web templates, JavaScript,
CSS, database migrations, and a pure-Go SQLite engine are all embedded. Copy
one file to a server (or a Raspberry Pi) and run it.

---

## Highlights

- **Server-rendered, fast, offline-capable** — html/template + [htmx](https://htmx.org) +
  [Alpine.js](https://alpinejs.dev) + [Chart.js](https://www.chartjs.org), all vendored. No CDN, no build step to run it.
- **Exact money** — every amount is stored as an `int64` number of paise, so there
  is no floating-point drift. Displayed in Indian format (`₹1,23,456.78`).
- **One file** — pure-Go SQLite (`modernc.org/sqlite`), no CGO, no system libraries.
- **Three ways to add a transaction** — the web UI, a Telegram bot, or Gmail import.

---

## Features

### Dashboard
- KPI strip: this month's credits, debits, net, year-to-date spend, savings rate.
- Pay-cycle view (treats salary day as the start of your spending month).
- 24-month charts: income vs expense, cumulative net, running buffer, category mix.
- Recent activity feed.

### Transactions
- Full create / edit / delete, with filtering (month, category, type, method, text
  search), sortable columns, and pagination.
- Tags (comma-separated, filterable), duplicate, and bulk actions (delete /
  re-categorise).
- **CSV import** with automatic column detection (Date / Narration, or Amount, or
  separate Debit / Credit columns; flexible date formats) and **CSV export**.

### Recurring items
- Define repeating income/expenses (monthly, quarterly, half-yearly, yearly,
  weekly, one-time) with start/end dates.
- "Post due" fires the transactions due this month, idempotently (never posts the
  same item twice for the same month).

### Planning
- **Savings goals** with target, deadline, progress bar, and a quick "contribute"
  action; shows the monthly amount needed to hit the deadline.
- **Budgets** — per-category monthly limits with an overspend warning.

### Accounts & net worth
- Define bank/cash/wallet accounts with opening balances; optionally link each
  transaction to an account to track its running balance.
- **Net worth** = account balances + investment value − credit-card outstanding.

### Investment portfolio
- Track holdings (mutual funds, stocks, ETFs, gold, …) with units and average
  cost; per-holding value / P&L / return and an asset-type allocation view.
- **Auto NAV**: mutual funds with an AMFI scheme code refresh from AMFI's daily
  file (free, no key) on a schedule or via the "Sync NAVs" button.

### Cashflow forecast & proactive alerts
- Dashboard **forecast** projects month-end net from actual + still-due recurring.
- **Alerts** (over-budget, card due/overdue, recurring due) via Telegram, on a
  schedule (`ALERTS_INTERVAL`) or `POST /api/alerts/run`.

### Reimbursements
- Tag an expense `reimbursable`; the **Reimbursements** page tracks the
  outstanding total and lets you mark items settled.

### Credit cards & billing cycles
- Define multiple cards (name, last-4, credit limit, statement day, due-offset days).
- Per-card view: current cycle's accrued spend, the last closed statement amount and
  due date, **Paid / Unpaid / Overdue** status, **outstanding balance** (purchases −
  payments) and **available credit**.
- **Mark paid** records a `Transfer` transaction (so it doesn't distort income/expense
  totals) and clears the statement — idempotent per statement.
- Credit-card purchases reduce available credit immediately. Gmail-imported spends are
  auto-linked to the matching card by last-4.

### Monthly & yearly views
- Monthly breakdown with a category doughnut.
- Year view: an income → category "sankey"-style flow with a year picker.

### Settings & data
- Manage categories (add, rename — cascades to all rows, delete — with reassignment).
- Dark mode, fiscal-year start.
- **Backup / restore** (JSON export & atomic import) and **reset to defaults**.

### Integrations
- **Telegram bot** — add transactions by chatting (`250 Coffee cat:Food method:upi`).
- **Gmail credit-card import** — a scheduled job reads bank spend-alert emails over
  IMAP and records them as expenses, de-duplicated by email Message-ID.
- **Outbound push API** — have the bot send you a message from a script/cron.

---

## Running it

### Locally (development)
```sh
make run            # go run ./cmd/server  → http://localhost:8080
make test           # go test ./...
make build          # single binary: ./finance-tracker
make css            # rebuild the purged Tailwind CSS (only if you change templates)
```

### Configuration (environment variables)
Set these in the environment or a `.env` file next to the binary.

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP port |
| `DB_PATH` | `./finance.db` | SQLite file location |
| `SEED_DEMO` | `false` | `true` loads demo data on first run (only when DB is empty) |
| `API_PUSH_TOKEN` | — | Bearer token gating the `/api/*` endpoints |
| `RECURRING_AUTOFIRE` | `false` | `true` posts the month's due recurring items on startup |
| `ALERTS_INTERVAL` | — | e.g. `24h` — schedule proactive Telegram alerts (needs the bot) |
| `NAV_SYNC_INTERVAL` | `12h` | how often to refresh mutual-fund NAVs from AMFI |
| `TELEGRAM_BOT_TOKEN` | — | Enables the Telegram bot (from @BotFather) |
| `TELEGRAM_ALLOWED_CHAT` | — | Restrict adds to one chat id; also the default push target |
| `GMAIL_IMAP_USER` | — | Gmail address (enables Gmail import) |
| `GMAIL_IMAP_PASSWORD` | — | Gmail **App Password** (not your login password) |
| `GMAIL_SYNC_INTERVAL` | `6h` | How often to poll Gmail (Go duration) |
| `GMAIL_LOOKBACK_DAYS` | `7` | How far back to search each run |
| `GMAIL_DEFAULT_CATEGORY` | `Miscellaneous` | Category for imported spends |
| `GMAIL_CC_SENDERS` | built-in list | Comma-separated From-address substrings (HDFC/ICICI/Axis/SBI/Kotak/AU by default) |
| `GMAIL_IMAP_HOST` | `imap.gmail.com:993` | IMAP host |
| `GMAIL_MAILBOX` | `INBOX` | Mailbox to scan |

### Deploy (Raspberry Pi / any Linux)
The binary is self-contained, so the simplest deploy is **cross-compile → copy → run
as a systemd service**:

```sh
# On your machine (arm64 Pi shown; use GOARCH=arm for 32-bit):
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
  -o finance-tracker ./cmd/server

# Copy to the Pi, then create /etc/systemd/system/finance-tracker.service:
#   ExecStart=/home/pi/finance-tracker/finance-tracker
#   EnvironmentFile=/home/pi/finance-tracker/.env
#   Restart=on-failure
sudo systemctl enable --now finance-tracker
```

Docker is also supported (`docker compose up -d --build`). See `DEPLOY-RPI.md` for the
full walkthrough including app-password setup.

Data lives in `DB_PATH` — back it up, or use **Settings → Export JSON**.

---

## HTTP API

The web UI runs on same-origin routes. Two endpoints are meant for scripts/cron and are
**protected by `API_PUSH_TOKEN`** (sent as `Authorization: Bearer <token>` or the
`X-Api-Token` header).

### Trigger a Gmail import
```sh
curl -X POST http://<host>:8080/api/gmail/sync \
  -H "Authorization: Bearer $API_PUSH_TOKEN"
# → {"scanned":N,"imported":M,"skipped":K}
```

### Send yourself a Telegram message
```sh
curl -X POST http://<host>:8080/api/push \
  -H "Authorization: Bearer $API_PUSH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text":"deploy finished ✅"}'
# chat_id is optional when TELEGRAM_ALLOWED_CHAT is set
```

### Add a transaction (token-protected JSON API)
```sh
curl -X POST http://<host>:8080/api/transactions \
  -H "Authorization: Bearer $API_PUSH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"amount":250,"description":"Coffee","category":"Miscellaneous"}'
# → 201 {"id":"…","date":"2026-06-01","amount":250,"amount_paise":25000,
#         "type":"Expense","payment_method":"UPI","category":"Miscellaneous"}
```
Body fields — `amount` (rupees, **required**), `description` (**required**), and
optional `date` (YYYY-MM-DD, default today), `type` (Income/Expense/Transfer, default
Expense), `payment_method` (default UPI), `category` (must exist, default
Miscellaneous), `notes`, `tags` (array), `card_id` (used when method is Credit Card).

There's also a small wrapper script — [`scripts/add-transaction.sh`](scripts/add-transaction.sh):
```sh
export API_PUSH_TOKEN=xxxx FT_HOST=http://<host>:8080
scripts/add-transaction.sh -a 250 -d "Coffee" -c "Food & Dining"
scripts/add-transaction.sh -a 85000 -d "Salary" -t Income -c "Salary" -m "Bank Transfer"
scripts/add-transaction.sh -h     # full usage
```

The UI's form route `POST /transactions` (form-encoded, **not** token-protected, returns
an HTML fragment) also exists for the browser; prefer the JSON API above for scripting.

### Other token-protected endpoints
- `POST /api/alerts/run` → run the alert check (sends a Telegram message if any).
- `POST /api/nav/sync` → refresh mutual-fund NAVs from AMFI.

### Full REST API + Swagger UI
A complete versioned JSON API lives under **`/api/v1`** (token-gated) with
list/get/create/update/delete for transactions, accounts, cards, holdings,
recurring, goals, budgets, and categories, plus `/net-worth` and the action
endpoints. **All monetary fields are integer paise.**

- **Swagger UI:** `GET /api/docs` (assets vendored — works offline)
- **OpenAPI spec:** `GET /api/openapi.json`

```sh
curl http://<host>:8080/api/v1/accounts -H "Authorization: Bearer $API_PUSH_TOKEN"
curl -X POST http://<host>:8080/api/v1/transactions \
  -H "Authorization: Bearer $API_PUSH_TOKEN" -H "Content-Type: application/json" \
  -d '{"amount":25000,"description":"Coffee","type":"Expense"}'   # 25000 paise = ₹250
```

### Health check
`GET /healthz` → `200 {"status":"ok"}` when the database is reachable.

---

## Telegram bot

Set `TELEGRAM_BOT_TOKEN` (and optionally `TELEGRAM_ALLOWED_CHAT` to lock it to your
chat). Then message the bot:

```
250 Coffee
1,234.50 Groceries cat:Groceries method:upi
85000 Salary type:income cat:Salary
```
Keys (all optional): `cat:` `method:` `type:` `date:YYYY-MM-DD`.
Defaults: type=expense, method=UPI, category=Miscellaneous, date=today.
Commands: `/balance`, `/help`.

---

## Gmail credit-card import

1. On the Google account, enable **2-Step Verification**, then create an **App
   Password** (Account → Security → App passwords).
2. Set `GMAIL_IMAP_USER` and `GMAIL_IMAP_PASSWORD` (the 16-char app password), restart.
3. It runs shortly after boot, then every `GMAIL_SYNC_INTERVAL`. Trigger manually from
   **Settings → Sync Gmail now** or `POST /api/gmail/sync`.

It parses real bank spend alerts (amount + merchant + card last-4), records each as a
**Credit Card expense** under `GMAIL_DEFAULT_CATEGORY`, links it to the matching card,
and skips OTPs, payment confirmations, statements, and refunds. De-duplication is by
email Message-ID, so re-runs never double-add. Parsing is heuristic and per-bank — tune
`GMAIL_CC_SENDERS` to your banks and re-categorise imported rows in the UI as needed.

---

## How it's built

```
cmd/server         startup: config → migrate → seed → HTTP server + bot + Gmail ticker
internal/domain    core types (Money paise, Transaction, RecurringItem, Card, …) — no I/O
internal/store     SQLite persistence (single connection, foreign keys on, migrations)
internal/service   business logic & view models (aggregations, billing cycles) — pure
internal/web       chi router, html/template rendering, htmx fragments, handlers
internal/telegrambot   Telegram Bot API client + message parser
internal/gmailsync     IMAP fetcher + heuristic email parser + idempotent importer
```

- **Single SQLite connection** (`MaxOpenConns=1`) — built for one user; safe and simple.
- **htmx fragments + `HX-Trigger`** — mutations return a small HTML fragment and a
  trigger header; dependent sections (KPIs, charts) refresh themselves.
- **Tested** — `go test ./...` covers money rounding/formatting, aggregations, billing
  cycles, store CRUD, importers, and HTTP handlers; runs clean under `-race` and `go vet`.

---

## Security notes

- Designed for **single-user, trusted-network** use (home LAN / VPN / Cloudflare Tunnel).
- `/api/*` endpoints require the bearer token; the HTML UI routes do not (they assume
  same-origin access). Don't expose the UI to the public internet without a reverse
  proxy and authentication in front.
- Gmail uses a scoped **App Password**, never your account password.
