// Package bot hosts the message dispatcher and the entry flow.
package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/rifi331/financio/internal/ai"
	"github.com/rifi331/financio/internal/auth"
	"github.com/rifi331/financio/internal/recap"
	"github.com/rifi331/financio/internal/receipt"
	"github.com/rifi331/financio/internal/report"
	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
)

// debugStack returns a stack trace for panic logs; wrapped for testability.
var debugStack = stackBytes

// Sender is the outbound messaging surface the dispatcher needs.
type Sender interface {
	SendMessage(chatID int64, text string) error
	SendMessages(chatID int64, parts []string) error
	SendDocument(chatID int64, filename string, data []byte) error
}

// TxStore is the persistence surface the dispatcher needs.
type TxStore interface {
	ListActiveCategories(ctx context.Context) ([]string, error)
	ListActiveCurrencies(ctx context.Context) ([]store.Currency, error)
	InsertTransaction(ctx context.Context, txn store.NewTransaction) (store.Transaction, error)
	DeleteTransactionByID(ctx context.Context, id, userID int64) (store.Transaction, bool, error)
	ListTransactions(ctx context.Context, f store.TxnFilter) ([]store.Transaction, error)
	ListDuplicateGroups(ctx context.Context, userID int64, from, to time.Time) ([]store.DuplicateGroup, error)
	GetPrompt(ctx context.Context, name string) (string, bool, error)
	SetPrompt(ctx context.Context, name, template string) error
	DeletePrompt(ctx context.Context, name string) (bool, error)
	ListPrompts(ctx context.Context) ([]store.Prompt, error)
}

// Parser is the AI extraction surface.
type Parser interface {
	ParseEntry(ctx context.Context, in ai.EntryInput) (ai.ParsedEntry, error)
	ParseRecapFilter(ctx context.Context, text string, today time.Time, categories []string, systemTemplate string) (ai.RecapFilter, error)
	ParsePhotoReceipt(ctx context.Context, image []byte, in ai.ReceiptInput) ([]ai.ParsedEntry, error)
	ParseTextReceipt(ctx context.Context, text string, in ai.ReceiptInput) ([]ai.ParsedEntry, error)
}

// promptTemplate loads a DB-stored prompt template; any problem falls back
// to "" (= compiled-in default) so customization can never break the bot.
func (d *Dispatcher) promptTemplate(ctx context.Context, name string) string {
	t, ok, err := d.store.GetPrompt(ctx, name)
	if err != nil {
		d.log.Warn("load prompt failed, using default", "name", name, "err", err)
		return ""
	}
	if !ok {
		return ""
	}
	return t
}

// Downloader fetches a Telegram file by id (used for photo/PDF media).
type Downloader interface {
	Download(ctx context.Context, fileID string) ([]byte, error)
}

// Dispatcher routes authorized messages to their intent handlers.
type Dispatcher struct {
	store  TxStore
	auth   *auth.Authorizer
	ai     Parser
	tg     Sender
	down   Downloader
	tz     *time.Location
	defCur string
	nowFn  func() time.Time
	log    *slog.Logger
}

// NewDispatcher wires the dispatcher. nowFn is injectable for tests (nil -> time.Now).
func NewDispatcher(s TxStore, a *auth.Authorizer, p Parser, tg Sender, down Downloader, tz *time.Location, defaultCurrency string, nowFn func() time.Time) *Dispatcher {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Dispatcher{store: s, auth: a, ai: p, tg: tg, down: down, tz: tz, defCur: defaultCurrency,
		nowFn: nowFn, log: slog.Default()}
}

// perMessageTimeout bounds the total processing time of one update.
const perMessageTimeout = 90 * time.Second

