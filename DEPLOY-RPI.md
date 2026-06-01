# Deploy on Raspberry Pi

Single self-contained binary in a tiny container. Pure-Go SQLite (no CGO), all
assets embedded. The host keeps the database in `./data`.

## Which arch?
- Pi 3 / 4 / 5 on 64-bit Raspberry Pi OS → **arm64** (recommended).
- Pi 2 / 3 / Zero 2 on 32-bit OS → **arm/v7**.
- Check with `uname -m`: `aarch64` = arm64, `armv7l` = arm/v7.

## Option A — build on the Pi (simplest)
On the Pi (Docker + compose plugin installed):

```sh
# copy/clone the finance-tracker folder to the Pi, then:
cd finance-tracker
docker compose up -d --build
```

Open `http://<pi-ip>:8080`. First run: set `SEED_DEMO: "true"` once to load demo
data, then back to `"false"`.

Logs / lifecycle:
```sh
docker compose logs -f
docker compose restart
docker compose down
```

## Option B — cross-build on a laptop, run on the Pi
Needs Docker buildx (bundled with Docker Desktop).

```sh
# arm64 Pi:
docker buildx build --platform linux/arm64 -t finance-tracker:arm64 --load .
# or 32-bit Pi:
docker buildx build --platform linux/arm/v7 -t finance-tracker:armv7 --load .

# ship the image to the Pi without a registry:
docker save finance-tracker:arm64 | ssh pi@<pi-ip> docker load
```

Then on the Pi, run it (compose or plain docker):
```sh
docker run -d --name finance-tracker --restart unless-stopped \
  -p 8080:8080 -v "$PWD/data:/data" \
  -e DB_PATH=/data/finance.db \
  finance-tracker:arm64
```

Or push to a registry and `docker compose pull` on the Pi instead of building.

## Telegram add-from-chat (optional)
Set in the Pi's environment (or a `.env` next to `docker-compose.yml`):
```
TELEGRAM_BOT_TOKEN=123:abc       # from @BotFather (use a bot nothing else polls)
TELEGRAM_ALLOWED_CHAT=12345678   # your chat id, to restrict who can add
```
The poll offset persists in `/data/telegram_offset`, so restarts don't replay.

## Gmail credit-card import (optional)
Auto-imports card spend alerts as transactions, deduped by email Message-ID,
parsed heuristically (amount + merchant; `Info:`/UPI payee and `at MERCHANT`).

1. On the Google account: enable **2-Step Verification**, then create an
   **App Password** (Security → App passwords).
2. Set in the Pi env / `.env`:
   ```
   GMAIL_IMAP_USER=you@gmail.com
   GMAIL_IMAP_PASSWORD=<16-char app password>
   # optional: GMAIL_SYNC_INTERVAL=6h  GMAIL_LOOKBACK_DAYS=7  GMAIL_CC_SENDERS=...
   ```
3. Restart. It runs ~30s after boot, then every `GMAIL_SYNC_INTERVAL`.

Trigger manually:
- UI: Settings → **Sync Gmail now**.
- API (for cron): `curl -X POST http://<pi>:8080/api/gmail/sync -H 'Authorization: Bearer $API_PUSH_TOKEN'`
  → `{"scanned":N,"imported":M,"skipped":K}`.

Heuristic parser — tune `GMAIL_CC_SENDERS` to your banks; imported rows land as
`Credit Card` expenses under `GMAIL_DEFAULT_CATEGORY`, re-categorize in the UI.

## Notes
- Data lives in `./data/finance.db` on the host — back it up, or use the in-app
  Settings → Export JSON.
- Image is `distroless/static` (no shell); manage it via Docker, not by exec-ing in.
- To put it behind HTTPS, front it with Caddy/Nginx or a Cloudflare Tunnel.
