package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const apiBase = "https://api.telegram.org"

// User is a Telegram user/bot.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// Chat identifies the conversation.
type Chat struct {
	ID int64 `json:"id"`
}

// Message is an incoming chat message (only fields we use).
type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

// Update is one getUpdates entry.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

// Client is a minimal Telegram Bot API client.
type Client struct {
	token string
	http  *http.Client
}

// NewClient builds a Client with a long-poll-friendly HTTP timeout.
func NewClient(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 70 * time.Second}}
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/bot%s/%s", apiBase, url.PathEscape(c.token), method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var env apiResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if !env.OK {
		return fmt.Errorf("telegram %s error %d: %s", method, env.ErrorCode, env.Description)
	}
	if out != nil && len(env.Result) > 0 {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// GetMe returns the bot account (used to verify the token at startup).
func (c *Client) GetMe(ctx context.Context) (*User, error) {
	var u User
	if err := c.call(ctx, "getMe", struct{}{}, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUpdates long-polls for new updates from offset.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]Update, error) {
	params := map[string]any{"offset": offset, "timeout": timeoutSec, "limit": 100, "allowed_updates": []string{"message"}}
	var updates []Update
	if err := c.call(ctx, "getUpdates", params, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

// SendMessage sends a text reply to a chat.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	return c.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text}, nil)
}
