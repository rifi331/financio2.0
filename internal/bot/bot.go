// Package bot hosts the message dispatcher and the entry flow.
package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rifi331/financio/internal/ai"
	"github.com/rifi331/financio/internal/auth"
	"github.com/rifi331/financio/internal/recap"
	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
)

// Sender is the outbound messaging surface the dispatcher needs.
type Sender interface {
	SendMessage(chatID int64, text string) error
	SendMessages(chatID int64, parts []string) error
}

// TxStore is the persistence surface the dispatcher needs.
type TxStore interface {
	ListActiveCategories(ctx context.Context) ([]string, error)
	ListActiveCurrencies(ctx context.Context) ([]store.Currency, error)
	InsertTransaction(ctx context.Context, txn store.NewTransaction) (store.Transaction, error)
	DeleteTransactionByID(ctx context.Context, id, userID int64) (store.Transaction, bool, error)
	ListTransactions(ctx context.Context, f store.TxnFilter) ([]store.Transaction, error)
}

// Parser is the AI extraction surface.
type Parser interface {
	ParseEntry(ctx context.Context, in ai.EntryInput) (ai.ParsedEntry, error)
	ParseRecapFilter(ctx context.Context, text string, today time.Time, categories []string) (ai.RecapFilter, error)
}

// Dispatcher routes authorized messages to their intent handlers.
type Dispatcher struct {
	store    TxStore
	auth     *auth.Authorizer
	ai       Parser
	tg       Sender
	tz       *time.Location
	defCur   string
	nowFn    func() time.Time
	log      *slog.Logger
}

// NewDispatcher wires the dispatcher. nowFn is injectable for tests (nil -> time.Now).
func NewDispatcher(s TxStore, a *auth.Authorizer, p Parser, tg Sender, tz *time.Location, defaultCurrency string, nowFn func() time.Time) *Dispatcher {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Dispatcher{store: s, auth: a, ai: p, tg: tg, tz: tz, defCur: defaultCurrency,
		nowFn: nowFn, log: slog.Default()}
}

// Handle processes one raw Telegram update end to end.
func (d *Dispatcher) Handle(ctx context.Context, chatID, userID int64, firstName, lastName string, msg router.Message) {
	user, ok, err := d.auth.Authorize(ctx, userID, firstName, lastName)
	if err != nil {
		d.log.Error("authorize failed", "user_id", userID, "err", err)
		return
	}
	if !ok {
		return // dropped silently; never reply
	}

	intent := router.Classify(msg)
	switch intent.Kind {
	case router.IntentDelete:
		d.handleDelete(ctx, chatID, user, intent.DeleteID, intent.Text)
	case router.IntentRecap:
		d.handleRecap(ctx, chatID, user, intent.Text)
	case router.IntentPhoto:
		d.reply(ctx, chatID, "Photo receipts are coming in the next session.")
	case router.IntentPDF:
		d.reply(ctx, chatID, "PDF statements are coming in the next session.")
	default:
		d.handleEntry(ctx, chatID, user, intent.Text)
	}
}

func (d *Dispatcher) handleEntry(ctx context.Context, chatID int64, user store.User, text string) {
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
		return
	}

	txn, err := d.validateAndStore(ctx, user, parsed, categories, currencies, now, raw)
	if err != nil {
		d.log.Warn("validation failed", "err", err, "raw", text)
		d.reply(ctx, chatID, parseFailHint)
		return
	}

	d.reply(ctx, chatID, formatConfirmation(txn, currencies, firstName(user.DisplayName), lang))
}

const parseFailHint = "Could not parse that — try e.g. '10rm nasi lemak' / 'gaji 5000 bulan ini'"

// handleDelete deletes by id scoped to the requesting user. Not-found and
// not-owned are indistinguishable in the reply ("Record not found!") so we
// never reveal whether an id exists for someone else.
func (d *Dispatcher) handleDelete(ctx context.Context, chatID int64, user store.User, id int64, text string) {
	lang := detectLanguage(text)
	txn, ok, err := d.store.DeleteTransactionByID(ctx, id, user.ID)
	if err != nil {
		d.log.Error("delete failed", "err", err, "id", id)
		d.reply(ctx, chatID, recapFailHint(lang))
		return
	}
	if !ok {
		if lang == "ID" {
			d.reply(ctx, chatID, "Catatan tidak ditemukan!")
		} else {
			d.reply(ctx, chatID, "Record not found!")
		}
		return
	}
	if lang == "ID" {
		d.reply(ctx, chatID, fmt.Sprintf("🗑️ item dengan id %d sudah dihapus\n🛍️: %s", id, txn.Item))
	} else {
		d.reply(ctx, chatID, fmt.Sprintf("🗑️ item deleted for id: %d\n🛍️: %s", id, txn.Item))
	}
}

// handleRecap: the AI returns a structured filter only; Go builds the
// parameterized query. No model output ever reaches SQL as text.
func (d *Dispatcher) handleRecap(ctx context.Context, chatID int64, user store.User, text string) {
	categories, err := d.store.ListActiveCategories(ctx)
	if err != nil {
		d.log.Error("load categories failed", "err", err)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}

	now := d.nowFn().In(d.tz)
	filter, err := d.ai.ParseRecapFilter(ctx, text, now, categories)
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

	f := store.TxnFilter{UserID: user.ID, From: from, To: to, Categories: validCats}
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
		return
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
	}
}

func recapFailHint(lang string) string {
	if lang == "ID" {
		return "Maaf, permintaan rekap tidak bisa diproses. Coba misalnya 'recap nov 2025' atau 'rekap bulan ini'."
	}
	return "Sorry, I couldn't process that recap request. Try e.g. 'recap nov 2025' or 'rekap bulan ini'."
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

func (d *Dispatcher) validateAndStore(ctx context.Context, user store.User, p ai.ParsedEntry, categories []string, currencies []store.Currency, now time.Time, raw string) (store.Transaction, error) {
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
		Source:       "text",
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
