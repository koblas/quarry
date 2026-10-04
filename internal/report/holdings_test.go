package report_test

import (
	"context"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func holdingsOf(t *testing.T, rows []store.Holding, currency money.Currency) report.Holdings {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{holdings: store.Holdings{Holdings: rows}}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{Currency: currency})

	require.NoError(t, err)
	return result
}

// cadHolding is a holding whose value in CAD is cents; nil leaves it with no conversion.
func cadHolding(cents *big.Int) store.Holding { return store.Holding{ValueCAD: cents} }

func totalValues(result report.Holdings) []string {
	values := make([]string, len(result.Totals))
	for i, total := range result.Totals {
		values[i] = total.Currency + " " + total.Value.String()
	}
	return values
}

func Test_holdings_reads_the_store_once_for_the_day_asked_and_returns_it_with_the_currency(t *testing.T) {
	var got store.HoldingsParams
	var reads int
	asOf := time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)
	srv := report.NewServer(report.WithStore(fakeStore{gotHoldings: &got, holdingsReads: &reads}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{AsOf: asOf, Currency: money.USD})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Equal(t, store.HoldingsParams{AsOf: asOf}, got)
	assert.Equal(t, report.Holdings{AsOf: asOf, Currency: money.USD}, result)
}

func Test_holdings_keeps_the_stores_rows_in_the_stores_order(t *testing.T) {
	rows := []store.Holding{{Account: "Zeta"}, {Account: "Alpha"}}

	result := holdingsOf(t, rows, money.CAD)

	assert.Equal(t, rows, result.Rows)
}

func Test_holdings_converts_each_value_to_the_asked_currency(t *testing.T) {
	row := store.Holding{Value: big.NewInt(100), ValueCAD: big.NewInt(136), ValueUSD: big.NewInt(100)}
	cases := []struct {
		name     string
		currency money.Currency
		want     *big.Int
	}{
		{name: "CAD reads the CAD value", currency: money.CAD, want: big.NewInt(136)},
		{name: "USD reads the USD value", currency: money.USD, want: big.NewInt(100)},
		{name: "native converts nothing", currency: money.Native, want: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := report.Holdings{Currency: c.currency}

			assert.Equal(t, c.want, result.Converted(row))
		})
	}
}

func Test_holdings_total_is_in_the_asked_currency_and_sums_that_currencys_values(t *testing.T) {
	rows := []store.Holding{
		{ValueCAD: big.NewInt(37_704_00), ValueUSD: big.NewInt(1)},
		{ValueCAD: big.NewInt(33_536_72), ValueUSD: big.NewInt(2)},
	}

	assert.Equal(t, []string{"CAD 7124072"}, totalValues(holdingsOf(t, rows, money.CAD)))
	assert.Equal(t, []string{"USD 3"}, totalValues(holdingsOf(t, rows, money.USD)))
}

func Test_holdings_total_leaves_out_values_with_no_conversion(t *testing.T) {
	rows := []store.Holding{cadHolding(big.NewInt(100)), cadHolding(nil), cadHolding(big.NewInt(250))}

	result := holdingsOf(t, rows, money.CAD)

	assert.Equal(t, []string{"CAD 350"}, totalValues(result))
}

func Test_holdings_total_sums_values_past_the_int64_range(t *testing.T) {
	rows := []store.Holding{cadHolding(big.NewInt(math.MaxInt64)), cadHolding(big.NewInt(math.MaxInt64))}

	result := holdingsOf(t, rows, money.CAD)

	assert.Equal(t, []string{"CAD 18446744073709551614"}, totalValues(result))
}

func Test_holdings_total_counts_a_negative_value(t *testing.T) {
	rows := []store.Holding{cadHolding(big.NewInt(500)), cadHolding(big.NewInt(-200))}

	result := holdingsOf(t, rows, money.CAD)

	assert.Equal(t, []string{"CAD 300"}, totalValues(result))
}

func Test_holdings_total_is_zero_when_a_zero_value_is_the_only_one_that_converts(t *testing.T) {
	rows := []store.Holding{cadHolding(big.NewInt(0)), cadHolding(nil)}

	result := holdingsOf(t, rows, money.CAD)

	assert.Equal(t, []string{"CAD 0"}, totalValues(result))
}

func Test_holdings_has_no_total_when_no_row_converts(t *testing.T) {
	cases := []struct {
		name     string
		rows     []store.Holding
		currency money.Currency
	}{
		{name: "no rows", rows: nil, currency: money.CAD},
		{name: "every row unconverted", rows: []store.Holding{cadHolding(nil), cadHolding(nil)}, currency: money.CAD},
		{name: "only the other currency converts", rows: []store.Holding{cadHolding(big.NewInt(100))}, currency: money.USD},
		{name: "a native listing", rows: []store.Holding{{Value: big.NewInt(100), ValueCAD: big.NewInt(100)}}, currency: money.Native},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Empty(t, holdingsOf(t, c.rows, c.currency).Totals)
		})
	}
}

func Test_holdings_refuses_when_the_read_fails_to_open_the_store(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Holdings(t.Context(), report.HoldingsRequest{})

	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_holdings_reports_an_interrupt_during_the_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Holdings(ctx, report.HoldingsRequest{})

	assert.EqualError(t, err, "holdings interrupted")
}

func Test_holdings_returns_any_other_read_failure_unchanged(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Holdings(t.Context(), report.HoldingsRequest{})

	assert.Equal(t, errDiskRead, err)
}
