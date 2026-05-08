# PROJECT

Статус: actual

## Подтверждено

- Назначение проекта: Telegram-бот guard-типа для каналов, групп и супергрупп.
- Решение владельца от 2026-05-08: бот не банит пользователей навсегда, а удаляет неразрешенных участников так, чтобы они могли снова открыть публичный чат или канал.
- Решение владельца от 2026-05-08: бот должен удалять сообщения, написанные не администраторами.
- Основной язык разработки: Go.
- Репозиторий на момент старта содержал только `AGENTS.md` и `README.md`; прикладной код отсутствовал.

## Гипотезы

- Runtime, вероятно, будет запускаться как отдельный long-polling процесс. Требуется проверка владельцем на целевом сервере.

## Открытые вопросы

- В корне нет reference symlink вида `refPROJECT`; назначение ref-ссылки не определено.
- Целевой runtime не зафиксирован: local, staging, prod или server-only.
- Не выбран процесс деплоя и мониторинга.

## Карта модулей

- `cmd/openclosed` - точка входа long-polling бота.
- `internal/telegram` - минимальный клиент Telegram Bot API и DTO для нужных update-типов.
- `internal/store` - PostgreSQL-хранилище состояния, очереди удаления и журнала модерации.
- `internal/guard` - бизнес-логика guard-mode: обработка вступлений, сообщений и очереди удаления.
- `docs/changes/guard-mode.md` - change-doc по текущему поведению.

## Актуальные документы

- `docs/changes/guard-mode.md` - actual, целевое поведение guard-mode.
- `AGENTS.md` - actual, общие правила работы.
- `README.md` - actual, пользовательские команды запуска и проверки.

## Команды разработки и проверки

- `go test ./...` - базовая проверка.
- `go test -race ./...` - race-проверка, если окружение позволяет.
- `gofmt -w <files>` - форматирование Go-файлов после подтвержденных правок.

## Runtime policy

Статус: draft.

- Локальный запуск бота требует `TELEGRAM_BOT_TOKEN`.
- Локальные runtime-команды не считаются доказательством состояния рабочей системы.
- Live/API-проверки Telegram выполнять только после отдельного подтверждения владельца и с токеном владельца.
- Server-only workflow и файл `comm` не зафиксированы.

## Конфигурация и секреты

- `TELEGRAM_BOT_TOKEN` - токен бота, секрет, не логировать.
- `DATABASE_URL` или `OPENCLOSED_DATABASE_URL` - PostgreSQL DSN, секреты из DSN не логировать.
- `OPENCLOSED_API_BASE` - base URL Telegram Bot API, по умолчанию `https://api.telegram.org`.
- `OPENCLOSED_POLL_TIMEOUT_SECONDS` - timeout long polling, по умолчанию `30`.
- `OPENCLOSED_REMOVAL_INTERVAL_SECONDS` - период обработки очереди удаления, по умолчанию `5`.

## Generated/managed files

- `migrations/001_guard_mode.sql` - PostgreSQL-схема guard-mode.
- `go.sum` управляется Go toolchain.

## Что не редактировать вручную

- Данные PostgreSQL без отдельного решения владельца.
- Секреты и токены в репозитории.