// Handle processes one raw Telegram update end to end. It never panics up to
// the caller (one bad message must not kill the poll loop) and always logs
// update_id/user_id/intent/latency_ms/outcome for observability.
func (d *Dispatcher) Handle(ctx context.Context, updateID, chatID, userID int64, firstName, lastName string, msg router.Message) {
	start := time.Now()
	outcome := "panic"
	defer func() {
		if r := recover(); r != nil {
			d.log.Error("handler panic recovered", "update_id", updateID, "user_id", userID, "panic", r,
				"stack", string(debugStack()))
		}
		d.log.Info("message processed",
			"update_id", updateID,
			"user_id", userID,
			"intent", router.Classify(msg).Kind.String(),
			"latency_ms", time.Since(start).Milliseconds(),
			"outcome", outcome)
	}()

	ctx, cancel := context.WithTimeout(ctx, perMessageTimeout)
	defer cancel()

	outcome = d.handle(ctx, updateID, chatID, userID, firstName, lastName, msg)
}

func (d *Dispatcher) handle(ctx context.Context, updateID, chatID, userID int64, firstName, lastName string, msg router.Message) (outcome string) {
	outcome = "error"
	user, ok, err := d.auth.Authorize(ctx, userID, firstName, lastName)
	if err != nil {
		d.log.Error("authorize failed", "user_id", userID, "err", err)
		return "auth_error"
	}
	if !ok {
		return // dropped silently; never reply
	}

	intent := router.Classify(msg)
	switch intent.Kind {
	case router.IntentDelete:
		outcome = d.handleDelete(ctx, chatID, user, intent.DeleteID, intent.Text)
	case router.IntentRecap:
		outcome = d.handleRecap(ctx, chatID, user, intent.Text)
	case router.IntentDuplicate:
		outcome = d.handleDuplicate(ctx, chatID, user, intent.Text)
	case router.IntentReport:
		outcome = d.handleReport(ctx, chatID, user, intent.Text)
	case router.IntentPrompt:
		outcome = d.handlePrompt(ctx, chatID, user, intent.Text)
	case router.IntentPhoto:
		outcome = d.handlePhoto(ctx, chatID, user, msg, "photo")
	case router.IntentPDF:
		outcome = d.handlePDF(ctx, chatID, user, msg)
	case router.IntentHelp:
		d.reply(ctx, chatID, helpText)
		outcome = "help"
	default:
		outcome = d.handleEntry(ctx, chatID, user, intent.Text)
	}
	return outcome
}

// helpText is the compact bilingual usage card sent for /start and /help.
const helpText = `🤖 Financio — expense tracking via Telegram

📝 Log / Catat:
• 10rm nasi lemak
• nasi lemak 10rm yesterday
• gaji 6500 (income)

🗑️ Delete / Hapus:
• hapus 123
• delete 123

📊 Recap / Rekap:
• recap nov 2025
• recap this week meals
• rekap bulan ini

🧾 Receipts / Struk:
• send a receipt photo
• kirim foto struk atau PDF statement`

func (d *Dispatcher) handleEntry(ctx context.Context, chatID int64, user store.User, text string) (outcome string) {
	outcome = "error"
	lang := detectLanguage(text)

	categories, currencies, err := d.loadRefData(ctx)
	if err != nil {
		d.log.Error("load ref data failed", "err", err)
		d.reply(ctx, chatID, parseFailHint)
		return
	}

	now := d.nowFn().In(d.tz)
	raw := text
	parsed, err := d.ai.ParseEntry(ctx, ai.EntryInput{
		Message:         text,
		CurrentDate:     now.Format("2006-01-02"),
		SystemTemplate:  d.promptTemplate(ctx, "entry"),
		Categories:      categories,
		Currencies:      currencyCodes(currencies),
		DefaultCurrency: d.defCur,
	})
	if err != nil {
		d.log.Error("parse entry failed", "err", err, "raw", text)
		d.reply(ctx, chatID, parseFailHint)
		return
	}

	if !parsed.IsTransaction {
		d.reply(ctx, chatID, hint(lang))
		return "not_transaction"
	}

	txn, err := d.insertParsed(ctx, user, parsed, categories, currencies, now, "text", raw)
	if err != nil {
		d.log.Warn("validation failed", "err", err, "raw", text)
		d.reply(ctx, chatID, parseFailHint)
		return
	}

	d.reply(ctx, chatID, formatConfirmation(txn, currencies, firstName(user.DisplayName), lang))
	return "saved"
}

