# openclosed

Telegram guard bot for public channels, groups and supergroups.

Target behavior:

- Non-admin members are removed without a permanent ban.
- Non-admin messages in groups and supergroups are deleted.
- Removal is persisted in PostgreSQL and retried after restart.

## Configuration

Copy env example:

```bash
cp .env.example .env
```

Required in `.env`:

- `TELEGRAM_BOT_TOKEN` - Telegram bot token.
- `DATABASE_URL` - PostgreSQL DSN.

Allowlist is stored in PostgreSQL, not env. Use `make allowlist-add chat_id=<id> user_id=<id>`.

## Run

Local Docker runtime:

```bash
make up
```

Manual Go run:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/001_guard_mode.up.sql
TELEGRAM_BOT_TOKEN=123:secret DATABASE_URL='postgres://user:pass@localhost:5432/openclosed?sslmode=disable' go run ./cmd/openclosed
```

The bot must be an administrator in protected chats with rights to restrict members and delete messages.

## Current Make Commands

```bash
make test
make check-migrations
make build
make up
make down
make logs ENV=local service=bot
make ps ENV=local
make backup ENV=prod
make allowlist-add chat_id=<id> user_id=<id>
make allowlist-list
```

Runtime commands that start services or touch data are owner-run unless explicitly confirmed in chat.
