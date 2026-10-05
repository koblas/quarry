// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRates is a duckstore.RatesSource that returns its rates to every request, except those the request already has.
type fakeRates struct{ rates []store.Rate }

func (f fakeRates) Refresh(_ context.Context, req store.RatesRequest) (store.RatesRefresh, error) {
	var fresh []store.Rate
	for _, r := range f.rates {
		if r.Date.Before(req.Have.First) || r.Date.After(req.Have.Last) {
			fresh = append(fresh, r)
		}
	}
	return store.RatesRefresh{Rates: fresh, Added: len(fresh)}, nil
}

// replaceStoreWithRates is replaceStore with rates fetched into the store, skipping Quicken and the network.
func replaceStoreWithRates(t *testing.T, home string, rows store.Rows, rates ...store.Rate) {
	t.Helper()
	_, err := duckstore.New(storeDirUnder(home), duckstore.WithRates(fakeRates{rates: rates})).Replace(context.Background(), rows)
	require.NoError(t, err)
}

func Test_run_sql_views_carry_each_amount_converted_at_its_dates_rate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	day := func(month time.Month, d int) time.Time { return time.Date(2026, month, d, 0, 0, 0, 0, time.UTC) }
	replaceStoreWithRates(t, home,
		spendRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			spendSplit{id: "c0", account: "acct-cad", currency: "CAD", day: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), cents: -500},
			spendSplit{id: "u0", account: "acct-usd", currency: "USD", day: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), cents: -700},
			spendSplit{id: "u1", account: "acct-usd", currency: "USD", day: day(time.January, 2), cents: 10},
			spendSplit{id: "u2", account: "acct-usd", currency: "USD", day: day(time.January, 3), cents: -10},
			spendSplit{id: "c1", account: "acct-cad", currency: "CAD", day: day(time.January, 5), cents: -20},
			spendSplit{id: "c2", account: "acct-cad", currency: "CAD", day: day(time.January, 6), cents: 20},
		),
		store.Rate{Date: day(time.January, 2), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(time.January, 5), USDCAD: money.Rate(1_600_000), Series: "FXUSDCAD"},
	)
	const query = `SELECT source, id, native, in_cad, in_usd, cad_type, usd_type FROM (
		SELECT 'balance' AS source, id, balance AS native, balance_cad AS in_cad, balance_usd AS in_usd,
			typeof(balance_cad) AS cad_type, typeof(balance_usd) AS usd_type FROM v_account_balances
		UNION ALL
		SELECT 'cash_flow', split_id, amount, amount_cad, amount_usd, typeof(amount_cad), typeof(amount_usd) FROM v_cash_flow
		UNION ALL
		SELECT 'spending', split_id, spent, spent_cad, spent_usd, typeof(spent_cad), typeof(spent_usd) FROM v_spending
	) ORDER BY source, id`
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "--csv", query}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"source,id,native,in_cad,in_usd,cad_type,usd_type\n"+
		"balance,acct-cad,-5.00,-5.00,-3.13,\"DECIMAL(38,2)\",\"DECIMAL(38,2)\"\n"+
		"balance,acct-usd,-7.00,-11.20,-7.00,\"DECIMAL(38,2)\",\"DECIMAL(38,2)\"\n"+
		"cash_flow,split-c0,-5.00,-5.00,,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"cash_flow,split-c1,-0.20,-0.20,-0.13,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"cash_flow,split-c2,0.20,0.20,0.13,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"cash_flow,split-u0,-7.00,,-7.00,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"cash_flow,split-u1,0.10,0.13,0.10,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"cash_flow,split-u2,-0.10,-0.13,-0.10,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"spending,split-c0,5.00,5.00,,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"spending,split-c1,0.20,0.20,0.13,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"spending,split-u0,7.00,,7.00,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"spending,split-u2,0.10,0.13,0.10,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n",
		stdout.String())
}