// handlePhoto: download the photo, parse it with the vision model, insert via
// the shared entry path (source="photo").
func (d *Dispatcher) handlePhoto(ctx context.Context, chatID int64, user store.User, msg router.Message, source string) (outcome string) {
	outcome = "error"
	lang := detectLanguage(msg.Text)

	image, err := d.down.Download(ctx, msg.PhotoFileID)
	if err != nil {
		d.log.Error("photo download failed", "err", err, "file_id", msg.PhotoFileID)
		d.reply(ctx, chatID, downloadFailHint(lang))
		return
	}

	entries, categories, currencies, err := d.parseReceipt(ctx, "photo_receipt", func(in ai.ReceiptInput) ([]ai.ParsedEntry, error) {
		return d.ai.ParsePhotoReceipt(ctx, image, in)
	})
	if err != nil {
		d.log.Error("parse photo receipt failed", "err", err)
		d.reply(ctx, chatID, parseFailHint)
		return
	}
	return d.insertReceiptEntries(ctx, chatID, user, entries, categories, currencies, source, msg.Text, lang)
}

// handlePDF: download the document, extract text (30KB guard), parse with the
// statement rules, insert via the shared entry path (source="pdf").
func (d *Dispatcher) handlePDF(ctx context.Context, chatID int64, user store.User, msg router.Message) (outcome string) {
	outcome = "error"
	lang := detectLanguage(msg.Text)

	data, err := d.down.Download(ctx, msg.DocumentFileID)
	if err != nil {
		d.log.Error("pdf download failed", "err", err, "file_id", msg.DocumentFileID)
		d.reply(ctx, chatID, downloadFailHint(lang))
		return
	}

	text, err := receipt.ExtractPDFText(data)
	if err != nil || !receipt.TextOK(text) {
		d.log.Warn("pdf text unreadable", "err", err, "bytes", len(text))
		d.reply(ctx, chatID, receipt.UnreadablePDFMessage(lang))
		return "pdf_unreadable"
	}

	entries, categories, currencies, err := d.parseReceipt(ctx, "pdf_receipt", func(in ai.ReceiptInput) ([]ai.ParsedEntry, error) {
		return d.ai.ParseTextReceipt(ctx, text, in)
	})
	if err != nil {
		d.log.Error("parse pdf receipt failed", "err", err)
		d.reply(ctx, chatID, parseFailHint)
		return
	}
	return d.insertReceiptEntries(ctx, chatID, user, entries, categories, currencies, "pdf", msg.DocumentName, lang)
}

