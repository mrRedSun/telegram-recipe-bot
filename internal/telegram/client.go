package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type User struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type Message struct {
	MessageID       int64    `json:"message_id"`
	MessageThreadID int64    `json:"message_thread_id"`
	From            *User    `json:"from"`
	Chat            Chat     `json:"chat"`
	Text            string   `json:"text"`
	Caption         string   `json:"caption"`
	ReplyToMessage  *Message `json:"reply_to_message"`
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type response[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
}

type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(base, token string) *Client {
	return &Client{
		base:  strings.TrimRight(base, "/"),
		token: token,
		http:  &http.Client{Timeout: 65 * time.Second},
	}
}

func (c *Client) method(name string) string { return c.base + "/bot" + c.token + "/" + name }

func (c *Client) Me(ctx context.Context) (User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.method("getMe"), nil)
	if err != nil {
		return User{}, errors.New("telegram getMe request construction failed")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return User{}, telegramTransportError(ctx, "getMe")
	}
	defer res.Body.Close()
	var out response[User]
	if err := decode(res, &out); err != nil {
		return User{}, err
	}
	if !out.OK {
		return User{}, errors.New("telegram getMe rejected")
	}
	return out.Result, nil
}

func (c *Client) Updates(ctx context.Context, offset int64) ([]Update, error) {
	v := url.Values{}
	v.Set("offset", strconv.FormatInt(offset, 10))
	v.Set("timeout", "50")
	v.Set("allowed_updates", `["message"]`)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.method("getUpdates")+"?"+v.Encode(), nil)
	if err != nil {
		return nil, errors.New("telegram getUpdates request construction failed")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, telegramTransportError(ctx, "getUpdates")
	}
	defer res.Body.Close()
	var out response[[]Update]
	if err := decode(res, &out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, errors.New("telegram getUpdates rejected")
	}
	return out.Result, nil
}

func (c *Client) Send(ctx context.Context, chatID, threadID int64, text string) (Message, error) {
	payload := map[string]any{"chat_id": chatID, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true}
	if threadID != 0 {
		payload["message_thread_id"] = threadID
	}
	return c.message(ctx, "sendMessage", payload)
}

func (c *Client) Edit(ctx context.Context, chatID, messageID int64, text string) error {
	_, err := c.message(ctx, "editMessageText", map[string]any{"chat_id": chatID, "message_id": messageID, "rich_message": map[string]any{"html": text, "skip_entity_detection": true}})
	return err
}

func (c *Client) message(ctx context.Context, method string, payload map[string]any) (Message, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Message{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.method(method), bytes.NewReader(body))
	if err != nil {
		return Message{}, fmt.Errorf("telegram %s request construction failed", method)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return Message{}, telegramTransportError(ctx, method)
	}
	defer res.Body.Close()
	var out response[Message]
	if err := decode(res, &out); err != nil {
		return Message{}, err
	}
	if !out.OK {
		return Message{}, fmt.Errorf("telegram %s rejected", method)
	}
	return out.Result, nil
}

func telegramTransportError(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("telegram %s transport failed", operation)
}

func decode(res *http.Response, dst any) error {
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("telegram HTTP %d", res.StatusCode)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
