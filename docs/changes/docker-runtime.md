# Docker Runtime

Статус: actual

## Проблема

Проекту нужен runtime/setup по образцу PINGO: `Makefile`, Docker image, `docker-compose.yml`, `.env.example`, PostgreSQL и проверяемые миграции.

## As-is

Подтверждено:

- В `openclosed` уже есть Go-код бота и PostgreSQL-хранилище.
- В корне есть reference symlink `PINGO -> /home/boris/D/Dev/PINGO`.
- В PINGO используются `.env.example`, `docker-compose.yml`, `Makefile`, Dockerfile на сервис и `scripts/check_migrations.sh`.

## Target behavior

Подтверждено:

- `make test` проверяет миграции и запускает Go-тесты.
- `docker-compose.yml` поднимает `postgres`, одноразовый `migrate` и `bot`.
- `bot` запускается только после healthy Postgres и успешной миграции.
- `.env.example` содержит Telegram token, PostgreSQL env и Bot API настройки.
- Allowlist хранится в PostgreSQL, не в env.

## Границы

Делаем:

- Dockerfile для bot binary.
- Compose runtime для local/staging/prod project name.
- Makefile в стиле PINGO.
- `.env.example`, `.gitignore`, `.dockerignore`.
- `.up/.down.sql` миграции и проверочный скрипт.

Не делаем:

- Live-запуск Telegram bot.
- Деплой на сервер.
- Админ-панель для allowlist.
- Изменение guard-логики.

## DoD

- `make check-migrations` проходит.
- `make test` проходит.
- `docker compose config` проходит.
- `go test -race ./...` проходит.
- Документы описывают Docker runtime и owner-run команды.