func (d *Dispatcher) parseReceipt(ctx context.Context, name string, call func(ai.ReceiptInput) ([]ai.ParsedEntry, error)) ([]ai.ParsedEntry, []string, []store.Currency, error) {
	categories, currencies, err := d.loadRefData(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	tmpl := d.promptTemplate(ctx, name)
	now := d.nowFn().In(d.tz)
	entries, err := call(ai.ReceiptInput{
		CurrentDate:     now.Format("2006-01-02"),
		SystemTemplate:  tmpl,
		Categories:      categories,
		Currencies:      currencyCodes(currencies),
		DefaultCurrency: d.defCur,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return entries, categories, currencies, nil
}

// insertReceiptEntries runs the shared validation + insert path for every
// parsed transaction. One insert -> the normal confirmation; >1 -> one compact
// aggregate reply. Invalid rows are skipped (logged), not fatal.
func (d *Dispatcher) insertReceiptEntries(ctx context.Context, chatID int64, user store.User, entries []ai.ParsedEntry, categories []string, currencies []store.Currency, source, raw, lang string) (outcome string) {
	now := d.nowFn().In(d.tz)
	var txns []store.Transaction
	for _, e := range entries {
		if !e.IsTransaction {
			continue
		}
		txn, err := d.insertParsed(ctx, user, e, categories, currencies, now, source, raw)
		if err != nil {
			d.log.Warn("receipt row validation failed", "err", err, "item", e.Item, "source", source)
			continue
		}
		txns = append(txns, txn)
	}

	switch {
	case len(txns) == 0:
		d.reply(ctx, chatID, hint(lang))
		return "no_transaction"
	case len(txns) == 1:
		d.reply(ctx, chatID, formatConfirmation(txns[0], currencies, firstName(user.DisplayName), lang))
	default:
		d.reply(ctx, chatID, receipt.FormatMulti(txns, currencies))
	}
	return "saved"
}

func downloadFailHint(lang string) string {
	if lang == "ID" {
		return "File tidak bisa diunduh. Coba kirim ulang."
	}
	return "Couldn't download that file. Please send it again."
}

const parseFailHint = "Could not parse that — try e.g. '10rm nasi lemak' / 'gaji 5000 bulan ini'"

// handleDelete deletes by id scoped to the requesting user. Not-found and
// not-owned are indistinguishable in the reply ("Record not found!") so we
// never reveal whether an id exists for someone else.
func (d *Dispatcher) handleDelete(ctx context.Context, chatID int64, user store.User, id int64, text string) (outcome string) {
	lang := detectLanguage(text)
	txn, ok, err := d.store.DeleteTransactionByID(ctx, id, user.ID)
	if err != nil {
		d.log.Error("delete failed", "err", err, "id", id)
		d.reply(ctx, chatID, recapFailHint(lang))
		return "error"
	}
	if !ok {
		if lang == "ID" {
			d.reply(ctx, chatID, "Catatan tidak ditemukan!")
		} else {
			d.reply(ctx, chatID, "Record not found!")
		}
		return "not_found"
	}
	if lang == "ID" {
		d.reply(ctx, chatID, fmt.Sprintf("🗑️ item dengan id %d sudah dihapus\n🛍️: %s", id, txn.Item))
	} else {
		d.reply(ctx, chatID, fmt.Sprintf("🗑️ item deleted for id: %d\n🛍️: %s", id, txn.Item))
	}
	return "deleted"
}

// handleRecap: the AI returns a structured filter only; Go builds the
// parameterized query. No model output ever reaches SQL as text.
func (d *Dispatcher) handleRecap(ctx context.Context, chatID int64, user store.User, text string) (outcome string) {
	outcome = "error"
	categories, err := d.store.ListActiveCategories(ctx)
	if err != nil {
		d.log.Error("load categories failed", "err", err)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}

	now := d.nowFn().In(d.tz)
	filter, err := d.ai.ParseRecapFilter(ctx, text, now, categories, d.promptTemplate(ctx, "recap_filter"))
	if err != nil {
		d.log.Error("parse recap filter failed", "err", err, "raw", text)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}
	if !filter.IsRecapRequest {
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}

	from, to, err := parseAndValidateRange(filter.DateFrom, filter.DateTo, now)
	if err != nil {
		d.log.Warn("recap range invalid", "err", err, "raw", text)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}

	lang := filter.Language
	if lang != "ID" {
		lang = "EN"
	}

	validCats := intersect(filter.Categories, categories)

	// Recap reports spending only — income rows are excluded from the totals.
	f := store.TxnFilter{UserID: user.ID, From: from, To: to, Categories: validCats, Direction: "expense"}
	txns, err := d.store.ListTransactions(ctx, f)
	if err != nil {
		d.log.Error("list transactions failed", "err", err)
		d.reply(ctx, chatID, recapFailHint(lang))
		return
	}

	if len(txns) == 0 {
		if lang == "ID" {
			d.reply(ctx, chatID, "Tidak ada catatan pada periode ini.")
		} else {
			d.reply(ctx, chatID, "No transactions in this period.")
		}
		return "empty"
	}

	currencies, err := d.store.ListActiveCurrencies(ctx)
	if err != nil {
		d.log.Error("load currencies failed", "err", err)
		d.reply(ctx, chatID, recapFailHint(lang))
		return
	}
	parts := recap.Format(txns, from, to, lang, currencies, len(validCats) > 0)
	if err := d.tg.SendMessages(chatID, parts); err != nil {
		d.log.Error("send recap failed", "err", err)
		return
	}
	return "recap"
}

var validPromptNames = []string{"entry", "recap_filter", "photo_receipt", "pdf_receipt"}

func validPromptName(name string) bool {
	for _, n := range validPromptNames {
		if n == name {
			return true
		}
	}
	return false
}

// handlePrompt implements the /prompt admin command:
//
//	/prompt                    — list prompts and updated_at
//	/prompt show <name>        — show a template
//	/prompt set <name> <text>  — replace a template (placeholders {{categories}},
//	                            {{currencies}}, {{default_currency}})
//	/prompt reset <name>       — delete the row, fall back to the built-in default
func (d *Dispatcher) handlePrompt(ctx context.Context, chatID int64, user store.User, text string) (outcome string) {
	outcome = "error"
	fields := strings.Fields(strings.TrimSpace(text))
	sub := "list"
	if len(fields) >= 2 {
		sub = strings.ToLower(fields[1])
	}

	usage := "Usage:\n/prompt list\n/prompt show <name>\n/prompt set <name> <template...>\n/prompt reset <name>\nNames: " + strings.Join(validPromptNames, ", ")

	switch sub {
	case "list":
		prompts, err := d.store.ListPrompts(ctx)
		if err != nil {
			d.log.Error("list prompts failed", "err", err)
			d.reply(ctx, chatID, "Failed to list prompts.")
			return
		}
		var b strings.Builder
		b.WriteString("Prompt templates in DB:\n")
		for _, pr := range prompts {
			fmt.Fprintf(&b, "• %s (updated %s)\n", pr.Name, pr.UpdatedAt.In(d.tz).Format("2006-01-02 15:04"))
		}
		b.WriteString("\nshow/set/reset with a name; placeholders: {{categories}}, {{currencies}}, {{default_currency}}")
		d.reply(ctx, chatID, b.String())
		return "prompt_list"

	case "show":
		if len(fields) < 3 || !validPromptName(fields[2]) {
			d.reply(ctx, chatID, usage)
			return
		}
		t, ok, err := d.store.GetPrompt(ctx, fields[2])
		if err != nil {
			d.reply(ctx, chatID, "Failed to load prompt.")
			return
		}
		if !ok {
			d.reply(ctx, chatID, fmt.Sprintf("%s: no DB override, using built-in default.", fields[2]))
			return "prompt_show"
		}
		d.reply(ctx, chatID, fmt.Sprintf("%s:\n%s", fields[2], t))
		return "prompt_show"

	case "set":
		rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), fields[0]))
		rest = strings.TrimSpace(strings.TrimPrefix(rest, fields[1]))
		if len(fields) < 4 || !validPromptName(fields[2]) || rest == "" {
			d.reply(ctx, chatID, usage)
			return
		}
		name := fields[2]
		body := strings.TrimSpace(strings.TrimPrefix(rest, name))
		if err := d.store.SetPrompt(ctx, name, body); err != nil {
			d.log.Error("set prompt failed", "err", err)
			d.reply(ctx, chatID, "Failed to save prompt.")
			return
		}
		d.reply(ctx, chatID, fmt.Sprintf("OK: %s updated. It applies from the next message; /prompt reset %s restores the default.", name, name))
		return "prompt_set"

	case "reset":
		if len(fields) < 3 || !validPromptName(fields[2]) {
			d.reply(ctx, chatID, usage)
			return
		}
		deleted, err := d.store.DeletePrompt(ctx, fields[2])
		if err != nil {
			d.reply(ctx, chatID, "Failed to reset prompt.")
			return
		}
		if deleted {
			d.reply(ctx, chatID, fmt.Sprintf("%s reset to the built-in default.", fields[2]))
		} else {
			d.reply(ctx, chatID, fmt.Sprintf("%s had no DB override.", fields[2]))
		}
		return "prompt_reset"
	}

	d.reply(ctx, chatID, usage)
	return
}

