package duckstore_test

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	cadChequing   = "acct-cad"
	usdChequing   = "acct-usd"
	usdBrokerage  = "acct-brk"
	zoneChildEnv  = "QUARRY_ACCOUNTS_ZONE_CHILD"
	zoneChildTest = "Test_accounts_zone_probe"
)

// accountRates is the rows behind the account conversion tests: a CAD chequing account holding 100.00, a USD one
// holding 8.00 and a USD brokerage account, all with a balance dated in the past.
func accountRates(t *testing.T, rates ...store.Rate) store.AccountList {
	t.Helper()
	past := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	rows := store.Rows{
		Accounts: []store.Account{
			account(cadChequing, 1, "CAD Chequing", "chequing", "CAD"),
			account(usdChequing, 2, "USD Chequing", "chequing", "USD"),
			account(usdBrokerage, 3, "USD Brokerage", store.AccountTypeBrokerage, "USD"),
		},
		Transactions: []store.Transaction{
			transaction("txn-1", cadChequing, past, 10000),
			{ID: "txn-2", SourceID: 1, AccountID: usdChequing, Date: past, Amount: 800, Currency: "USD", Status: "uncleared"},
		},
		ImportRuns: minimalRows().ImportRuns,
	}
	got, err := newStoreWithRates(t, rows, rates...).Accounts(t.Context())
	require.NoError(t, err)
	return got
}

func cells(list store.AccountList) [][]*int64 {
	out := make([][]*int64, len(list.Accounts))
	for i, a := range list.Accounts {
		out[i] = []*int64{a.Balance, a.BalanceCAD, a.BalanceUSD}
	}
	return out
}

func Test_accounts_converts_at_the_latest_rate_dated_today_or_earlier_ignoring_a_later_one(t *testing.T) {
	t.Parallel()
	past := store.Rate{Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), USDCAD: 1_250_000, Series: store.SeriesCurrent}
	later := store.Rate{Date: time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC), USDCAD: 1_600_000, Series: store.SeriesCurrent}

	got := accountRates(t, past, later)

	assert.Equal(t, [][]*int64{
		{new(int64(10000)), new(int64(10000)), new(int64(8000))},
		{nil, nil, nil},
		{new(int64(800)), new(int64(1000)), new(int64(800))},
	}, cells(got))
}

func Test_accounts_gives_a_cad_account_its_own_cad_cell_and_no_other_cell_without_rates(t *testing.T) {
	t.Parallel()

	got := accountRates(t)

	assert.Equal(t, [][]*int64{
		{new(int64(10000)), new(int64(10000)), nil},
		{nil, nil, nil},
		{new(int64(800)), nil, new(int64(800))},
	}, cells(got))
}

func Test_accounts_gives_the_date_of_the_earliest_rate(t *testing.T) {
	t.Parallel()
	first := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	later := store.Rate{Date: time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC), USDCAD: 1_600_000, Series: store.SeriesCurrent}

	got := accountRates(t, later, store.Rate{Date: first, USDCAD: 1_250_000, Series: store.SeriesCurrent})

	assert.Equal(t, first, got.FirstRate)
}

func Test_accounts_gives_no_first_rate_date_for_a_store_without_rates(t *testing.T) {
	t.Parallel()

	got := accountRates(t)

	assert.True(t, got.FirstRate.IsZero())
}

func Test_accounts_gives_the_first_rate_date_of_a_store_with_rates_after_today_and_no_cross_cell(t *testing.T) {
	t.Parallel()
	later := time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC)

	got := accountRates(t, store.Rate{Date: later, USDCAD: 1_600_000, Series: store.SeriesCurrent})

	assert.Equal(t, later, got.FirstRate)
	assert.Nil(t, got.Accounts[2].BalanceCAD)
}

func Test_accounts_gives_the_first_rate_date_of_a_store_with_no_accounts(t *testing.T) {
	t.Parallel()
	first := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	rows := store.Rows{ImportRuns: minimalRows().ImportRuns}

	got, err := newStoreWithRates(t, rows, store.Rate{Date: first, USDCAD: 1_250_000, Series: store.SeriesCurrent}).Accounts(t.Context())

	require.NoError(t, err)
	assert.Equal(t, first, got.FirstRate)
}

func Test_accounts_converts_a_cad_balance_to_usd_at_the_latest_rate(t *testing.T) {
	t.Parallel()
	rate := store.Rate{Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), USDCAD: money.Rate(1_600_000), Series: store.SeriesCurrent}

	got := accountRates(t, rate)

	assert.Equal(t, new(int64(6250)), got.Accounts[0].BalanceUSD)
}

func Test_accounts_use_the_local_date_in_every_zone(t *testing.T) {
	zones := []string{"Pacific/Kiritimati", "Pacific/Pago_Pago"}

	for _, zone := range zones {
		t.Run(zone, func(t *testing.T) {
			probe := exec.CommandContext( //nolint:gosec // re-executes this test binary
				t.Context(), os.Args[0], "-test.run=^"+zoneChildTest+"$", "-test.v", "-test.count=1")
			probe.Env = append(os.Environ(), "TZ="+zone, zoneChildEnv+"=1")

			out, err := probe.CombinedOutput()

			require.NoError(t, err, string(out))
			assert.Contains(t, string(out), "--- PASS: "+zoneChildTest)
		})
	}
}

// Test_accounts_zone_probe is the child of Test_accounts_use_the_local_date_in_every_zone; alone it skips.
func Test_accounts_zone_probe(t *testing.T) {
	if os.Getenv(zoneChildEnv) == "" {
		t.Skip("runs only inside Test_accounts_use_the_local_date_in_every_zone")
	}
	today := localToday()
	tomorrow := today.AddDate(0, 0, 1)

	got := accountRates(t,
		store.Rate{Date: today, USDCAD: 1_300_000, Series: store.SeriesCurrent},
		store.Rate{Date: tomorrow, USDCAD: 1_400_000, Series: store.SeriesCurrent})

	assert.Equal(t, today, got.AsOf)
	assert.Equal(t, new(int64(1040)), got.Accounts[2].BalanceCAD)
}
