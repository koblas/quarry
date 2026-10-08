package document_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// emptySnapshot lists 2026-03-01 with no rows; first is the store's first balance.
func emptySnapshot(first time.Time) report.NetWorth {
	return report.NetWorth{
		AsOf: civil(2026, time.March, 1), Currency: money.CAD, FirstBalance: first,
		Dates: []report.NetWorthDate{{Date: civil(2026, time.March, 1)}},
	}
}

// emptyHistory lists the 2026-01-31 and 2026-02-28 month ends, from a window that starts mid-month, with no rows.
func emptyHistory(first time.Time) report.NetWorth {
	return report.NetWorth{
		AsOf: civil(2026, time.February, 28), Currency: money.CAD, FirstBalance: first,
		Window: &store.Window{Since: civil(2026, time.January, 15), Until: civil(2026, time.February, 28)},
		Dates:  []report.NetWorthDate{{Date: civil(2026, time.January, 31)}, {Date: civil(2026, time.February, 28)}},
	}
}

func zeroRow(date time.Time) store.NetWorthRow {
	return store.NetWorthRow{Date: date, Type: "chequing", Currency: "CAD", Accounts: 1, Balance: big.NewInt(0), BalanceCAD: big.NewInt(0)}
}

func Test_NetWorthWarnings_names_the_first_balance_when_no_account_has_one_yet(t *testing.T) {
	first := civil(2026, time.March, 2)
	cases := []struct {
		name string
		n    report.NetWorth
		want string
	}{
		{
			name: "snapshot", n: emptySnapshot(first),
			want: "no account has a balance on 2026-03-01; the first balance is on 2026-03-02",
		},
		{
			name: "history names the first and last month ends listed, not the window's since",
			n:    emptyHistory(first),
			want: "no account has a balance at any month end from 2026-01-31 to 2026-02-28; the first balance is on 2026-03-02",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{c.want}, document.NetWorthWarnings(c.n, document.NativeFlag))
		})
	}
}

func Test_NetWorthWarnings_says_the_reports_have_no_data_when_the_store_has_no_balance(t *testing.T) {
	cases := []struct {
		name string
		n    report.NetWorth
		want string
	}{
		{
			name: "snapshot", n: emptySnapshot(time.Time{}),
			want: "no account has a balance on 2026-03-01; no account in Quicken's reports has transactions or holdings",
		},
		{
			name: "history", n: emptyHistory(time.Time{}),
			want: "no account has a balance at any month end from 2026-01-31 to 2026-02-28; " +
				"no account in Quicken's reports has transactions or holdings",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{c.want}, document.NetWorthWarnings(c.n, document.NativeFlag))
		})
	}
}

func Test_NetWorthWarnings_gives_the_same_line_in_every_listing_currency(t *testing.T) {
	for _, currency := range []money.Currency{money.CAD, money.USD, money.Native} {
		t.Run(currency.String(), func(t *testing.T) {
			n := emptySnapshot(civil(2026, time.March, 2))
			n.Currency = currency

			assert.Equal(t, []string{"no account has a balance on 2026-03-01; the first balance is on 2026-03-02"}, document.NetWorthWarnings(n, document.NativeFlag))
		})
	}
}

func Test_NetWorthWarnings_says_nothing_for_a_date_whose_rows_all_sum_to_zero(t *testing.T) {
	n := emptySnapshot(civil(2026, time.March, 2))
	n.Dates[0].Rows = []store.NetWorthRow{zeroRow(civil(2026, time.March, 1))}

	assert.Equal(t, []string{}, document.NetWorthWarnings(n, document.NativeFlag))
}

func Test_NetWorthWarnings_says_nothing_when_one_month_end_has_a_row(t *testing.T) {
	n := emptyHistory(civil(2026, time.March, 2))
	n.Dates[1].Rows = []store.NetWorthRow{zeroRow(civil(2026, time.February, 28))}

	assert.Equal(t, []string{}, document.NetWorthWarnings(n, document.NativeFlag))
}

func Test_NetWorthWarnings_says_nothing_when_the_first_balance_is_not_after_the_last_day_listed(t *testing.T) {
	cases := []struct {
		name string
		n    report.NetWorth
	}{
		{name: "snapshot on the first balance", n: emptySnapshot(civil(2026, time.March, 1))},
		{name: "snapshot after the first balance", n: emptySnapshot(civil(2026, time.February, 1))},
		{name: "history with the first balance on its last month end", n: emptyHistory(civil(2026, time.February, 28))},
		{name: "history with the first balance between its month ends", n: emptyHistory(civil(2026, time.February, 10))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{}, document.NetWorthWarnings(c.n, document.NativeFlag))
		})
	}
}

func Test_NetWorthWarnings_says_nothing_when_no_day_is_listed(t *testing.T) {
	n := report.NetWorth{AsOf: civil(2026, time.March, 1), Currency: money.CAD, FirstBalance: civil(2026, time.March, 2)}

	assert.Equal(t, []string{}, document.NetWorthWarnings(n, document.NativeFlag))
}

func Test_NetWorthWarnings_puts_the_empty_result_line_before_a_holding_line(t *testing.T) {
	n := emptySnapshot(civil(2026, time.March, 2))
	n.Unvalued = []store.UnvaluedHolding{unpriced("a-1", "Brokerage", "s-1", "Acme", 1)}

	assert.Equal(t, []string{
		"no account has a balance on 2026-03-01; the first balance is on 2026-03-02",
		`"Brokerage" holds 1 security with no price on or before 2026-03-01, so its balance leaves it out; ` +
			`enter a price in Quicken, then run quarry sync`,
	}, document.NetWorthWarnings(n, document.NativeFlag))
}
