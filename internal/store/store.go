package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type KnownMember struct {
	ChatID    int64
	UserID    int64
	Username  string
	FirstName string
	LastName  string
	IsBot     bool
	Status    string
}

type RemovalItem struct {
	ChatID   int64
	UserID   int64
	Reason   string
	Attempts int
}

type Event struct {
	ChatID    int64
	UserID    int64
	EventType string
	UpdateID  *int64
	MessageID *int64
	Status    string
	Error     string
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres database url: %w", err)
	}
	cfg.MaxConns = 5
	cfg.MinConns = 0
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) UpsertChat(ctx context.Context, chatID int64, chatType string, title string, username string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO chats (chat_id, chat_type, title, username, first_seen_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT(chat_id) DO UPDATE SET
			chat_type = excluded.chat_type,
			title = excluded.title,
			username = excluded.username,
			updated_at = excluded.updated_at
	`, chatID, chatType, title, username, now, now)
	if err != nil {
		return fmt.Errorf("upsert chat %d: %w", chatID, err)
	}
	return nil
}

func (s *Store) IsGuardEnabled(ctx context.Context, chatID int64) (bool, error) {
	var enabled bool
	err := s.pool.QueryRow(ctx, `SELECT guard_enabled FROM chats WHERE chat_id = $1`, chatID).Scan(&enabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, fmt.Errorf("query guard flag for chat %d: %w", chatID, err)
	}
	return enabled, nil
}

func (s *Store) IsAllowed(ctx context.Context, chatID int64, userID int64) (bool, error) {
	var exists int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM allowlist WHERE chat_id = $1 AND user_id = $2`, chatID, userID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("query allowlist for chat %d user %d: %w", chatID, userID, err)
	}
	return true, nil
}

func (s *Store) UpsertKnownMember(ctx context.Context, member KnownMember) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO known_members (
			chat_id, user_id, username, first_name, last_name, is_bot, last_status, first_seen_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(chat_id, user_id) DO UPDATE SET
			username = excluded.username,
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			is_bot = excluded.is_bot,
			last_status = excluded.last_status,
			updated_at = excluded.updated_at
	`, member.ChatID, member.UserID, member.Username, member.FirstName, member.LastName, member.IsBot, member.Status, now, now)
	if err != nil {
		return fmt.Errorf("upsert known member chat %d user %d: %w", member.ChatID, member.UserID, err)
	}
	return nil
}

func (s *Store) EnqueueRemoval(ctx context.Context, chatID int64, userID int64, reason string, updateID *int64) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO removal_queue (
			chat_id, user_id, status, reason, source_update_id, attempts, next_attempt_at, created_at, updated_at
		)
		VALUES ($1, $2, 'pending', $3, $4, 0, $5, $6, $7)
		ON CONFLICT(chat_id, user_id) DO UPDATE SET
			status = 'pending',
			reason = excluded.reason,
			source_update_id = COALESCE(excluded.source_update_id, removal_queue.source_update_id),
			next_attempt_at = excluded.next_attempt_at,
			last_error = '',
			attempts = CASE WHEN removal_queue.status = 'done' THEN 0 ELSE removal_queue.attempts END,
			updated_at = excluded.updated_at
	`, chatID, userID, reason, nullableInt64(updateID), now, now, now)
	if err != nil {
		return fmt.Errorf("enqueue removal chat %d user %d: %w", chatID, userID, err)
	}
	return nil
}

func (s *Store) ListDueRemovals(ctx context.Context, limit int) ([]RemovalItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT chat_id, user_id, reason, attempts
		FROM removal_queue
		WHERE status IN ('pending', 'error') AND next_attempt_at <= $1
		ORDER BY updated_at ASC
		LIMIT $2
	`, time.Now().UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("query due removals: %w", err)
	}
	defer rows.Close()

	var items []RemovalItem
	for rows.Next() {
		var item RemovalItem
		if err := rows.Scan(&item.ChatID, &item.UserID, &item.Reason, &item.Attempts); err != nil {
			return nil, fmt.Errorf("scan due removal: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due removals: %w", err)
	}
	return items, nil
}

func (s *Store) MarkRemovalDone(ctx context.Context, chatID int64, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE removal_queue
		SET status = 'done', last_error = '', updated_at = $1
		WHERE chat_id = $2 AND user_id = $3
	`, time.Now().UTC(), chatID, userID)
	if err != nil {
		return fmt.Errorf("mark removal done chat %d user %d: %w", chatID, userID, err)
	}
	return nil
}

func (s *Store) MarkRemovalError(ctx context.Context, chatID int64, userID int64, attempts int, nextAttemptAt time.Time, lastError string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE removal_queue
		SET status = 'error', attempts = $1, next_attempt_at = $2, last_error = $3, updated_at = $4
		WHERE chat_id = $5 AND user_id = $6
	`, attempts, nextAttemptAt.UTC(), lastError, time.Now().UTC(), chatID, userID)
	if err != nil {
		return fmt.Errorf("mark removal error chat %d user %d: %w", chatID, userID, err)
	}
	return nil
}

func (s *Store) RecordEvent(ctx context.Context, event Event) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO moderation_events (
			created_at, chat_id, user_id, event_type, update_id, message_id, status, error
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, time.Now().UTC(), event.ChatID, event.UserID, event.EventType, nullableInt64(event.UpdateID), nullableInt64(event.MessageID), event.Status, event.Error)
	if err != nil {
		return fmt.Errorf("record moderation event %s chat %d user %d: %w", event.EventType, event.ChatID, event.UserID, err)
	}
	return nil
}

func (s *Store) LoadUpdateOffset(ctx context.Context) (int64, bool, error) {
	var value string
	err := s.pool.QueryRow(ctx, `SELECT value FROM bot_state WHERE key = 'update_offset'`).Scan(&value)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("load update offset: %w", err)
	}
	offset, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse update offset %q: %w", value, err)
	}
	return offset, true, nil
}

func (s *Store) SaveUpdateOffset(ctx context.Context, offset int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bot_state (key, value, updated_at)
		VALUES ('update_offset', $1, $2)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, strconv.FormatInt(offset, 10), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save update offset %d: %w", offset, err)
	}
	return nil
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