// handleReport builds the full-report PDF (summary, expense pies per
// currency, income/expense tables) and sends it as a document.
func (d *Dispatcher) handleReport(ctx context.Context, chatID int64, user store.User, text string) (outcome string) {
	outcome = "error"
	categories, err := d.store.ListActiveCategories(ctx)
	if err != nil {
		d.log.Error("load categories failed", "err", err)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}

	now := d.nowFn().In(d.tz)
	filter, err := d.ai.ParseRecapFilter(ctx, text, now, categories, d.promptTemplate(ctx, "recap_filter"))
	if err != nil {
		d.log.Error("parse report filter failed", "err", err, "raw", text)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}
	from, to, err := parseAndValidateRange(filter.DateFrom, filter.DateTo, now)
	if err != nil {
		d.log.Warn("report range invalid", "err", err, "raw", text)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}

	f := store.TxnFilter{UserID: user.ID, From: from, To: to, Categories: intersect(filter.Categories, categories)}
	txns, err := d.store.ListTransactions(ctx, f)
	if err != nil {
		d.log.Error("list transactions failed", "err", err)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}
	if len(txns) == 0 {
		if filter.Language == "ID" {
			d.reply(ctx, chatID, "Tidak ada catatan pada periode ini.")
		} else {
			d.reply(ctx, chatID, "No transactions in this period.")
		}
		return "empty"
	}

	currencies, err := d.store.ListActiveCurrencies(ctx)
	if err != nil {
		d.log.Error("load currencies failed", "err", err)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}

	pdfBytes, err := report.Build(report.Data{
		From: from, To: to, Txns: txns, Currencies: currencies,
		TZ: d.tz, Generated: now,
	})
	if err != nil {
		d.log.Error("build report failed", "err", err)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}
	filename := fmt.Sprintf("financio-report-%s.pdf", from.In(d.tz).Format("2006-01"))
	if err := d.tg.SendDocument(chatID, filename, pdfBytes); err != nil {
		d.log.Error("send report failed", "err", err)
		return
	}
	return "report"
}

