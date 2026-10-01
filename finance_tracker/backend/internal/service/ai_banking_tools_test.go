package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
)

func bankChatService(t *testing.T) (*insightService, repository.BankRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.BankConnection{}, &domain.BankAccountLink{}, &domain.BankStagedTx{}))
	repo := repository.NewBankRepository(db)
	return &insightService{bankRepo: repo}, repo
}

func stagedRow(comment string, state string) *domain.BankStagedTx {
	return &domain.BankStagedTx{
		LinkID: 1, ExternalID: "eb:1:" + comment,
		RawPayee: "", RawDetails: "PIRKINYS 2026.09.29 23.41 EUR (104583) LIDL SNIPISKES",
		RawAmount: 23.41, RawDK: "D", RawCurrency: "EUR",
		Date: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), Amount: 23.41,
		Type: domain.TransactionTypeExpense, Category: "Finance",
		Comment: comment, DebitAccount: "swed",
		Verdict: domain.VerdictNew, State: state,
	}
}

// The raw narrative is the whole reason this tool is worth having: a card
// purchase misread as a bank fee can only be recognised from the text the
// bank sent, so it has to reach the model.
func TestStagedChatToolExposesTheRawNarrative(t *testing.T) {
	svc, repo := bankChatService(t)
	require.NoError(t, repo.SaveStaged(stagedRow("Swedbank card fee", domain.StagedStateStaged)))

	out, err := svc.runChatTool("get_staged_transactions", `{}`)
	require.NoError(t, err)

	var got struct {
		Total        int64 `json:"total"`
		Transactions []struct {
			ID         uint   `json:"id"`
			Comment    string `json:"comment"`
			RawDetails string `json:"raw_details"`
		} `json:"transactions"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got.Transactions, 1)
	require.Equal(t, int64(1), got.Total)
	require.Contains(t, got.Transactions[0].RawDetails, "LIDL SNIPISKES",
		"the merchant must survive into the tool result or the model cannot spot the misread")
}

// Editing fixes the proposal. It must not move the row out of the queue —
// one-by-one human approval is the feature's premise.
func TestChatEditFixesTheProposalWithoutImporting(t *testing.T) {
	svc, repo := bankChatService(t)
	require.NoError(t, repo.SaveStaged(stagedRow("Swedbank card fee", domain.StagedStateStaged)))
	rows, _, err := repo.ListStaged(repository.StagedFilter{State: domain.StagedStateStaged, Page: 1, PageSize: 10})
	require.NoError(t, err)
	id := rows[0].ID

	args, _ := json.Marshal(map[string]any{"id": id, "category": "Food", "comment": "Lidl", "labels": "groceries"})
	out, err := svc.runChatTool("update_staged_transaction", string(args))
	require.NoError(t, err)
	require.Contains(t, out, "still_awaiting_approval")

	after, err := repo.GetStagedByIDs([]uint{id})
	require.NoError(t, err)
	require.Equal(t, domain.Category("Food"), after[0].Category)
	require.Equal(t, "Lidl", after[0].Comment)
	require.Equal(t, "groceries", after[0].Labels)
	// Still waiting, still un-imported, and the audit trail is untouched.
	require.Equal(t, domain.StagedStateStaged, after[0].State)
	require.Nil(t, after[0].ImportedTxID)
	require.Contains(t, after[0].RawDetails, "LIDL SNIPISKES")
	require.Equal(t, domain.VerdictNew, after[0].Verdict, "editing must not re-derive the verdict")
}

// There is no chat tool that can put a row in the ledger, and that is the
// point. If one is ever added, this test should be the thing that argues with
// whoever adds it.
func TestNoChatToolCanCommitABankRow(t *testing.T) {
	for _, tool := range chatTools() {
		n := strings.ToLower(tool.Function.Name)
		if strings.Contains(n, "commit") || strings.Contains(n, "dismiss") ||
			strings.Contains(n, "approve") || strings.Contains(n, "import_staged") {
			t.Errorf("chat tool %q can act on the review queue — approval is the user's", tool.Function.Name)
		}
	}
	svc, _ := bankChatService(t)
	for _, name := range []string{"commit_staged", "import_staged_transactions", "dismiss_staged"} {
		if _, err := svc.runChatTool(name, `{"ids":[1]}`); err == nil {
			t.Errorf("%q should not be a tool at all", name)
		}
	}
}

// A row the user already dealt with is no longer a proposal to improve.
func TestChatEditRefusesARowThatLeftTheQueue(t *testing.T) {
	svc, repo := bankChatService(t)
	require.NoError(t, repo.SaveStaged(stagedRow("already done", domain.StagedStateImported)))
	rows, _, err := repo.ListStaged(repository.StagedFilter{State: domain.StagedStateImported, Page: 1, PageSize: 10})
	require.NoError(t, err)

	args, _ := json.Marshal(map[string]any{"id": rows[0].ID, "category": "Food"})
	_, err = svc.runChatTool("update_staged_transaction", string(args))
	require.Error(t, err)
	require.Contains(t, err.Error(), "review queue")
}

func TestChatEditRejectsBadValues(t *testing.T) {
	svc, repo := bankChatService(t)
	require.NoError(t, repo.SaveStaged(stagedRow("x", domain.StagedStateStaged)))
	rows, _, err := repo.ListStaged(repository.StagedFilter{State: domain.StagedStateStaged, Page: 1, PageSize: 10})
	require.NoError(t, err)
	id := rows[0].ID

	for _, tc := range []struct{ name, args, wantErr string }{
		{"unknown account", `{"id":%d,"debit_account":"monzo"}`, "monzo"},
		{"bad date", `{"id":%d,"date":"29/09/2026"}`, "YYYY-MM-DD"},
		{"bad type", `{"id":%d,"type":"refund"}`, "expense"},
		{"nothing to do", `{"id":%d}`, "at least one field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := strings.Replace(tc.args, "%d", strings.TrimSpace(jsonNum(id)), 1)
			_, err := svc.runChatTool("update_staged_transaction", args)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// An install with no bank connected should say so, not panic on a nil repo.
func TestBankingChatToolsAreInertWithoutAConnection(t *testing.T) {
	svc := &insightService{}
	for _, name := range []string{"get_staged_transactions", "get_bank_connections", "update_staged_transaction"} {
		_, err := svc.runChatTool(name, `{"id":1,"category":"Food"}`)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no bank is connected")
	}
}

func jsonNum(v uint) string {
	b, _ := json.Marshal(v)
	return string(b)
}
