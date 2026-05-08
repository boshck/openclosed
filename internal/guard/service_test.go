package guard

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"openclosed/internal/store"
	"openclosed/internal/telegram"
)

func TestService_HandleChatMember(t *testing.T) {
	tests := []struct {
		name        string
		member      telegram.ChatMember
		allowlisted bool
		wantQueued  bool
	}{
		{
			name: "regular member is queued",
			member: telegram.ChatMember{
				Status: "member",
				User:   telegram.User{ID: 100, FirstName: "User"},
			},
			wantQueued: true,
		},
		{
			name: "administrator is kept",
			member: telegram.ChatMember{
				Status: "administrator",
				User:   telegram.User{ID: 101, FirstName: "Admin"},
			},
			wantQueued: false,
		},
		{
			name: "allowlisted member is kept",
			member: telegram.ChatMember{
				Status: "member",
				User:   telegram.User{ID: 102, FirstName: "Allowed"},
			},
			allowlisted: true,
			wantQueued:  false,
		},
		{
			name: "left user is ignored",
			member: telegram.ChatMember{
				Status: "left",
				User:   telegram.User{ID: 103, FirstName: "Gone"},
			},
			wantQueued: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			if tt.allowlisted {
				repo.allowlisted[allowKey{-100, tt.member.User.ID}] = true
			}
			svc := NewService(&fakeBot{}, repo, 999, discardLogger())

			err := svc.HandleUpdate(context.Background(), telegram.Update{
				UpdateID: 1,
				ChatMember: &telegram.ChatMemberUpdated{
					Chat:          telegram.Chat{ID: -100, Type: "supergroup", Title: "test"},
					NewChatMember: tt.member,
				},
			})
			if err != nil {
				t.Fatalf("HandleUpdate() error = %v", err)
			}

			gotQueued := repo.enqueued[allowKey{-100, tt.member.User.ID}]
			if gotQueued != tt.wantQueued {
				t.Fatalf("queued = %v, want %v", gotQueued, tt.wantQueued)
			}
		})
	}
}

func TestService_HandleMessage(t *testing.T) {
	tests := []struct {
		name          string
		from          *telegram.User
		memberStatus  telegram.ChatMember
		wantDelete    bool
		wantQueued    bool
		wantMemberErr error
	}{
		{
			name: "non admin message is deleted and author queued",
			from: &telegram.User{ID: 200, FirstName: "User"},
			memberStatus: telegram.ChatMember{
				Status: "member",
				User:   telegram.User{ID: 200},
			},
			wantDelete: true,
			wantQueued: true,
		},
		{
			name: "admin message is kept",
			from: &telegram.User{ID: 201, FirstName: "Admin"},
			memberStatus: telegram.ChatMember{
				Status: "administrator",
				User:   telegram.User{ID: 201},
			},
			wantDelete: false,
			wantQueued: false,
		},
		{
			name:       "message without from is ignored",
			from:       nil,
			wantDelete: false,
			wantQueued: false,
		},
		{
			name:          "admin check error does not delete",
			from:          &telegram.User{ID: 202, FirstName: "Unknown"},
			wantMemberErr: errors.New("api unavailable"),
			wantDelete:    false,
			wantQueued:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			bot := &fakeBot{
				members: map[allowKey]telegram.ChatMember{
					{-200, userID(tt.from)}: tt.memberStatus,
				},
				getMemberErr: tt.wantMemberErr,
			}
			svc := NewService(bot, repo, 999, discardLogger())

			err := svc.HandleUpdate(context.Background(), telegram.Update{
				UpdateID: 2,
				Message: &telegram.Message{
					MessageID: 10,
					From:      tt.from,
					Chat:      telegram.Chat{ID: -200, Type: "supergroup", Title: "test"},
				},
			})
			if tt.wantMemberErr != nil {
				if err == nil {
					t.Fatalf("HandleUpdate() error = nil, want error")
				}
			} else if err != nil {
				t.Fatalf("HandleUpdate() error = %v", err)
			}

			if got := len(bot.deletedMessages) > 0; got != tt.wantDelete {
				t.Fatalf("deleted = %v, want %v", got, tt.wantDelete)
			}
			if tt.from != nil {
				gotQueued := repo.enqueued[allowKey{-200, tt.from.ID}]
				if gotQueued != tt.wantQueued {
					t.Fatalf("queued = %v, want %v", gotQueued, tt.wantQueued)
				}
			}
		})
	}
}

