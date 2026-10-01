package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// TransactionScan is the AI's extraction of one transaction from a photo
// (receipt, bank-app screenshot). Every field is a suggestion the user
// reviews in the form before saving — nothing is persisted here.
type TransactionScan struct {
	Type     string   `json:"type"`     // expense | income | investment
	Date     string   `json:"date"`     // YYYY-MM-DD
	Amount   float64  `json:"amount"`   // EUR, positive
	Comment  string   `json:"comment"`  // merchant / description
	Category string   `json:"category"` // one of the known categories, or ""
	Labels   []string `json:"labels"`   // from the user's vocabulary
	// Account linkage recognized from the image (e.g. a known bank-app look);
	// setting these makes the created transaction adjust that balance.
	DebitAccount  string `json:"debit_account"`
	CreditAccount string `json:"credit_account"`
	Note          string `json:"note"` // one-line explanation / caveats
}

// visionMediaTypes is the set the Anthropic vision API accepts.
var visionMediaTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true,
}

// ScanTransaction sends an uploaded image to the vision model and returns the
// extracted transaction fields for the form to prefill. Read-only: it never
// writes a transaction.
func (s *insightService) ScanTransaction(ctx context.Context, imageData []byte, mediaType string) (*TransactionScan, error) {
	settings, err := s.repo.GetAISettings()
	if err != nil {
		return nil, err
	}
	if err := requireAI(settings); err != nil {
		return nil, err
	}
	if len(imageData) == 0 {
		return nil, errors.New("empty image")
	}
	if !visionMediaTypes[mediaType] {
		return nil, fmt.Errorf("unsupported image type %q — use JPEG, PNG, WebP or GIF", mediaType)
	}

	// Category and label vocabularies so the model maps onto what the app
	// already uses instead of inventing new ones.
	var cats []string
	for _, c := range domain.ValidCategories {
		cats = append(cats, string(c))
	}
	vocab, _, _ := s.labelVocabulary(60)

	prompt := fmt.Sprintf(`You are extracting ONE financial transaction from an image (a receipt, invoice, or a bank/payment-app screenshot) for a personal finance tracker. Amounts are in EUR; if the image shows another currency, still return the numeric amount you see and note the currency in "note".

Return ONE JSON object, nothing else:
{"type":"expense|income|investment","date":"YYYY-MM-DD","amount":<positive number>,"comment":"<merchant or short description>","category":"<one of the categories below, or empty if unsure>","labels":["<0-3 labels from the list below>"],"debit_account":"<account code or empty>","credit_account":"<account code or empty>","note":"<one short sentence: what you read, and any caveat like an unclear total or a non-EUR amount>"}

CATEGORIES (pick the best fit, or "" if genuinely unclear):
%s

LABELS (use only these, or none):
%s

ACCOUNTS (codes: seb, swed, swed_etf, seb_pen, luminor, art, rev_m, rev_r, rev_stocks, ibkr_stocks, cash — leave empty when unsure):
- A screenshot of "PzM sąskaita" (the user's bank app view, usually black with orange accents) is the user's SWEDBANK account: use "swed" — debit_account for an expense, credit_account for income.
- Revolut-app screenshots are rev_m or rev_r (leave empty if you can't tell which).
- Plain paper receipts don't show the paying account — leave both empty.

Rules: amount is the transaction total (the amount actually charged), as a positive number. Most receipts are expenses. If the date isn't visible, use an empty string. Never invent a merchant — if unreadable, say so in note and leave comment short.`,
		strings.Join(cats, ", "), strings.Join(vocab, ", "))

	req := anthropicRequest{
		Model:     settings.Model,
		MaxTokens: 1024,
		Messages: []anthropicMessage{{
			Role: "user",
			Content: []anthropicBlock{
				{Type: "image", Source: &anthropicImageSource{
					Type: "base64", MediaType: mediaType,
					Data: base64.StdEncoding.EncodeToString(imageData),
				}},
				{Type: "text", Text: prompt},
			},
		}},
	}

	msg, err := postAnthropic(ctx, settings, req)
	if err != nil {
		return nil, fmt.Errorf("calling AI gateway: %w", err)
	}

	var out TransactionScan
	if err := json.Unmarshal([]byte(extractJSON(msg.Content, '{', '}')), &out); err != nil {
		return nil, errors.New("the model could not read a transaction from that image — try a clearer photo")
	}

	// Server-side validation: the form gets sane, app-valid values only.
	out.Type = strings.ToLower(strings.TrimSpace(out.Type))
	if out.Type != "income" && out.Type != "investment" {
		out.Type = "expense"
	}
	if out.Amount < 0 {
		out.Amount = -out.Amount // a receipt total is positive regardless of sign
	}
	if cat, ok := normalizeScanCategory(out.Category); ok {
		out.Category = cat
	} else {
		out.Category = ""
	}
	if out.Date != "" {
		if _, derr := time.Parse("2006-01-02", out.Date); derr != nil {
			out.Date = "" // let the form default to today
		}
	}
	out.Comment = strings.TrimSpace(out.Comment)
	out.Labels = normalizeSuggested(out.Labels, nil, 3)
	// Accounts: only real codes survive; anything else clears silently (the
	// form leaves its default).
	if a, ok := normalizeAccountCode(out.DebitAccount); ok {
		out.DebitAccount = a
	} else {
		out.DebitAccount = ""
	}
	if a, ok := normalizeAccountCode(out.CreditAccount); ok {
		out.CreditAccount = a
	} else {
		out.CreditAccount = ""
	}
	out.Note = strings.TrimSpace(clipText(out.Note, 240))

	s.logAIActivity("scan", out.Type, fmt.Sprintf("scanned image → %s €%.2f %q (%s)", out.Type, out.Amount, out.Comment, out.Category))
	return &out, nil
}

// normalizeScanCategory maps a model-proposed category onto a real one
// (case-insensitive), returning ok=false when it matches nothing.
func normalizeScanCategory(c string) (string, bool) {
	c = strings.TrimSpace(c)
	if c == "" {
		return "", false
	}
	for _, v := range domain.ValidCategories {
		if strings.EqualFold(string(v), c) {
			return string(v), true
		}
	}
	return "", false
}
