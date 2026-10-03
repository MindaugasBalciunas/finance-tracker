package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/openbanking"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedHistory writes transactions straight to the DB — the point is what the
// ledger already contains, not how it got there.
func (e *bankTestEnv) seedHistory(t *testing.T, txs ...domain.Transaction) {
	t.Helper()
	for i := range txs {
		if txs[i].Date.IsZero() {
			txs[i].Date = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		}
		require.NoError(t, e.db.Create(&txs[i]).Error)
	}
}

func (e *bankTestEnv) seedRule(t *testing.T, label, category, commentMatch string) {
	t.Helper()
	require.NoError(t, e.db.Create(&domain.LabelRule{
		Label: label, Category: category, CommentMatch: commentMatch,
	}).Error)
}

func (e *bankTestEnv) stagedByComment(t *testing.T, comment string) domain.BankStagedTx {
	t.Helper()
	for _, r := range e.staged(t) {
		if r.Comment == comment {
			return r
		}
	}
	t.Fatalf("no staged row with comment %q", comment)
	return domain.BankStagedTx{}
}

// A merchant the hardcoded classifier has never heard of lands in its
// catch-all "Entertainment" bucket. The user's own ledger knows better, and
// that is the whole complaint about bank imports: the form would have offered
// Food, the importer offered Entertainment.
func TestEnrichCategoryFromHistory(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t,
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora", Amount: 41.10},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora", Amount: 28.90},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora", Amount: 52.00},
	)

	env.stage(t, []openbanking.Transaction{
		ebRow("8001", "2026-09-10", "33.15", "D", "Barbora", "Pirkiniai internetu"),
	})

	row := env.stagedByComment(t, "Barbora (Pirkiniai internetu)")
	assert.Equal(t, domain.Category("Food"), row.Category, "the category similar rows actually got, not the catch-all")
	assert.Contains(t, row.EnrichNote, "3 past transactions matching \"Barbora\"")
}

// A row the classifier genuinely recognised keeps its answer: a merchant rule
// is a stronger signal than a comment-substring count, and overwriting Lidl →
// Food with whatever the LIKE query returns would be a regression.
func TestEnrichLeavesRecognisedRowsAlone(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t,
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Clothing", Comment: "Lidl"},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Clothing", Comment: "Lidl"},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Clothing", Comment: "Lidl"},
	)

	env.stage(t, bankFeed())

	for _, r := range env.staged(t) {
		if r.Comment == "'50146 LIDL SNIPISKES" {
			assert.Equal(t, domain.Category("Food"), r.Category)
			assert.Empty(t, r.EnrichNote)
			return
		}
	}
	t.Fatal("the Lidl row was not staged")
}

// Label rules are the user-taught layer the form applies on every save. The
// bank import ignored them entirely, so a synced Ignitis bill arrived without
// the "bills" label the same row typed by hand would have carried.
func TestEnrichAppliesLabelRules(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedRule(t, "bills", "Utilities", "")
	env.seedRule(t, "electricity", "", "ignitis")

	env.stage(t, bankFeed())

	row := env.stagedByComment(t, "UAB IGNITIS")
	assert.Equal(t, "bills,electricity", row.Labels)
	assert.Contains(t, row.EnrichNote, "from your rules")

	// And the labels survive the commit, so what the user approved is what
	// lands in the ledger.
	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": []uint{row.ID}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.ImportedTxIDs, 1)

	var tx domain.Transaction
	require.NoError(t, env.db.First(&tx, out.ImportedTxIDs[0]).Error)
	assert.Equal(t, "bills,electricity", tx.Labels)
}

