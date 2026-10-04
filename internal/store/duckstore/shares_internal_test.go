// White-box: the walk query casts DECIMAL columns to VARCHAR, so DuckDB never returns text that
// is not a number; only a fake rowQuerier can reach the parse refusal.
package duckstore

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// walkRow is a rowQuerier handing the walk one row with the given column text.
type walkRow struct {
	action                     string
	shares, splitNew, splitOld sql.NullString
}

func (r walkRow) QueryRows(_ context.Context, _ string, _ []any, row func(scan func(dest ...any) error) error) error {
	return row(func(dest ...any) error {
		for i, value := range []any{"acct", "sec", walkDay, r.action, r.shares, r.splitNew, r.splitOld} {
			reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(value))
		}
		return nil
	})
}

var walkDay = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

func text(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

func Test_holding_shares_fails_on_a_decimal_column_that_is_not_a_number(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  walkRow
	}{
		{name: "shares", row: walkRow{action: "buy", shares: text("n/a")}},
		{name: "split new side", row: walkRow{action: store.ActionSplit, splitNew: text("n/a"), splitOld: text("12")}},
		{name: "split old side", row: walkRow{action: store.ActionSplit, splitNew: text("1"), splitOld: text("n/a")}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, err := holdingSpans(t.Context(), c.row)

			require.ErrorIs(t, err, errNotDecimal)
			assert.ErrorContains(t, err, `"n/a"`)
		})
	}
}

func Test_holding_shares_reads_numeric_decimal_columns(t *testing.T) {
	t.Parallel()
	row := walkRow{action: "buy", shares: text("1.5")}

	counts, err := holdingSpans(t.Context(), row)

	require.NoError(t, err)
	assert.Equal(t, "3/2", counts[holdingKey{account: "acct", security: "sec"}].final.String())
}

const oneMillion = 1_000_000

func walkTxn(source int64, security string, day time.Time, millionths int64) store.InvestmentTransaction {
	return store.InvestmentTransaction{
		ID: fmt.Sprintf("itxn-%d", source), SourceID: source, AccountID: "acct", SecurityID: &security,
		Date: day, Action: "buy", Shares: &millionths, Amount: 100, Currency: "CAD",
	}
}

func walkSplit(source int64, security string, day time.Time, newShares, oldShares int64) store.InvestmentTransaction {
	txn := walkTxn(source, security, day, 0)
	txn.Action, txn.Shares = store.ActionSplit, nil
	txn.SplitNewShares, txn.SplitOldShares = &newShares, &oldShares
	return txn
}

// walkOf loads txns into a scratch database and walks them with the real query.
func walkOf(t *testing.T, txns []store.InvestmentTransaction) map[holdingKey]*holdingWalk {
	t.Helper()
	db, err := duckdb.CreateInMemory(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(t.Context(), schemaDDL)
	require.NoError(t, err)
	rows, err := investmentTransactionRows(txns)
	require.NoError(t, err)
	require.NoError(t, appendTable(t.Context(), db, "investment_transactions", rows))

	walks, err := holdingSpans(t.Context(), db)

	require.NoError(t, err)
	return walks
}

// openMillionths is the shares of the span with no end, or 0 when the walk has none.
func openMillionths(walk *holdingWalk) int64 {
	for _, span := range walk.spans {
		if span.open {
			return span.millionths
		}
	}
	return 0
}

func Test_holding_spans_open_span_is_the_rounded_final_count(t *testing.T) {
	t.Parallel()
	day, next := walkDay, walkDay.AddDate(0, 0, 1)
	cases := []struct {
		name string
		txns []store.InvestmentTransaction
		want int64
	}{
		{name: "plain buy", txns: []store.InvestmentTransaction{walkTxn(1, "sec", day, 3*oneMillion/2)}, want: 1_500_000},
		{
			name: "split-produced repeating count",
			txns: []store.InvestmentTransaction{walkTxn(1, "sec", day, oneMillion), walkSplit(2, "sec", next, 25*oneMillion, 3*oneMillion)},
			want: 8_333_333,
		},
		{
			name: "1.5 millionths tie rounds up to the even one",
			txns: []store.InvestmentTransaction{walkTxn(1, "sec", day, 3), walkSplit(2, "sec", next, 1, 2)},
			want: 2,
		},
		{
			name: "2.5 millionths tie rounds down to the even one",
			txns: []store.InvestmentTransaction{walkTxn(1, "sec", day, 5), walkSplit(2, "sec", next, 1, 2)},
			want: 2,
		},
		{
			name: "a non-zero count that rounds to zero has no open span",
			txns: []store.InvestmentTransaction{walkTxn(1, "sec", day, 1), walkSplit(2, "sec", next, 1, 3)},
			want: 0,
		},
		{name: "negative count", txns: []store.InvestmentTransaction{walkTxn(1, "sec", day, -2*oneMillion)}, want: -2_000_000},
		{
			name: "fully sold",
			txns: []store.InvestmentTransaction{walkTxn(1, "sec", day, oneMillion), walkTxn(2, "sec", next, -oneMillion)},
			want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			walk := walkOf(t, c.txns)[holdingKey{account: "acct", security: "sec"}]

			assert.Equal(t, c.want, openMillionths(walk))
			assert.Equal(t, millionthsOf(walk.final), openMillionths(walk))
		})
	}
}