func recapFailHint(lang string) string {
	if lang == "ID" {
		return "Maaf, permintaan rekap tidak bisa diproses. Coba misalnya 'recap nov 2025' atau 'rekap bulan ini'."
	}
	return "Sorry, I couldn't process that recap request. Try e.g. 'recap nov 2025' or 'rekap bulan ini'."
}

// handleDuplicate lists same-user transactions in the requested month that
// share item + amount + currency + date — likely accidental double entries.
// The month is interpreted by the same AI filter the recap flow uses.
func (d *Dispatcher) handleDuplicate(ctx context.Context, chatID int64, user store.User, text string) (outcome string) {
	outcome = "error"
	categories, err := d.store.ListActiveCategories(ctx)
	if err != nil {
		d.log.Error("load categories failed", "err", err)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}

	now := d.nowFn().In(d.tz)
	filter, err := d.ai.ParseRecapFilter(ctx, text, now, categories, d.promptTemplate(ctx, "recap_filter"))
	if err != nil {
		d.log.Error("parse recap filter failed", "err", err, "raw", text)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}
	from, to, err := parseAndValidateRange(filter.DateFrom, filter.DateTo, now)
	if err != nil {
		d.log.Warn("duplicate range invalid", "err", err, "raw", text)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}

	groups, err := d.store.ListDuplicateGroups(ctx, user.ID, from, to)
	if err != nil {
		d.log.Error("list duplicate groups failed", "err", err)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}

	lang := filter.Language
	if lang != "ID" {
		lang = "EN"
	}
	if len(groups) == 0 {
		if lang == "ID" {
			d.reply(ctx, chatID, fmt.Sprintf("✅ Tidak ada transaksi duplikat pada %s.", from.In(d.tz).Format("January 2006")))
		} else {
			d.reply(ctx, chatID, fmt.Sprintf("✅ No duplicate transactions in %s.", from.In(d.tz).Format("January 2006")))
		}
		return "empty"
	}

	dp := func(code string) int {
		if code == "IDR" {
			return 0
		}
		return 2
	}
	var b strings.Builder
	if lang == "ID" {
		fmt.Fprintf(&b, "🔁 %d grup duplikat pada %s:\n", len(groups), from.In(d.tz).Format("January 2006"))
	} else {
		fmt.Fprintf(&b, "🔁 %d duplicate groups in %s:\n", len(groups), from.In(d.tz).Format("January 2006"))
	}
	for _, g := range groups {
		ids := make([]string, len(g.IDs))
		for i, id := range g.IDs {
			ids[i] = fmt.Sprintf("hapus %d", id)
			if lang != "ID" {
				ids[i] = fmt.Sprintf("delete %d", id)
			}
		}
		fmt.Fprintf(&b, "• %dx %s — %s (%s) [%s]\n",
			len(g.IDs), recap.FormatAmountCode(g.Currency, dp(g.Currency), g.Amount),
			g.Item, g.EntryDate.In(d.tz).Format("2006-01-02"), strings.Join(ids, " / "))
	}
	if err := d.tg.SendMessages(chatID, []string{b.String()}); err != nil {
		d.log.Error("send duplicates failed", "err", err)
		return
	}
	return "duplicates"
}