// Rules match on category, so the category has to be resolved first —
// otherwise the rule set is matched against the classifier's placeholder and
// the row gets the wrong labels for a category it no longer has.
func TestEnrichResolvesCategoryBeforeRules(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedRule(t, "groceries", "Food", "")
	env.seedRule(t, "fun", "Entertainment", "")
	env.seedHistory(t,
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora"},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora"},
	)

	env.stage(t, []openbanking.Transaction{
		ebRow("8002", "2026-09-10", "33.15", "D", "Barbora", "Pirkiniai internetu"),
	})

	row := env.stagedByComment(t, "Barbora (Pirkiniai internetu)")
	assert.Equal(t, domain.Category("Food"), row.Category)
	assert.Equal(t, "groceries", row.Labels, "the placeholder category must not get to pick the labels")
}

// Correcting the category in the review queue is exactly when a rule should
// start applying — the same thing editing a saved transaction does.
func TestStagedEditReappliesRules(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedRule(t, "groceries", "Food", "")

	env.stage(t, []openbanking.Transaction{
		ebRow("8003", "2026-09-10", "19.99", "D", "Some Shop", "Pirkiniai"),
	})
	row := env.stagedByComment(t, "Some Shop (Pirkiniai)")
	require.Empty(t, row.Labels)

	rec := bankJSON(t, env.r, "PUT", stagedPath(row.ID), map[string]any{"category": "Food"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out stagedResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "groceries", out.Labels)
}

// A label the user took off must stay off. A rule that already matched before
// the edit does not get a second chance to put it back.
func TestStagedEditDoesNotResurrectRemovedLabels(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedRule(t, "bills", "Utilities", "")

	env.stage(t, bankFeed())
	row := env.stagedByComment(t, "UAB IGNITIS")
	require.Equal(t, "bills", row.Labels)

	// Remove it...
	rec := bankJSON(t, env.r, "PUT", stagedPath(row.ID), map[string]any{"labels": ""})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// ...then make an unrelated edit. The rule still matches, but it matched
	// before the edit too, so it stays silent.
	rec = bankJSON(t, env.r, "PUT", stagedPath(row.ID), map[string]any{"comment": "Ignitis electricity"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out stagedResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out.Labels)
}

// Enrichment moves the category and the labels. It must not touch the four
// fields content dedup keys on, or a row already in the ledger from a CSV
// import stops looking like a duplicate and ships twice.
func TestEnrichLeavesDedupKeyIntact(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedRule(t, "groceries", "Entertainment", "")
	env.seedHistory(t,
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora"},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora"},
		// The row the bank is about to re-send, already in the ledger.
		domain.Transaction{
			Date:   time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
			Type:   domain.TransactionTypeExpense,
			Amount: 33.15, Category: "Food", Comment: "Barbora (Pirkiniai internetu)",
		},
	)

	env.stage(t, []openbanking.Transaction{
		ebRow("8004", "2026-09-10", "33.15", "D", "Barbora", "Pirkiniai internetu"),
	})

	row := env.stagedByComment(t, "Barbora (Pirkiniai internetu)")
	assert.Equal(t, domain.VerdictDuplicateContent, row.Verdict, "enrichment must not break content dedup")
	assert.False(t, row.Preticked())
	assert.Equal(t, "2026-09-10", row.Date.Format("2006-01-02"))
	assert.Equal(t, 33.15, row.Amount)
	assert.Equal(t, domain.TransactionTypeExpense, row.Type)
}

// The amount fallback is comment-independent, so it must not answer before
// the merchant has been looked up. A €9.90 haircut came back Utilities off
// five unrelated €9.90 rows while "Barbara beauty" sat four times under
// Health.
func TestEnrichPrefersMerchantOverAmount(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t,
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Health", Comment: "Barbara beauty. haircut", Amount: 25},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Health", Comment: "Barbara beauty. haircut", Amount: 30},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Utilities", Comment: "Something else", Amount: 9.90},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Utilities", Comment: "Another thing", Amount: 9.90},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Utilities", Comment: "A third", Amount: 9.90},
	)

	env.stage(t, []openbanking.Transaction{
		ebRow("8005", "2026-09-10", "9.90", "D", "Barbara beauty", "Kirpykla"),
	})

	row := env.stagedByComment(t, "Barbara beauty (Kirpykla)")
	assert.Equal(t, domain.Category("Health"), row.Category)
	assert.Contains(t, row.EnrichNote, `matching "Barbara beauty"`)
}

