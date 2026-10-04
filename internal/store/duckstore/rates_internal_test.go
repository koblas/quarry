// White-box: needSpan takes the instant as a value, so fixed instants whose
// UTC and local dates differ are the only way to pin which date it reads
// without moving the process zone.
package duckstore

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_needSpan_ends_on_the_local_calendar_date_of_the_instant(t *testing.T) {
	t.Parallel()
	earliest := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	transactions := []store.Transaction{{Date: earliest}}
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "west of UTC, late evening is still the local day when UTC is the next",
			now:  time.Date(2026, 3, 14, 23, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60)),
			want: time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "east of UTC, just after local midnight is the new local day when UTC is the old",
			now:  time.Date(2026, 3, 15, 0, 30, 0, 0, time.FixedZone("UTC+9", 9*60*60)),
			want: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := needSpan(transactions, nil, c.now)

			assert.Equal(t, store.DateSpan{First: earliest, Last: c.want}, got)
		})
	}
}

func Test_needSpan_is_empty_when_there_are_no_transactions_of_either_kind(t *testing.T) {
	t.Parallel()

	got := needSpan(nil, nil, time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))

	assert.Equal(t, store.DateSpan{}, got)
}

func Test_needSpan_is_empty_when_every_transaction_is_dated_after_now(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name         string
		transactions []store.Transaction
	}{
		{name: "one transaction tomorrow", transactions: []store.Transaction{{Date: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)}}},
		{name: "several transactions later still", transactions: []store.Transaction{
			{Date: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}, {Date: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)},
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, store.DateSpan{}, needSpan(c.transactions, nil, now))
		})
	}
}

func Test_needSpan_starts_on_today_for_a_transaction_dated_today(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)

	got := needSpan([]store.Transaction{{Date: today}}, nil, time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))

	assert.Equal(t, store.DateSpan{First: today, Last: today}, got)
}

func Test_needSpan_starts_on_the_earliest_cash_or_investment_date(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	today := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)
	d1, d2, d3 := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC), time.Date(2025, 6, 5, 0, 0, 0, 0, time.UTC), time.Date(2025, 9, 5, 0, 0, 0, 0, time.UTC)
	cash := func(ds ...time.Time) []store.Transaction {
		out := make([]store.Transaction, len(ds))
		for i, d := range ds {
			out[i] = store.Transaction{Date: d}
		}
		return out
	}
	invest := func(ds ...time.Time) []store.InvestmentTransaction {
		out := make([]store.InvestmentTransaction, len(ds))
		for i, d := range ds {
			out[i] = store.InvestmentTransaction{Date: d}
		}
		return out
	}
	cases := []struct {
		name        string
		transaction []store.Transaction
		investments []store.InvestmentTransaction
		first       time.Time
	}{
		{name: "investment earlier than cash", transaction: cash(d2), investments: invest(d3, d1), first: d1},
		{name: "cash earlier than investment", transaction: cash(d3, d1), investments: invest(d2), first: d1},
		{name: "investment only", investments: invest(d2, d3), first: d2},
		{name: "cash only", transaction: cash(d2, d3), first: d2},
		{name: "equal dates", transaction: cash(d2), investments: invest(d2), first: d2},
		{name: "future investment beside past cash", transaction: cash(d2), investments: invest(today.AddDate(1, 0, 0)), first: d2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, store.DateSpan{First: c.first, Last: today}, needSpan(c.transaction, c.investments, now))
		})
	}
}

func Test_needSpan_is_empty_when_every_investment_transaction_is_dated_after_now(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	investments := []store.InvestmentTransaction{{Date: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)}}

	assert.Equal(t, store.DateSpan{}, needSpan(nil, investments, now))
}
