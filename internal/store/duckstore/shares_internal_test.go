// White-box: the walk query casts DECIMAL columns to VARCHAR, so DuckDB never returns text that
// is not a number; only a fake rowQuerier can reach the parse refusal.
package duckstore

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

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
		for i, value := range []any{"acct", "sec", r.action, r.shares, r.splitNew, r.splitOld} {
			reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(value))
		}
		return nil
	})
}

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

			_, err := holdingShares(t.Context(), c.row)

			require.ErrorIs(t, err, errNotDecimal)
			assert.ErrorContains(t, err, `"n/a"`)
		})
	}
}

func Test_holding_shares_reads_numeric_decimal_columns(t *testing.T) {
	t.Parallel()
	row := walkRow{action: "buy", shares: text("1.5")}

	counts, err := holdingShares(t.Context(), row)

	require.NoError(t, err)
	assert.Equal(t, "3/2", counts[holdingKey{account: "acct", security: "sec"}].String())
}
