package cli_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_findings_csv_gives_every_record_the_header_width_with_the_investment_cells_empty_but_for_shares_without_cost(t *testing.T) {
	txn := "txn-1"
	fixedAt := csvFindingDay(9)
	fake := fakeReportStore{findings: store.FindingList{
		Findings: []store.Finding{
			{ID: "duplicate:txn-1+txn-2", Type: finding.Duplicate, FirstFoundAt: csvFindingDay(1), FixedAt: &fixedAt},
			{
				ID: "uncategorized:no-payee", Type: finding.Uncategorized, FirstFoundAt: csvFindingDay(1),
				Items: []store.FindingItem{{TransactionID: &txn, Date: csvFindingDay(2), Account: "Visa", Currency: "CAD", Amount: -1000}},
			},
		},
		Accounts: []store.Account{{ID: "acct-1", Name: "Margin, USD", Type: store.AccountTypeBrokerage, Currency: "USD"}},
		Investments: store.Investments{
			Securities: []store.Security{{ID: "sec-4", Name: `XEQT "core"`}},
			Transactions: []store.InvestmentTransaction{{
				ID: "itxn-7", AccountID: "acct-1", SecurityID: new("sec-4"), Date: csvFindingDay(3),
				Action: store.ActionAddShares, Shares: new(int64(1_500_000)), Currency: "CAD",
			}},
		},
	}}
	var stdout bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &bytes.Buffer{},
		LoadConfig: func(string) (config.Config, error) { return config.Config{NonRegistered: []string{"acct-1"}}, nil },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}

	err := cli.Execute(t.Context(), []string{"findings", "--csv", "--status", "all"}, env)

	require.NoError(t, err)
	rows, err := csv.NewReader(strings.NewReader(stdout.String())).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 4)
	width := len(strings.Split(strings.TrimSuffix(findingsCSVHeader, "\n"), ","))
	for _, row := range rows {
		assert.Len(t, row, width)
	}
	investment := func(row []string) []string { return row[width-4:] }
	assert.Equal(t, []string{"investment_transaction_id", "security_id", "security", "shares"}, investment(rows[0]))
	assert.Equal(t, []string{"", "", "", ""}, investment(rows[1]), "fixed finding")
	assert.Equal(t, []string{"", "", "", ""}, investment(rows[2]), "uncategorized item")
	assert.Equal(t, []string{"itxn-7", "sec-4", `XEQT "core"`, "1.500000"}, investment(rows[3]))
	assert.Equal(t, []string{"2026-08-03", "Margin, USD", "USD"}, rows[3][3:6])
}
