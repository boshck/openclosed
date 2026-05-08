package guard

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"openclosed/internal/store"
	"openclosed/internal/telegram"
)

type BotAPI interface {
	GetChatMember(ctx context.Context, chatID int64, userID int64) (telegram.ChatMember, error)
	DeleteMessage(ctx context.Context, chatID int64, messageID int64) error
	UnbanChatMember(ctx context.Context, chatID int64, userID int64) error
}

type Repository interface {
	UpsertChat(ctx context.Context, chatID int64, chatType string, title string, username string) error
	IsGuardEnabled(ctx context.Context, chatID int64) (bool, error)
	IsAllowed(ctx context.Context, chatID int64, userID int64) (bool, error)
	UpsertKnownMember(ctx context.Context, member store.KnownMember) error
	EnqueueRemoval(ctx context.Context, chatID int64, userID int64, reason string, updateID *int64) error
	ListDueRemovals(ctx context.Context, limit int) ([]store.RemovalItem, error)
	MarkRemovalDone(ctx context.Context, chatID int64, userID int64) error
	MarkRemovalError(ctx context.Context, chatID int64, userID int64, attempts int, nextAttemptAt time.Time, lastError string) error
	RecordEvent(ctx context.Context, event store.Event) error
}

type Service struct {
	bot        BotAPI
	repo       Repository
	selfUserID int64
	logger     *slog.Logger
}

func NewService(bot BotAPI, repo Repository, selfUserID int64, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		bot:        bot,
		repo:       repo,
		selfUserID: selfUserID,
		logger:     logger,
	}
}

func (s *Service) HandleUpdate(ctx context.Context, update telegram.Update) error {
	if update.ChatMember != nil {
		return s.handleChatMember(ctx, update.UpdateID, *update.ChatMember)
	}
	if update.Message != nil {
		return s.handleMessage(ctx, update.UpdateID, *update.Message)
	}
	return nil
}

func (s *Service) handleChatMember(ctx context.Context, updateID int64, upd telegram.ChatMemberUpdated) error {
	chat := upd.Chat
	if err := s.repo.UpsertChat(ctx, chat.ID, chat.Type, chatTitle(chat), chat.Username); err != nil {
		return err
	}
	enabled, err := s.repo.IsGuardEnabled(ctx, chat.ID)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	member := upd.NewChatMember
	if err := s.repo.UpsertKnownMember(ctx, knownMember(chat.ID, member.User, member.Status)); err != nil {
		return err
	}
	if !member.IsActiveMember() || member.IsAdministrator() || member.User.ID == s.selfUserID {
		return s.repo.RecordEvent(ctx, store.Event{
			ChatID:    chat.ID,
			UserID:    member.User.ID,
			EventType: "chat_member_seen",
			UpdateID:  &updateID,
			Status:    member.Status,
		})
	}

	allowed, err := s.repo.IsAllowed(ctx, chat.ID, member.User.ID)
	if err != nil {
		return err
	}
	if allowed {
		return s.repo.RecordEvent(ctx, store.Event{
			ChatID:    chat.ID,
			UserID:    member.User.ID,
			EventType: "chat_member_allowed",
			UpdateID:  &updateID,
			Status:    "allowed",
		})
	}

	if err := s.repo.EnqueueRemoval(ctx, chat.ID, member.User.ID, "chat_member_joined", &updateID); err != nil {
		return err
	}
	return s.repo.RecordEvent(ctx, store.Event{
		ChatID:    chat.ID,
		UserID:    member.User.ID,
		EventType: "removal_enqueued",
		UpdateID:  &updateID,
		Status:    "pending",
	})
}