// With no merchant match at all, the amount fallback still answers — that is
// what catches subscriptions whose descriptions never repeat.
func TestEnrichFallsBackToAmount(t *testing.T) {
	env := bankTestRouter(t, true)
	for i := 0; i < 3; i++ {
		env.seedHistory(t, domain.Transaction{
			Type: domain.TransactionTypeExpense, Category: "Subscriptions",
			Comment: "Some subscription", Amount: 11.99,
		})
	}

	env.stage(t, []openbanking.Transaction{
		ebRow("8006", "2026-09-10", "11.99", "D", "NEWCO BILLING", "Monthly"),
	})

	row := env.stagedByComment(t, "NEWCO BILLING (Monthly)")
	assert.Equal(t, domain.Category("Subscriptions"), row.Category)
	assert.Contains(t, row.EnrichNote, "exactly this amount")
}

// SQLite's LOWER() is ASCII-only, so a lowered pattern could never match a
// Lithuanian merchant name — which is most of them.
func TestEnrichMatchesLithuanianNames(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t,
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Health", Comment: "INDRA MASIOKIENĖ (5 Emosesijos)"},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Health", Comment: "INDRA MASIOKIENĖ (3 Emosesijos)"},
	)

	env.stage(t, []openbanking.Transaction{
		ebRow("8007", "2026-09-10", "35.00", "D", "INDRA MASIOKIENĖ", "Masazas"),
	})

	row := env.stagedByComment(t, "INDRA MASIOKIENĖ (Masazas)")
	assert.Equal(t, domain.Category("Health"), row.Category)
}

// The bank's own narrative must not invent labels. A grocery run described
// as "Barbora (Pirkiniai internetu)" was picking up an "internet" rule off
// the suffix — a label the same purchase typed by hand would never get.
func TestEnrichRulesIgnoreTheBankNarrative(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedRule(t, "groceries", "", "barbora")
	env.seedRule(t, "internet", "", "internet")

	env.stage(t, []openbanking.Transaction{
		ebRow("8008", "2026-09-10", "41.20", "D", "Barbora", "Pirkiniai internetu"),
	})

	row := env.stagedByComment(t, "Barbora (Pirkiniai internetu)")
	assert.Equal(t, "groceries", row.Labels)
}

// Without a labeling source the import behaves exactly as it did before —
// the feature degrades, nothing breaks.
func TestEnrichIsOptional(t *testing.T) {
	env := bankTestRouter(t, true)
	env.h.labeling = nil
	env.seedRule(t, "bills", "Utilities", "")

	env.stage(t, bankFeed())
	row := env.stagedByComment(t, "UAB IGNITIS")
	assert.Empty(t, row.Labels)
	assert.Empty(t, row.EnrichNote)
}

// stagedPath is the edit endpoint for one row.
func stagedPath(id uint) string {
	return "/api/v1/banking/staged/" + strconv.FormatUint(uint64(id), 10)
}

// Label rules only cover what the user wrote a rule for. Everything else is
// in the history: a merchant tagged the same way seven times should not come
// back from the bank bare.
func TestEnrichCopiesLabelsFromSimilarHistory(t *testing.T) {
	env := bankTestRouter(t, true)
	for i := 0; i < 4; i++ {
		env.seedHistory(t, domain.Transaction{
			Type: domain.TransactionTypeExpense, Category: "Food",
			Comment: "Barbora delivery", Labels: "groceries,barbora",
		})
	}

	env.stage(t, []openbanking.Transaction{
		ebRow("8101", "2026-09-10", "41.20", "D", "Barbora", "Pirkiniai internetu"),
	})

	row := env.stagedByComment(t, "Barbora (Pirkiniai internetu)")
	assert.Equal(t, "groceries,barbora", row.Labels)
	assert.Contains(t, row.EnrichNote, "on 4 similar past transactions")
}

