package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.telegram.org"

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

type ClientOption func(*Client)

func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func NewClient(token string, opts ...ClientOption) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("telegram token is required")
	}

	c := &Client{
		token:   token,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 35 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

type APIError struct {
	Code        int
	Description string
	RetryAfter  int
}

func (e *APIError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("telegram api error %d: %s; retry_after=%d", e.Code, e.Description, e.RetryAfter)
	}
	return fmt.Sprintf("telegram api error %d: %s", e.Code, e.Description)
}

type responseParameters struct {
	RetryAfter int `json:"retry_after,omitempty"`
}

func (c *Client) GetMe(ctx context.Context) (User, error) {
	var user User
	if err := c.do(ctx, "getMe", struct{}{}, &user); err != nil {
		return User{}, err
	}
	return user, nil
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int, allowedUpdates []string) ([]Update, error) {
	var updates []Update
	payload := struct {
		Offset         int64    `json:"offset,omitempty"`
		Limit          int      `json:"limit,omitempty"`
		Timeout        int      `json:"timeout,omitempty"`
		AllowedUpdates []string `json:"allowed_updates,omitempty"`
	}{
		Offset:         offset,
		Limit:          100,
		Timeout:        timeoutSeconds,
		AllowedUpdates: allowedUpdates,
	}
	if err := c.do(ctx, "getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *Client) DeleteMessage(ctx context.Context, chatID int64, messageID int64) error {
	payload := struct {
		ChatID    int64 `json:"chat_id"`
		MessageID int64 `json:"message_id"`
	}{
		ChatID:    chatID,
		MessageID: messageID,
	}
	return c.do(ctx, "deleteMessage", payload, nil)
}

func (c *Client) UnbanChatMember(ctx context.Context, chatID int64, userID int64) error {
	payload := struct {
		ChatID int64 `json:"chat_id"`
		UserID int64 `json:"user_id"`
	}{
		ChatID: chatID,
		UserID: userID,
	}
	return c.do(ctx, "unbanChatMember", payload, nil)
}

func (c *Client) GetChatMember(ctx context.Context, chatID int64, userID int64) (ChatMember, error) {
	var member ChatMember
	payload := struct {
		ChatID int64 `json:"chat_id"`
		UserID int64 `json:"user_id"`
	}{
		ChatID: chatID,
		UserID: userID,
	}
	if err := c.do(ctx, "getChatMember", payload, &member); err != nil {
		return ChatMember{}, err
	}
	return member, nil
}

func (c *Client) do(ctx context.Context, method string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.methodURL(method), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call telegram method %s: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read telegram response: %w", err)
	}

	var envelope struct {
		OK          bool                `json:"ok"`
		Result      json.RawMessage     `json:"result"`
		ErrorCode   int                 `json:"error_code,omitempty"`
		Description string              `json:"description,omitempty"`
		Parameters  *responseParameters `json:"parameters,omitempty"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("decode telegram response: %w", err)
	}
	if !envelope.OK {
		apiErr := &APIError{Code: envelope.ErrorCode, Description: envelope.Description}
		if envelope.Parameters != nil {
			apiErr.RetryAfter = envelope.Parameters.RetryAfter
		}
		return apiErr
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("decode telegram result: %w", err)
	}
	return nil
}

func (c *Client) methodURL(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
}
