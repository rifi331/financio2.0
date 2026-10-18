package main

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Regression: botMessage used to dereference m.Document unconditionally and
// panicked on every plain text message in production (nil Document).
func TestBotMessagePlainAndMedia(t *testing.T) {
	text := &tgbotapi.Message{Text: "10rm nasi lemak"}
	got := botMessage(text)
	if got.Text != "10rm nasi lemak" || got.HasPhoto || got.DocumentName != "" || got.DocumentFileID != "" {
		t.Errorf("plain message mapped wrong: %+v", got)
	}

	doc := &tgbotapi.Message{
		Document: &tgbotapi.Document{FileName: "statement.pdf", FileID: "f1"},
	}
	got = botMessage(doc)
	if got.DocumentName != "statement.pdf" || got.DocumentFileID != "f1" {
		t.Errorf("document message mapped wrong: %+v", got)
	}

	photo := &tgbotapi.Message{
		Photo:   []tgbotapi.PhotoSize{{FileID: "small"}, {FileID: "large"}},
		Caption: "receipt",
	}
	got = botMessage(photo)
	if !got.HasPhoto || got.PhotoFileID != "large" || got.Text != "receipt" {
		t.Errorf("photo message mapped wrong: %+v", got)
	}
}
