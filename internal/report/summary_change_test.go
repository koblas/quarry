package report_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const noRate = "no rate"

var (
	endOfAugust    = day(2026, time.August, 31)
	endOfSeptember = day(2026, time.September, 30)
)

// heldOn is a net worth row of accountType on date; converted is its balance in CAD, nil when no rate converts it.
func heldOn(date time.Time, accountType, currency string, cents int64, converted *big.Int) store.NetWorthRow {
	row := typedRow(accountType, currency, cents, converted)
	row.Date = date
	return row
}

// cadHeld is a CAD row, which converts to itself.
func cadHeld(date time.Time, accountType string, cents int64) store.NetWorthRow {
	return heldOn(date, accountType, "CAD", cents, big.NewInt(cents))
}

// unrated is a USD row no exchange rate converts.
func unrated(date time.Time, accountType string, cents int64) store.NetWorthRow {
	return heldOn(date, accountType, "USD", cents, nil)
}

func changeOf(t *testing.T, currency money.Currency, rows ...store.NetWorthRow) *report.NetWorthChange {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{netWorth: store.NetWorth{Rows: rows}}))
	request := septemberRequest(t)
	request.Currency = currency

	got, err := srv.Summary(t.Context(), request)

	require.NoError(t, err)
	return got.Change
}

// typeChanges renders each type entry as "<currency> <type> <value>", the value "no rate" when nil.
func typeChanges(change *report.NetWorthChange) []string {
	lines := make([]string, 0, len(change.Types))
	for _, entry := range change.Types {
		lines = append(lines, fmt.Sprintf("%s %s %s", entry.Currency, entry.Type, valueText(entry.Value)))
	}
	return lines
}

// totalChanges renders each total as "<currency> <value>", the value "no rate" when nil.
func totalChanges(change *report.NetWorthChange) []string {
	lines := make([]string, 0, len(change.Totals))
	for _, entry := range change.Totals {
		lines = append(lines, fmt.Sprintf("%s %s", entry.Currency, valueText(entry.Value)))
	}
	return lines
}

func valueText(value *big.Int) string {
	if value == nil {
		return noRate
	}
	return value.String()
}

func Test_change_counts_a_type_missing_on_the_start_day_as_zero(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "chequing", 100000),
		cadHeld(endOfSeptember, "chequing", 150000), cadHeld(endOfSeptember, "brokerage", 500000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD brokerage 500000", "CAD chequing 50000"}, typeChanges(change))
	assert.Equal(t, []string{"CAD 550000"}, totalChanges(change))
}

func Test_change_has_one_total_in_the_reporting_currency_when_every_balance_converts(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "chequing", 100000), heldOn(endOfAugust, "savings", "USD", 10000, big.NewInt(13000)),
		cadHeld(endOfSeptember, "chequing", 90000), heldOn(endOfSeptember, "savings", "USD", 10000, big.NewInt(13500)))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing -10000", "CAD savings 500"}, typeChanges(change))
	assert.Equal(t, []string{"CAD -9500"}, totalChanges(change))
}

func Test_change_in_USD_uses_the_USD_balances(t *testing.T) {
	row := func(date time.Time, cad, usd int64) store.NetWorthRow {
		held := heldOn(date, "chequing", "CAD", cad, big.NewInt(cad))
		held.BalanceUSD = big.NewInt(usd)
		return held
	}

	change := changeOf(t, money.USD, row(endOfAugust, 100000, 75000), row(endOfSeptember, 100000, 80000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"USD chequing 5000"}, typeChanges(change))
	assert.Equal(t, []string{"USD 5000"}, totalChanges(change))
}

func Test_change_of_a_type_is_no_rate_when_only_the_start_day_needs_a_rate(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "chequing", 100000), unrated(endOfAugust, "savings", 10000),
		cadHeld(endOfSeptember, "chequing", 150000), heldOn(endOfSeptember, "savings", "USD", 10000, big.NewInt(13000)))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing 50000", "CAD savings " + noRate}, typeChanges(change))
}

func Test_change_of_a_type_is_no_rate_when_only_the_end_day_needs_a_rate(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "chequing", 100000), heldOn(endOfAugust, "savings", "USD", 10000, big.NewInt(13000)),
		cadHeld(endOfSeptember, "chequing", 150000), unrated(endOfSeptember, "savings", 10000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing 50000", "CAD savings " + noRate}, typeChanges(change))
}

func Test_change_of_a_type_is_no_rate_when_one_of_its_rows_converts_and_another_needs_a_rate(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "brokerage", 100000), unrated(endOfAugust, "brokerage", 10000),
		cadHeld(endOfSeptember, "brokerage", 150000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD brokerage " + noRate}, typeChanges(change))
}