// Agreement, not presence. A label stuck on one row by accident is not a
// pattern, and copying it forward would spread the mistake.
func TestEnrichIgnoresOneOffLabels(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t,
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora delivery", Labels: "groceries"},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora delivery", Labels: "groceries"},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora delivery", Labels: "groceries,oops"},
	)

	env.stage(t, []openbanking.Transaction{
		ebRow("8102", "2026-09-10", "41.20", "D", "Barbora", "Pirkiniai internetu"),
	})

	row := env.stagedByComment(t, "Barbora (Pirkiniai internetu)")
	assert.Equal(t, "groceries", row.Labels)
}

// A rule and the history can both have something to say; neither erases the
// other, and nothing is added twice.
func TestEnrichMergesRulesAndHistory(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedRule(t, "groceries", "", "barbora")
	env.seedHistory(t,
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora delivery", Labels: "groceries,barbora"},
		domain.Transaction{Type: domain.TransactionTypeExpense, Category: "Food", Comment: "Barbora delivery", Labels: "groceries,barbora"},
	)

	env.stage(t, []openbanking.Transaction{
		ebRow("8103", "2026-09-10", "41.20", "D", "Barbora", "Pirkiniai internetu"),
	})

	row := env.stagedByComment(t, "Barbora (Pirkiniai internetu)")
	assert.Equal(t, "groceries,barbora", row.Labels)
	assert.Contains(t, row.EnrichNote, "from your rules")
	assert.Contains(t, row.EnrichNote, "similar past transactions")
}