func TestService_HandleMessage_NewChatMembers(t *testing.T) {
	tests := []struct {
		name         string
		memberStatus string
		wantQueued   bool
	}{
		{
			name:         "new regular member is queued",
			memberStatus: "member",
			wantQueued:   true,
		},
		{
			name:         "new admin member is kept",
			memberStatus: "administrator",
			wantQueued:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			bot := &fakeBot{
				members: map[allowKey]telegram.ChatMember{
					{-300, 301}: {
						Status: tt.memberStatus,
						User:   telegram.User{ID: 301},
					},
				},
			}
			svc := NewService(bot, repo, 999, discardLogger())

			err := svc.HandleUpdate(context.Background(), telegram.Update{
				UpdateID: 3,
				Message: &telegram.Message{
					MessageID:      11,
					Chat:           telegram.Chat{ID: -300, Type: "supergroup", Title: "test"},
					NewChatMembers: []telegram.User{{ID: 301, FirstName: "Joined"}},
				},
			})
			if err != nil {
				t.Fatalf("HandleUpdate() error = %v", err)
			}

			gotQueued := repo.enqueued[allowKey{-300, 301}]
			if gotQueued != tt.wantQueued {
				t.Fatalf("queued = %v, want %v", gotQueued, tt.wantQueued)
			}
		})
	}
}

func TestService_RunRemovalOnce(t *testing.T) {
	repo := newFakeRepo()
	repo.removals = []store.RemovalItem{
		{ChatID: -100, UserID: 300, Reason: "test", Attempts: 0},
	}
	bot := &fakeBot{}
	svc := NewService(bot, repo, 999, discardLogger())

	done, err := svc.RunRemovalOnce(context.Background(), 100)
	if err != nil {
		t.Fatalf("RunRemovalOnce() error = %v", err)
	}
	if done != 1 {
		t.Fatalf("done = %d, want 1", done)
	}
	if got := bot.unbanned[allowKey{-100, 300}]; !got {
		t.Fatalf("user was not removed through unbanChatMember")
	}
	if got := repo.done[allowKey{-100, 300}]; !got {
		t.Fatalf("removal was not marked done")
	}
}

type allowKey struct {
	chatID int64
	userID int64
}

type fakeBot struct {
	members         map[allowKey]telegram.ChatMember
	getMemberErr    error
	deletedMessages []int64
	unbanned        map[allowKey]bool
}

func (b *fakeBot) GetChatMember(_ context.Context, chatID int64, userID int64) (telegram.ChatMember, error) {
	if b.getMemberErr != nil {
		return telegram.ChatMember{}, b.getMemberErr
	}
	if member, ok := b.members[allowKey{chatID, userID}]; ok {
		return member, nil
	}
	return telegram.ChatMember{Status: "member", User: telegram.User{ID: userID}}, nil
}

func (b *fakeBot) DeleteMessage(_ context.Context, _ int64, messageID int64) error {
	b.deletedMessages = append(b.deletedMessages, messageID)
	return nil
}

func (b *fakeBot) UnbanChatMember(_ context.Context, chatID int64, userID int64) error {
	if b.unbanned == nil {
		b.unbanned = make(map[allowKey]bool)
	}
	b.unbanned[allowKey{chatID, userID}] = true
	return nil
}

type fakeRepo struct {
	allowlisted map[allowKey]bool
	enqueued    map[allowKey]bool
	done        map[allowKey]bool
	removals    []store.RemovalItem
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		allowlisted: make(map[allowKey]bool),
		enqueued:    make(map[allowKey]bool),
		done:        make(map[allowKey]bool),
	}
}

func (r *fakeRepo) UpsertChat(context.Context, int64, string, string, string) error {
	return nil
}

func (r *fakeRepo) IsGuardEnabled(context.Context, int64) (bool, error) {
	return true, nil
}

func (r *fakeRepo) IsAllowed(_ context.Context, chatID int64, userID int64) (bool, error) {
	return r.allowlisted[allowKey{chatID, userID}], nil
}

func (r *fakeRepo) UpsertKnownMember(context.Context, store.KnownMember) error {
	return nil
}

func (r *fakeRepo) EnqueueRemoval(_ context.Context, chatID int64, userID int64, _ string, _ *int64) error {
	r.enqueued[allowKey{chatID, userID}] = true
	return nil
}

func (r *fakeRepo) ListDueRemovals(context.Context, int) ([]store.RemovalItem, error) {
	return r.removals, nil
}

func (r *fakeRepo) MarkRemovalDone(_ context.Context, chatID int64, userID int64) error {
	r.done[allowKey{chatID, userID}] = true
	return nil
}

func (r *fakeRepo) MarkRemovalError(context.Context, int64, int64, int, time.Time, string) error {
	return nil
}

func (r *fakeRepo) RecordEvent(context.Context, store.Event) error {
	return nil
}

func userID(user *telegram.User) int64 {
	if user == nil {
		return 0
	}
	return user.ID
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