func Test_change_total_is_no_rate_when_a_day_holds_a_total_in_another_currency(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "chequing", 100000), unrated(endOfAugust, "savings", 10000),
		cadHeld(endOfSeptember, "chequing", 150000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD " + noRate}, totalChanges(change))
}

func Test_change_total_is_no_rate_when_a_day_has_no_total_in_the_reporting_currency(t *testing.T) {
	change := changeOf(t, money.CAD,
		unrated(endOfAugust, "savings", 10000),
		cadHeld(endOfSeptember, "chequing", 150000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD " + noRate}, totalChanges(change))
}

func Test_change_total_counts_a_start_day_of_unrated_zero_balances_as_zero(t *testing.T) {
	change := changeOf(t, money.CAD,
		unrated(endOfAugust, "savings", 0),
		cadHeld(endOfSeptember, "chequing", 150000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing 150000"}, typeChanges(change))
	assert.Equal(t, []string{"CAD 150000"}, totalChanges(change))
}

func Test_change_counts_an_empty_end_day_as_zero(t *testing.T) {
	change := changeOf(t, money.CAD, cadHeld(endOfAugust, "chequing", 100000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing -100000"}, typeChanges(change))
	assert.Equal(t, []string{"CAD -100000"}, totalChanges(change))
}

func Test_native_change_counts_an_empty_end_day_as_zero(t *testing.T) {
	change := changeOf(t, money.Native, cadHeld(endOfAugust, "chequing", 100000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing -100000"}, typeChanges(change))
	assert.Equal(t, []string{"CAD -100000"}, totalChanges(change))
}

func Test_change_is_absent_when_the_start_day_has_no_balance(t *testing.T) {
	change := changeOf(t, money.CAD, cadHeld(endOfSeptember, "chequing", 150000))

	assert.Nil(t, change)
}

func Test_native_change_counts_a_currency_on_the_end_day_only_as_zero_on_the_start_day(t *testing.T) {
	change := changeOf(t, money.Native,
		cadHeld(endOfAugust, "chequing", 100000),
		cadHeld(endOfSeptember, "chequing", 100000), heldOn(endOfSeptember, "chequing", "USD", 150000, nil))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing 0", "USD chequing 150000"}, typeChanges(change))
	assert.Equal(t, []string{"CAD 0", "USD 150000"}, totalChanges(change))
}

func Test_native_change_lists_a_type_only_for_the_currencies_that_hold_it(t *testing.T) {
	change := changeOf(t, money.Native,
		cadHeld(endOfAugust, "chequing", 100000), cadHeld(endOfAugust, "brokerage", 200000),
		cadHeld(endOfSeptember, "chequing", 110000), heldOn(endOfSeptember, "chequing", "USD", 5000, nil))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD brokerage -200000", "CAD chequing 10000", "USD chequing 5000"}, typeChanges(change))
}

func Test_native_change_orders_currencies_CAD_then_USD_then_alphabetically(t *testing.T) {
	change := changeOf(t, money.Native,
		heldOn(endOfAugust, "chequing", "GBP", 100, nil), heldOn(endOfAugust, "chequing", "USD", 200, nil),
		heldOn(endOfAugust, "chequing", "CAD", 300, nil), heldOn(endOfAugust, "chequing", "EUR", 400, nil))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD -300", "USD -200", "EUR -400", "GBP -100"}, totalChanges(change))
	assert.Equal(t, []string{"CAD chequing -300", "USD chequing -200", "EUR chequing -400", "GBP chequing -100"}, typeChanges(change))
}

func Test_native_change_is_shown_when_only_a_USD_balance_is_on_the_start_day(t *testing.T) {
	change := changeOf(t, money.Native,
		heldOn(endOfAugust, "chequing", "USD", 100000, nil),
		heldOn(endOfSeptember, "chequing", "USD", 120000, nil))

	require.NotNil(t, change)
	assert.Equal(t, []string{"USD chequing 20000"}, typeChanges(change))
	assert.Equal(t, []string{"USD 20000"}, totalChanges(change))
}

func Test_native_change_is_absent_when_the_start_day_has_no_balance(t *testing.T) {
	change := changeOf(t, money.Native, cadHeld(endOfSeptember, "chequing", 150000))

	assert.Nil(t, change)
}

func Test_change_is_absent_for_a_listing_without_dates(t *testing.T) {
	assert.Nil(t, report.NetWorth{}.Change())
}
