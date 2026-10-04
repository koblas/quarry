package document_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

var emptyAsOf = time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)

const namedNothingLine = "no holdings on 2026-03-12 in the named accounts; they have no investment transactions"

func march(day int) time.Time { return time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC) }

func Test_HoldingsWarnings_word_an_empty_result_by_what_the_store_holds(t *testing.T) {
	named := []store.Account{namedAccount("a-1", "Brokerage", store.AccountTypeBrokerage)}
	cases := []struct {
		name     string
		accounts []store.Account
		asOf     time.Time
		first    time.Time
		last     time.Time
		want     string
	}{
		{
			name: "every account, transactions on other days", asOf: march(1), first: march(2), last: march(5),
			want: "no holdings on 2026-03-01; the store's investment transactions run 2026-03-02 to 2026-03-05",
		},
		{
			name: "every account, no transactions at all", asOf: march(1),
			want: "no holdings on 2026-03-01; the store has no investment transactions",
		},
		{
			name: "named accounts, transactions on other days", accounts: named, asOf: march(1), first: march(2), last: march(5),
			want: "no holdings on 2026-03-01 in the named accounts; their investment transactions run 2026-03-02 to 2026-03-05",
		},
		{
			name: "named accounts, no transactions at all", accounts: named, asOf: march(1),
			want: "no holdings on 2026-03-01 in the named accounts; they have no investment transactions",
		},
		{
			name: "one day of transactions runs from that day to itself", asOf: march(1), first: march(2), last: march(2),
			want: "no holdings on 2026-03-01; the store's investment transactions run 2026-03-02 to 2026-03-02",
		},
		{
			name: "a day after everything was sold gives the same span line", asOf: march(9), first: march(2), last: march(5),
			want: "no holdings on 2026-03-09; the store's investment transactions run 2026-03-02 to 2026-03-05",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := report.Holdings{
				Accounts: c.accounts, AsOf: c.asOf, FirstTransaction: c.first, LastTransaction: c.last, Currency: money.CAD,
			}

			got := document.HoldingsWarnings(h)

			assert.Equal(t, []string{c.want}, got)
		})
	}
}

func Test_HoldingsWarnings_say_nothing_about_an_empty_result_when_something_is_held(t *testing.T) {
	h := report.Holdings{
		Rows: []store.Holding{holdingsTestRow()}, AsOf: march(1), FirstTransaction: march(2), LastTransaction: march(5), Currency: money.CAD,
	}

	got := document.HoldingsWarnings(h)

	assert.Empty(t, got)
}

func Test_HoldingsWarnings_put_the_non_investment_line_before_the_empty_line(t *testing.T) {
	h := report.Holdings{
		Accounts: []store.Account{namedAccount("a-1", "Chequing", accountTypeChequing)}, AsOf: emptyAsOf, Currency: money.CAD,
	}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{notInvestmentLine("Chequing"), namedNothingLine}, got)
}