// parseAndValidateRange parses the AI's date bounds and enforces the guard
// rails: from <= to, both valid, range clamped to 5 years.
func parseAndValidateRange(fromStr, toStr string, now time.Time) (time.Time, time.Time, error) {
	from, err := time.ParseInLocation("2006-01-02", fromStr, now.Location())
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("bad date_from %q", fromStr)
	}
	to, err := time.ParseInLocation("2006-01-02", toStr, now.Location())
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("bad date_to %q", toStr)
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("date_from %s after date_to %s", fromStr, toStr)
	}
	if to.Sub(from) > 5*365*24*time.Hour {
		to = from.AddDate(5, 0, 0)
	}
	return from, to, nil
}

func intersect(requested, known []string) []string {
	var out []string
	for _, r := range requested {
		for _, k := range known {
			if strings.EqualFold(r, k) {
				out = append(out, k)
				break
			}
		}
	}
	return out
}

func firstName(displayName string) string {
	if i := strings.IndexByte(displayName, ' '); i > 0 {
		return displayName[:i]
	}
	return displayName
}

// insertParsed is the ONE validation + insert path shared by text, photo, and
// PDF sources (BRIEF: single insert path for all sources).
func (d *Dispatcher) insertParsed(ctx context.Context, user store.User, p ai.ParsedEntry, categories []string, currencies []store.Currency, now time.Time, source, raw string) (store.Transaction, error) {
	if p.Amount <= 0 {
		return store.Transaction{}, fmt.Errorf("amount must be > 0, got %v", p.Amount)
	}
	if p.Category == "" || !contains(categories, p.Category) {
		return store.Transaction{}, fmt.Errorf("category %q not in list", p.Category)
	}
	if p.Currency == "" {
		p.Currency = d.defCur
	}
	if currencyByCode(currencies, p.Currency) == nil {
		return store.Transaction{}, fmt.Errorf("currency %q not in list", p.Currency)
	}
	entryDate, err := time.ParseInLocation("2006-01-02", p.Date, d.tz)
	if err != nil {
		return store.Transaction{}, fmt.Errorf("bad date %q", p.Date)
	}
	if entryDate.After(now.AddDate(0, 0, 1)) {
		return store.Transaction{}, fmt.Errorf("date %s is more than 1 day in the future", p.Date)
	}
	if p.Direction == "" {
		p.Direction = "expense"
	}

	amount := formatMinor(p.Amount, currencyByCode(currencies, p.Currency).DecimalPlaces)

	var desc []byte
	if len(p.Details) > 0 {
		desc, _ = json.Marshal(p.Details)
	}

	return d.store.InsertTransaction(ctx, store.NewTransaction{
		UserID:       user.ID,
		Item:         p.Item,
		Category:     p.Category,
		CurrencyCode: p.Currency,
		TotalAmount:  amount,
		Direction:    p.Direction,
		EntryDate:    entryDate,
		Description:  desc,
		Source:       source,
		RawMessage:   raw,
	})
}

