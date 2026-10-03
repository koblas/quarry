package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_IsInvestmentAccount(t *testing.T) {
	cases := []struct {
		name        string
		accountType string
		want        bool
	}{
		{name: "brokerage is an investment account", accountType: "brokerage", want: true},
		{name: "retirement is an investment account", accountType: "retirement", want: true},
		{name: "chequing is not", accountType: "chequing", want: false},
		{name: "an empty type is not", accountType: "", want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, store.IsInvestmentAccount(c.accountType))
		})
	}
}

func Test_query_column_numeric(t *testing.T) {
	cases := []struct {
		name     string
		typeName string
		want     bool
	}{
		{name: "TINYINT", typeName: "TINYINT", want: true},
		{name: "SMALLINT", typeName: "SMALLINT", want: true},
		{name: "INTEGER", typeName: "INTEGER", want: true},
		{name: "BIGINT", typeName: "BIGINT", want: true},
		{name: "HUGEINT", typeName: "HUGEINT", want: true},
		{name: "BIGNUM", typeName: "BIGNUM", want: true},
		{name: "UTINYINT", typeName: "UTINYINT", want: true},
		{name: "USMALLINT", typeName: "USMALLINT", want: true},
		{name: "UINTEGER", typeName: "UINTEGER", want: true},
		{name: "UBIGINT", typeName: "UBIGINT", want: true},
		{name: "UHUGEINT", typeName: "UHUGEINT", want: true},
		{name: "FLOAT", typeName: "FLOAT", want: true},
		{name: "DOUBLE", typeName: "DOUBLE", want: true},
		{name: "DECIMAL", typeName: "DECIMAL(18,2)", want: true},
		{name: "a LIST of integers is not", typeName: "INTEGER[]", want: false},
		{name: "an ARRAY of integers is not", typeName: "INTEGER[2]", want: false},
		{name: "a LIST of DECIMALs is not", typeName: "DECIMAL(4,1)[]", want: false},
		{name: "VARCHAR is not", typeName: "VARCHAR", want: false},
		{name: "a STRUCT of an integer is not", typeName: `STRUCT("a" INTEGER)`, want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, store.QueryColumn{Type: c.typeName}.Numeric())
		})
	}
}

func Test_unprintable_value_error_names_the_column_and_its_type(t *testing.T) {
	err := &store.UnprintableValueError{Column: "doc", Type: "JSON"}

	assert.EqualError(t, err, `cannot print column "doc" of type JSON`)
}

func Test_query_error_reads_as_its_reason(t *testing.T) {
	err := &store.QueryError{Reason: "Binder Error: Referenced column \"x\" not found in FROM clause!"}

	assert.EqualError(t, err, "Binder Error: Referenced column \"x\" not found in FROM clause!")
}

var errDriverInterrupt = errors.New("interrupt error: interrupted")

func Test_InterruptedBy_reads_as_Interrupted_and_unwraps_to_the_context_error(t *testing.T) {
	cases := []struct {
		name  string
		end   func(t *testing.T) context.Context
		cause error
		other error
	}{
		{name: "a cancelled context", end: cancelledContext, cause: context.Canceled, other: context.DeadlineExceeded},
		{name: "an expired deadline", end: expiredContext, cause: context.DeadlineExceeded, other: context.Canceled},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := store.InterruptedBy(c.end(t), errDriverInterrupt)

			assert.Equal(t, store.Interrupted(errDriverInterrupt).Error(), err.Error())
			require.ErrorIs(t, err, store.ErrQueryInterrupted)
			require.ErrorIs(t, err, errDriverInterrupt)
			require.ErrorIs(t, err, c.cause)
			require.NotErrorIs(t, err, c.other)
		})
	}
}

func cancelledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

func expiredContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
	t.Cleanup(cancel)
	return ctx
}

func Test_SpendingGroups_lists_every_grouping_in_by_order(t *testing.T) {
	groups := store.SpendingGroups()

	assert.Equal(t, []store.SpendingGroup{store.SpendByCategory, store.SpendByPayee, store.SpendByTag, store.SpendByMonth}, groups)
}

func Test_SpendingGroup_names_each_grouping_by_its_flag_word(t *testing.T) {
	cases := []struct {
		group store.SpendingGroup
		want  string
	}{
		{group: store.SpendByCategory, want: "category"},
		{group: store.SpendByPayee, want: "payee"},
		{group: store.SpendByTag, want: "tag"},
		{group: store.SpendByMonth, want: "month"},
		{group: store.SpendingGroup(4), want: ""},
		{group: store.SpendingGroup(-1), want: ""},
	}

	for _, c := range cases {
		t.Run(c.want+" is the word", func(t *testing.T) {
			assert.Equal(t, c.want, c.group.String())
		})
	}
}

func Test_CashFlowPeriods_lists_every_period_in_by_order(t *testing.T) {
	periods := store.CashFlowPeriods()

	assert.Equal(t, []store.CashFlowPeriod{store.CashFlowByMonth, store.CashFlowByYear}, periods)
}

func Test_CashFlowPeriod_names_each_period_by_its_flag_word(t *testing.T) {
	cases := []struct {
		period store.CashFlowPeriod
		want   string
	}{
		{period: store.CashFlowByMonth, want: "month"},
		{period: store.CashFlowByYear, want: "year"},
		{period: store.CashFlowPeriod(2), want: ""},
		{period: store.CashFlowPeriod(-1), want: ""},
	}

	for _, c := range cases {
		t.Run(c.want+" is the word", func(t *testing.T) {
			assert.Equal(t, c.want, c.period.String())
		})
	}
}
