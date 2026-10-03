package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/require"
)

// searchSplitJSON is one entry of a search transaction's "splits".
type searchSplitJSON struct {
	Category *string `json:"category"`
	Memo     *string `json:"memo"`
	Amount   string  `json:"amount"`
	Transfer bool    `json:"transfer"`
}

// searchTransactionJSON is one entry of the search document's "transactions".
type searchTransactionJSON struct {
	TransactionID string            `json:"transaction_id"`
	Date          string            `json:"date"`
	AccountID     string            `json:"account_id"`
	Account       string            `json:"account"`
	Payee         *string           `json:"payee"`
	Memo          *string           `json:"memo"`
	Amount        string            `json:"amount"`
	Currency      string            `json:"currency"`
	Transfer      bool              `json:"transfer"`
	Excluded      bool              `json:"excluded"`
	Splits        []searchSplitJSON `json:"splits"`
}

// searchJSONDoc is quarry search --json's stdout.
type searchJSONDoc struct {
	Since         *string                 `json:"since"`
	Until         *string                 `json:"until"`
	AccountFilter []recurringIDName       `json:"account_filter"`
	Text          *string                 `json:"text"`
	Category      *string                 `json:"category"`
	Min           *string                 `json:"min"`
	Max           *string                 `json:"max"`
	Limit         int                     `json:"limit"`
	Matched       int                     `json:"matched"`
	Truncated     bool                    `json:"truncated"`
	Transactions  []searchTransactionJSON `json:"transactions"`
	Warnings      []string                `json:"warnings"`
}

// decodeSearchJSON reads stdout as the search document, refusing keys the document does not rule.
func decodeSearchJSON(t *testing.T, stdout string) searchJSONDoc {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	decoder.DisallowUnknownFields()
	var doc searchJSONDoc

	require.NoError(t, decoder.Decode(&doc), stdout)
	return doc
}

// searchSplit is one split of a searchTxn: "" category or memo means none, and transferTo is the
// account id of the other leg of a transfer; sourceID orders the splits of one transaction.
type searchSplit struct {
	category, memo, transferTo string
	sourceID                   int64
	cents                      int64
}

// searchTxn is one transaction of any number of splits, listed with the source id it carries:
// "" payee or memo means none, and the payee is stored under the id "payee-<name>".
type searchTxn struct {
	id, account, payee, memo string
	sourceID                 int64
	day                      time.Time
	excluded                 bool
	splits                   []searchSplit
}

// searchRows is spendRows' reference data (without its payees) plus txns in the order given, each
// priced in its account's currency at the sum of its splits, and a transfers row for each pair of
// split ids in pairs (lower source id first).
func searchRows(accounts []store.Account, pairs [][2]string, txns ...searchTxn) store.Rows {
	rows := spendRows(accounts)
	rows.Payees = nil
	currencies := map[string]string{}
	for _, a := range accounts {
		currencies[a.ID] = a.Currency
	}
	known := map[string]bool{}
	for _, tx := range txns {
		var payeeID *string
		if tx.payee != "" {
			id := "payee-" + tx.payee
			if !known[id] {
				known[id] = true
				rows.Payees = append(rows.Payees, store.Payee{ID: id, SourceID: int64(len(rows.Payees) + 1), Name: tx.payee})
			}
			payeeID = &id
		}
		var amount int64
		for j, sp := range tx.splits {
			rows.Splits = append(rows.Splits, store.Split{
				ID: fmt.Sprintf("split-%s-%d", tx.id, j), SourceID: sp.sourceID, TransactionID: "txn-" + tx.id,
				CategoryID: optional(sp.category), Memo: optional(sp.memo), Amount: sp.cents,
				TransferAccountID: optional(sp.transferTo),
			})
			amount += sp.cents
		}
		rows.Transactions = append(rows.Transactions, store.Transaction{
			ID: "txn-" + tx.id, SourceID: tx.sourceID, AccountID: tx.account, Date: tx.day, Amount: amount,
			Currency: currencies[tx.account], Status: "uncleared", PayeeID: payeeID, Memo: optional(tx.memo),
			ExcludedFromReports: tx.excluded,
		})
	}
	for i, pair := range pairs {
		to := pair[1]
		rows.Transfers = append(rows.Transfers, store.Transfer{ID: fmt.Sprintf("transfer-%d", i), FromSplitID: pair[0], ToSplitID: &to})
	}
	return rows
}

// optional is nil for "", else a pointer to s.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
