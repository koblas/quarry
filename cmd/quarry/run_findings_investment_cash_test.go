// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_findings_leaves_investment_cash_rows_out_of_duplicate_and_unlinked_transfer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CREDITCARD", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	incomePK := b.Category(v9fixture.TagRow{Name: "Dividends", Type: new(int64(categoryKindIncome))})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	day := func(d int) time.Time { return time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC) }

	invest := func(code int64, posted time.Time, row v9fixture.TransactionRow) {
		row.Account = brokeragePK
		row.PostedDate = &posted
		row.Type = &code
		pk := b.InvestmentTransaction(row)
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount, CategoryTag: incomePK})
	}
	register := func(account int64, posted time.Time, amount string) int64 {
		pk := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &posted})
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: amount, CategoryTag: billsPK})
		return pk
	}
	invest(investmentCodeDividend, day(1), v9fixture.TransactionRow{Amount: "25.00"})
	invest(investmentCodeDividend, day(2), v9fixture.TransactionRow{Amount: "25.00"})
	invest(investmentCodeSell, day(10), v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "500.00"})
	register(chequingPK, day(11), "-500.00")
	controlDuplicateFirst := register(chequingPK, day(15), "-40.00")
	controlDuplicateSecond := register(chequingPK, day(16), "-40.00")
	controlOut := register(savingsPK, day(20), "-75.00")
	controlIn := register(visaPK, day(21), "75.00")
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))

	findingIDs := func(typ string) []string {
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(context.Background(), []string{"findings", "--json", "--type", typ}, &stdout, &stderr), stderr.String())
		var doc struct {
			Findings []struct {
				ID string `json:"id"`
			} `json:"findings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
		ids := make([]string, 0, len(doc.Findings))
		for _, f := range doc.Findings {
			ids = append(ids, f.ID)
		}
		return ids
	}

	assert.Equal(t, []string{fmt.Sprintf("duplicate:txn-%d+txn-%d", controlDuplicateFirst, controlDuplicateSecond)}, findingIDs("duplicate"))
	assert.Equal(t, []string{fmt.Sprintf("unlinked-transfer:txn-%d+txn-%d", controlOut, controlIn)}, findingIDs("unlinked-transfer"))
}