// TestMergeIntoHandEnteredTransaction — the case the live sync hit.
//
// A €65.20 Maxima purchase typed in as "Maxima food" and the same purchase
// arriving from the bank as "MAXIMA" differ in the one field content dedup
// cannot do without, so the queue offered it as new and the ledger would have
// got it twice.
func TestMergeIntoHandEnteredTransaction(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t, domain.Transaction{
		Date:   time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		Type:   domain.TransactionTypeExpense,
		Amount: 65.20, Category: "Food", Comment: "Maxima food",
		Labels: "maxima,groceries", DebitAccount: "swed",
	})

	env.stage(t, []openbanking.Transaction{
		ebRow("m1", "2026-10-02", "65.20", "D", "", "PIRKINYS 516793******2669 02.10.26 17:34 65.20 EUR (134851) MAXIMA/X-787 MAXIMA Vilnius 000LT"),
	})

	rec := bankJSON(t, env.r, "GET", "/api/v1/banking/staged", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var page struct {
		Transactions []stagedResponse `json:"transactions"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.Transactions, 1)

	row := page.Transactions[0]
	// The merchant was recovered, so this is no longer a "card fee".
	assert.Equal(t, domain.Category("Food"), row.Category)
	require.NotNil(t, row.MergeCandidate, "the hand-entered row must be offered")
	assert.Equal(t, "Maxima food", row.MergeCandidate.Comment)

	// Merging keeps the user's own words and stamps the bank reference on.
	rec = bankJSON(t, env.r, "POST", stagedPath(row.ID)+"/merge",
		map[string]any{"transaction_id": row.MergeCandidate.ID})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var tx domain.Transaction
	require.NoError(t, env.db.First(&tx, row.MergeCandidate.ID).Error)
	assert.Equal(t, "Maxima food", tx.Comment, "the user's description survives")
	assert.Equal(t, "maxima,groceries", tx.Labels)
	assert.Equal(t, row.ExternalID, tx.ExternalID)

	var n int64
	require.NoError(t, env.db.Model(&domain.Transaction{}).Count(&n).Error)
	assert.EqualValues(t, 1, n, "merging must not create a second transaction")

	// And the next sync recognises it on the provider id instead of offering
	// it again — which is the whole point of stamping the reference.
	res := env.stage(t, []openbanking.Transaction{
		ebRow("m1", "2026-10-02", "65.20", "D", "", "PIRKINYS 516793******2669 02.10.26 17:34 65.20 EUR (134851) MAXIMA/X-787 MAXIMA Vilnius 000LT"),
	})
	assert.Equal(t, 0, res.StagedNew)
	staged, _, err := env.repo.ListStaged(repository.StagedFilter{State: domain.StagedStateStaged})
	require.NoError(t, err)
	assert.Empty(t, staged)
}

// A merge is a judgement call made from four fields, so getting it wrong must
// cost a tap rather than a hand-edit of the database.
func TestUnmergeReturnsTheRowToTheQueue(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t, domain.Transaction{
		Date:   time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		Type:   domain.TransactionTypeExpense,
		Amount: 65.20, Category: "Food", Comment: "Maxima food", DebitAccount: "swed",
	})
	env.stage(t, []openbanking.Transaction{
		ebRow("m2", "2026-10-02", "65.20", "D", "MAXIMA", "Pirkiniai"),
	})
	row := env.staged(t)[0]

	var target domain.Transaction
	require.NoError(t, env.db.First(&target, "comment = ?", "Maxima food").Error)

	rec := bankJSON(t, env.r, "POST", stagedPath(row.ID)+"/merge", map[string]any{"transaction_id": target.ID})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = bankJSON(t, env.r, "POST", stagedPath(row.ID)+"/unmerge", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.NoError(t, env.db.First(&target, target.ID).Error)
	assert.Empty(t, target.ExternalID, "the ledger row is unlinked again")

	back, err := env.repo.GetStagedByExternalID(row.ExternalID)
	require.NoError(t, err)
	assert.Equal(t, domain.StagedStateStaged, back.State)
}

// Merging must not reach across accounts, amounts or more than a day — and a
// row that already belongs to another bank row is never re-pointed.
func TestMergeCandidateIsStrict(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t,
		// Right amount and day, wrong account.
		domain.Transaction{Date: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
			Type: domain.TransactionTypeExpense, Amount: 65.20, Comment: "Elsewhere", DebitAccount: "seb"},
		// Right account and day, wrong amount.
		domain.Transaction{Date: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
			Type: domain.TransactionTypeExpense, Amount: 66.00, Comment: "Close", DebitAccount: "swed"},
		// Right account and amount, a week away.
		domain.Transaction{Date: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
			Type: domain.TransactionTypeExpense, Amount: 65.20, Comment: "Last week", DebitAccount: "swed"},
	)
	env.stage(t, []openbanking.Transaction{
		ebRow("m3", "2026-10-02", "65.20", "D", "MAXIMA", "Pirkiniai"),
	})

	rows, _, err := env.repo.ListStaged(repository.StagedFilter{State: domain.StagedStateStaged})
	require.NoError(t, err)
	candidates, err := env.h.mergeCandidates(rows)
	require.NoError(t, err)
	assert.Empty(t, candidates, "none of those is the same payment")
}

// A row that looks like something already added must never arrive ticked:
// content dedup missed it (the descriptions differ), so a batch "Add" would
// create exactly the duplicate the candidate is warning about.
func TestMergeCandidateIsNeverPreticked(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t, domain.Transaction{
		Date:   time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		Type:   domain.TransactionTypeExpense,
		Amount: 65.20, Comment: "Maxima food", DebitAccount: "swed",
	})
	env.stage(t, []openbanking.Transaction{
		ebRow("m4", "2026-10-02", "65.20", "D", "MAXIMA", "Pirkiniai"),
	})

	rec := bankJSON(t, env.r, "GET", "/api/v1/banking/staged", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var page struct {
		Transactions []stagedResponse `json:"transactions"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.Transactions, 1)
	assert.NotNil(t, page.Transactions[0].MergeCandidate)
	assert.False(t, page.Transactions[0].Preticked)
}
