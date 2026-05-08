# openclosed

Telegram guard bot for public channels, groups and supergroups.

Target behavior:

- Non-admin members are removed without a permanent ban.
- Non-admin messages in groups and supergroups are deleted.
- Removal is persisted in PostgreSQL and retried after restart.

## Configuration

Required:

- `TELEGRAM_BOT_TOKEN` - Telegram bot token.
- `DATABASE_URL` - PostgreSQL DSN.

Optional:

- `OPENCLOSED_DATABASE_URL` - PostgreSQL DSN fallback when `DATABASE_URL` is empty.
- `OPENCLOSED_API_BASE` - Telegram Bot API base URL, default `https://api.telegram.org`.
- `OPENCLOSED_POLL_TIMEOUT_SECONDS` - long-polling timeout, default `30`.
- `OPENCLOSED_REMOVAL_INTERVAL_SECONDS` - removal queue interval, default `5`.

## Run

Apply the PostgreSQL schema first:

```bash
psql "$DATABASE_URL" -f migrations/001_guard_mode.sql
```

```bash
TELEGRAM_BOT_TOKEN=123:secret DATABASE_URL='postgres://user:pass@localhost:5432/openclosed?sslmode=disable' go run ./cmd/openclosed
```

The bot must be an administrator in protected chats with rights to restrict members and delete messages.

## Verify

```bash
go test ./...
go test -race ./...
```
