// Package telegram is a thin long-polling wrapper over the Telegram Bot API.
package telegram

import (
	"context"
	"log/slog"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Client wraps a telegram-bot-api bot for long polling and plain-text sends.
type Client struct {
	api *tgbotapi.BotAPI
	log *slog.Logger
}

// New validates the token via getMe and returns a Client.
func New(token string) (*Client, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Client{api: api, log: slog.Default()}, nil
}

// Run polls getUpdates (60s timeout, messages only) and calls handler for each
// update until ctx is cancelled. The update offset is advanced internally.
func (c *Client) Run(ctx context.Context, handler func(tgbotapi.Update)) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	u.AllowedUpdates = []string{"message"}

	updates := c.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			c.api.StopReceivingUpdates()
			return
		case upd, ok := <-updates:
			if !ok {
				return
			}
			handler(upd)
		}
	}
}

// SendMessage sends one plain-text message (no parse mode).
func (c *Client) SendMessage(chatID int64, text string) error {
	return c.SendMessages(chatID, []string{text})
}

// SendMessages sends each part as its own plain-text message, splitting any
// part longer than 4000 characters at line boundaries where possible.
func (c *Client) SendMessages(chatID int64, parts []string) error {
	for _, part := range parts {
		for _, chunk := range splitMessage(part) {
			msg := tgbotapi.NewMessage(chatID, chunk)
			msg.ParseMode = "" // plain text only; avoids escaping bugs
			if _, err := c.api.Send(msg); err != nil {
				c.log.Error("send message failed", "chat_id", chatID, "err", err)
				return err
			}
		}
	}
	return nil
}

const maxMsgLen = 4000

func splitMessage(text string) []string {
	if len(text) <= maxMsgLen {
		return []string{text}
	}
	var out []string
	for len(text) > maxMsgLen {
		cut := strings.LastIndexAny(text[:maxMsgLen], "\n .,")
		if cut < maxMsgLen/2 {
			cut = maxMsgLen
		}
		out = append(out, strings.TrimRight(text[:cut], "\n .,"))
		text = strings.TrimLeft(text[cut:], "\n .,")
	}
	if text != "" {
		out = append(out, text)
	}
	return out
}
