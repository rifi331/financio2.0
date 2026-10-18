// Package telegram is a thin long-polling wrapper over the Telegram Bot API.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MaxDownloadBytes caps media downloads (photos/PDFs) at 10MB.
const MaxDownloadBytes = 10 << 20

// ErrConflict means Telegram answered 409: another poller holds the
// getUpdates lock. Single-replica contract per BRIEF — fatal.
var ErrConflict = errors.New("telegram 409 conflict: another bot instance is polling with this token (single replica only)")

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
// update until ctx is cancelled, returning nil. The update offset is advanced
// only after a batch is handed to the handler. Transient errors are retried
// with backoff so the poll loop survives outages; a 409 Conflict returns
// ErrConflict (fatal — never run two replicas).
func (c *Client) Run(ctx context.Context, handler func(tgbotapi.Update)) error {
	offset := 0
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		u := tgbotapi.NewUpdate(offset)
		u.Timeout = 60
		u.AllowedUpdates = []string{"message"}

		upds, err := c.api.GetUpdates(u)
		if err != nil {
			if errors.Is(err, ctx.Err()) {
				return nil
			}
			var apiErr *tgbotapi.Error
			if errors.As(err, &apiErr) && apiErr.Code == http.StatusConflict {
				return ErrConflict
			}
			c.log.Error("getUpdates failed, backing off", "err", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		backoff = time.Second

		for _, upd := range upds {
			offset = upd.UpdateID + 1
			handler(upd)
		}
	}
}

// Download fetches a file by id via getFile + download, capped at
// MaxDownloadBytes. It satisfies the bot.Downloader interface.
func (c *Client) Download(ctx context.Context, fileID string) ([]byte, error) {
	f, err := c.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("getFile: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.Link(c.api.Token), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.api.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxDownloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	if len(data) > MaxDownloadBytes {
		return nil, errors.New("file exceeds 10MB download cap")
	}
	return data, nil
}

// SendMessage sends one plain-text message (no parse mode).
func (c *Client) SendMessage(chatID int64, text string) error {
	return c.SendMessages(chatID, []string{text})
}

// SendDocument uploads an in-memory file as a Telegram document.
func (c *Client) SendDocument(chatID int64, filename string, data []byte) error {
	msg := tgbotapi.NewDocument(chatID, tgbotapi.FileBytes{Name: filename, Bytes: data})
	if _, err := c.api.Send(msg); err != nil {
		c.log.Error("send document failed", "chat_id", chatID, "err", err)
		return err
	}
	return nil
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