func (d *Dispatcher) loadRefData(ctx context.Context) ([]string, []store.Currency, error) {
	categories, err := d.store.ListActiveCategories(ctx)
	if err != nil {
		return nil, nil, err
	}
	currencies, err := d.store.ListActiveCurrencies(ctx)
	if err != nil {
		return nil, nil, err
	}
	return categories, currencies, nil
}

func (d *Dispatcher) reply(ctx context.Context, chatID int64, text string) {
	if err := d.tg.SendMessage(chatID, text); err != nil {
		d.log.Error("reply failed", "chat_id", chatID, "err", err)
	}
}

var idWordsRe = regexp.MustCompile(`(?i)\b(rekap|kemarin|kemaren|bulan|minggu|gaji|hapus|terima|untuk|saya|sudah|belum|dan|yang|dengan|bukan|hari|esok)\b`)

// detectLanguage applies a cheap Indonesian-word heuristic.
func detectLanguage(text string) string {
	if idWordsRe.MatchString(text) {
		return "ID"
	}
	return "EN"
}

func hint(lang string) string {
	if lang == "ID" {
		return "Sepertinya bukan catatan transaksi. Coba kirim seperti '10rm nasi lemak' atau 'gaji 5000 bulan ini'."
	}
	return "That doesn't look like a transaction. Try something like '10rm nasi lemak' or 'salary 5000 this month'."
}

func formatConfirmation(t store.Transaction, currencies []store.Currency, first, lang string) string {
	cur := currencyByCode(currencies, t.CurrencyCode)
	var b strings.Builder
	fmt.Fprintf(&b, "🆔: %d\n", t.ID)
	fmt.Fprintf(&b, "🛍️: %s\n", t.Item)
	fmt.Fprintf(&b, "💵: %s\n", FormatAmount(cur, t.TotalAmount))
	fmt.Fprintf(&b, "🗓️: %s\n", t.EntryDate.Format("2006-01-02"))
	fmt.Fprintf(&b, "🔠: %s\n", t.CategoryName)
	if t.Direction == "income" {
		b.WriteString("↩️: Income\n")
	}
	if lang == "ID" {
		fmt.Fprintf(&b, "Terima kasih, %s!", first)
	} else {
		fmt.Fprintf(&b, "Thanks, %s!", first)
	}
	return b.String()
}

// FormatAmount renders amounts per currency rules (moved to internal/recap,
// aliased here for existing callers).
var FormatAmount = recap.FormatAmount

// formatMinor converts a float major-unit amount to a NUMERIC string with the
// currency's decimal places.
func formatMinor(amount float64, decimalPlaces int16) string {
	return strconv.FormatFloat(amount, 'f', int(decimalPlaces), 64)
}

func currencyCodes(cs []store.Currency) []string {
	codes := make([]string, len(cs))
	for i, c := range cs {
		codes[i] = c.Code
	}
	return codes
}

func currencyByCode(cs []store.Currency, code string) *store.Currency {
	for i := range cs {
		if cs[i].Code == code {
			return &cs[i]
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func stackBytes() []byte {
	return debug.Stack()
}