func (s *Service) handleMessage(ctx context.Context, updateID int64, msg telegram.Message) error {
	chat := msg.Chat
	if err := s.repo.UpsertChat(ctx, chat.ID, chat.Type, chatTitle(chat), chat.Username); err != nil {
		return err
	}
	enabled, err := s.repo.IsGuardEnabled(ctx, chat.ID)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	for _, user := range msg.NewChatMembers {
		if user.ID == s.selfUserID {
			continue
		}
		if err := s.repo.UpsertKnownMember(ctx, knownMember(chat.ID, user, "member")); err != nil {
			return err
		}
		if err := s.enqueueNewMemberIfRemovable(ctx, chat.ID, user.ID, &updateID); err != nil {
			return err
		}
	}

	if chat.Type != "group" && chat.Type != "supergroup" {
		return nil
	}
	if msg.From == nil || msg.From.ID == s.selfUserID {
		return nil
	}

	user := *msg.From
	if err := s.repo.UpsertKnownMember(ctx, knownMember(chat.ID, user, "message_seen")); err != nil {
		return err
	}
	allowed, err := s.repo.IsAllowed(ctx, chat.ID, user.ID)
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}

	member, err := s.bot.GetChatMember(ctx, chat.ID, user.ID)
	if err != nil {
		recordErr := s.repo.RecordEvent(ctx, store.Event{
			ChatID:    chat.ID,
			UserID:    user.ID,
			EventType: "admin_check_failed",
			UpdateID:  &updateID,
			MessageID: &msg.MessageID,
			Status:    "error",
			Error:     err.Error(),
		})
		if recordErr != nil {
			return recordErr
		}
		return fmt.Errorf("check chat member chat %d user %d: %w", chat.ID, user.ID, err)
	}
	if member.IsAdministrator() {
		return s.repo.RecordEvent(ctx, store.Event{
			ChatID:    chat.ID,
			UserID:    user.ID,
			EventType: "admin_message_kept",
			UpdateID:  &updateID,
			MessageID: &msg.MessageID,
			Status:    "kept",
		})
	}

	deleteErr := s.bot.DeleteMessage(ctx, chat.ID, msg.MessageID)
	event := store.Event{
		ChatID:    chat.ID,
		UserID:    user.ID,
		EventType: "message_delete",
		UpdateID:  &updateID,
		MessageID: &msg.MessageID,
		Status:    "done",
	}
	if deleteErr != nil {
		event.Status = "error"
		event.Error = deleteErr.Error()
	}
	if err := s.repo.RecordEvent(ctx, event); err != nil {
		return err
	}
	if deleteErr != nil {
		return fmt.Errorf("delete message chat %d message %d: %w", chat.ID, msg.MessageID, deleteErr)
	}

	return s.enqueueIfNotAllowed(ctx, chat.ID, user.ID, "message_from_non_admin", &updateID)
}

func (s *Service) RunRemovalOnce(ctx context.Context, limit int) (int, error) {
	items, err := s.repo.ListDueRemovals(ctx, limit)
	if err != nil {
		return 0, err
	}

	done := 0
	for _, item := range items {
		err := s.bot.UnbanChatMember(ctx, item.ChatID, item.UserID)
		if err != nil {
			attempts := item.Attempts + 1
			nextAttemptAt := time.Now().UTC().Add(removalBackoff(attempts))
			if markErr := s.repo.MarkRemovalError(ctx, item.ChatID, item.UserID, attempts, nextAttemptAt, err.Error()); markErr != nil {
				return done, markErr
			}
			_ = s.repo.RecordEvent(ctx, store.Event{
				ChatID:    item.ChatID,
				UserID:    item.UserID,
				EventType: "removal_failed",
				Status:    "error",
				Error:     err.Error(),
			})
			s.logger.Warn("removal failed", "chat_id", item.ChatID, "user_id", item.UserID, "attempts", attempts, "error", err)
			continue
		}

		if err := s.repo.MarkRemovalDone(ctx, item.ChatID, item.UserID); err != nil {
			return done, err
		}
		if err := s.repo.RecordEvent(ctx, store.Event{
			ChatID:    item.ChatID,
			UserID:    item.UserID,
			EventType: "removal_done",
			Status:    "done",
		}); err != nil {
			return done, err
		}
		done++
	}

	return done, nil
}

func (s *Service) enqueueIfNotAllowed(ctx context.Context, chatID int64, userID int64, reason string, updateID *int64) error {
	allowed, err := s.repo.IsAllowed(ctx, chatID, userID)
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}
	if err := s.repo.EnqueueRemoval(ctx, chatID, userID, reason, updateID); err != nil {
		return err
	}
	return s.repo.RecordEvent(ctx, store.Event{
		ChatID:    chatID,
		UserID:    userID,
		EventType: "removal_enqueued",
		UpdateID:  updateID,
		Status:    "pending",
	})
}

func (s *Service) enqueueNewMemberIfRemovable(ctx context.Context, chatID int64, userID int64, updateID *int64) error {
	allowed, err := s.repo.IsAllowed(ctx, chatID, userID)
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}

	member, err := s.bot.GetChatMember(ctx, chatID, userID)
	if err != nil {
		recordErr := s.repo.RecordEvent(ctx, store.Event{
			ChatID:    chatID,
			UserID:    userID,
			EventType: "new_member_admin_check_failed",
			UpdateID:  updateID,
			Status:    "error",
			Error:     err.Error(),
		})
		if recordErr != nil {
			return recordErr
		}
		return fmt.Errorf("check new member chat %d user %d: %w", chatID, userID, err)
	}
	if member.IsAdministrator() {
		return s.repo.RecordEvent(ctx, store.Event{
			ChatID:    chatID,
			UserID:    userID,
			EventType: "new_member_admin_kept",
			UpdateID:  updateID,
			Status:    "kept",
		})
	}

	return s.enqueueIfNotAllowed(ctx, chatID, userID, "new_chat_member_message", updateID)
}

func knownMember(chatID int64, user telegram.User, status string) store.KnownMember {
	return store.KnownMember{
		ChatID:    chatID,
		UserID:    user.ID,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		IsBot:     user.IsBot,
		Status:    status,
	}
}

func chatTitle(chat telegram.Chat) string {
	if chat.Title != "" {
		return chat.Title
	}
	return chat.FirstName
}

func removalBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Duration(attempts) * 10 * time.Second
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}
