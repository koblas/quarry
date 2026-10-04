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

var (
	beforeFirstRate = time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	firstRateDay    = time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
)

const noRateLinePrefix = "valued on 2026-03-09, before 2026-03-10, the first exchange rate in the store, "

// unconvertedRow is a priced security in code with no converted value; id and account tell two rows apart.
func unconvertedRow(id, account, code string) store.Holding {
	row := heldSecurity(id, new("Fund "+id), new(code), new(int64(1_000_000)))
	row.AccountID = account
	return row
}

func Test_HoldingsWarnings_say_one_holding_is_not_converted_before_the_first_rate(t *testing.T) {
	h := report.Holdings{Rows: []store.Holding{unconvertedRow("s-1", "a-1", "USD")}, Currency: money.CAD, AsOf: beforeFirstRate, FirstRate: firstRateDay}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{"1 holding " + noRateLinePrefix + "is not converted to CAD and is totalled in USD"}, got)
}

func Test_HoldingsWarnings_count_each_row_that_needs_a_rate_not_each_security(t *testing.T) {
	rows := []store.Holding{unconvertedRow("s-1", "a-1", "USD"), unconvertedRow("s-1", "a-2", "USD")}
	h := report.Holdings{Rows: rows, Currency: money.CAD, AsOf: beforeFirstRate, FirstRate: firstRateDay}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{"2 holdings " + noRateLinePrefix + "are not converted to CAD and are totalled in USD"}, got)
}

func Test_HoldingsWarnings_swap_the_currencies_in_a_usd_report(t *testing.T) {
	h := report.Holdings{Rows: []store.Holding{unconvertedRow("s-1", "a-1", "CAD")}, Currency: money.USD, AsOf: beforeFirstRate, FirstRate: firstRateDay}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{"1 holding " + noRateLinePrefix + "is not converted to USD and is totalled in CAD"}, got)
}

func Test_HoldingsWarnings_say_the_store_has_no_rates_when_it_holds_none(t *testing.T) {
	rows := []store.Holding{unconvertedRow("s-1", "a-1", "USD"), unconvertedRow("s-2", "a-1", "USD")}
	h := report.Holdings{Rows: rows, Currency: money.CAD, AsOf: beforeFirstRate}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{"the store has no exchange rates, so values are listed in each security's own currency; run quarry sync to fetch them"}, got)
}

func Test_HoldingsWarnings_are_silent_when_no_row_needs_a_rate(t *testing.T) {
	converted := unconvertedRow("s-2", "a-1", "USD")
	converted.ValueCAD = big.NewInt(136)
	cases := []struct {
		name      string
		row       store.Holding
		currency  money.Currency
		firstRate time.Time
	}{
		{name: "a CAD row in a CAD report before the first rate", row: unconvertedRow("s-1", "a-1", "CAD"), currency: money.CAD, firstRate: firstRateDay},
		{name: "a CAD row in a CAD report with no rates at all", row: unconvertedRow("s-1", "a-1", "CAD"), currency: money.CAD},
		{name: "a converted USD row on a day after the first rate", row: converted, currency: money.CAD, firstRate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{name: "a native listing before the first rate", row: unconvertedRow("s-1", "a-1", "USD"), currency: money.Native, firstRate: firstRateDay},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := report.Holdings{Rows: []store.Holding{c.row}, Currency: c.currency, AsOf: beforeFirstRate, FirstRate: c.firstRate}

			assert.Equal(t, []string{}, document.HoldingsWarnings(h))
		})
	}
}

func Test_HoldingsWarnings_put_the_rate_line_after_no_price_and_before_no_currency_and_count_only_cad_and_usd_rows(t *testing.T) {
	const (
		noPrice    = "1 holding has no price on or before 2026-03-09, so it has no value and is left out of the total; enter a price for it in Quicken, then run quarry sync"
		noCurrency = `"Mystery Fund" has no currency in Quicken, so quarry leaves its value out of the total; set its currency in Quicken, then run quarry sync`
		other      = `"Euro Fund" is priced in EUR, which quarry does not convert, so its value is left out of the total`
	)
	priced := new(int64(1_000_000))
	rows := []store.Holding{
		heldSecurity("s-0", new("Bare"), new("CAD"), nil),
		heldSecurity("s-1", new("Mystery Fund"), nil, priced),
		heldSecurity("s-2", new("Euro Fund"), new("EUR"), priced),
		unconvertedRow("s-3", "a-1", "USD"),
	}
	h := report.Holdings{Rows: rows, Currency: money.CAD, AsOf: beforeFirstRate, FirstRate: firstRateDay}

	got := document.HoldingsWarnings(h)

	assert.Equal(t, []string{noPrice, "1 holding " + noRateLinePrefix + "is not converted to CAD and is totalled in USD", noCurrency, other}, got)
}
