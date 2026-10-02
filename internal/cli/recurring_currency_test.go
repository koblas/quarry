package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeRecurringIn runs recurring over fake with a config whose reporting.currency is cfg.
func executeRecurringIn(t *testing.T, cfg money.Currency, fake fakeReportStore, stdout, stderr *bytes.Buffer, args ...string) error {
	t.Helper()
	env := cli.Env{
		Stdout: stdout, Stderr: stderr,
		Now:        func() time.Time { return spendNow },
		LoadConfig: func(string) (config.Config, error) { return config.Config{Currency: cfg}, nil },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), append([]string{"recurring", "--since", "2000"}, args...), env)
}

// cadSeriesWithUSDCells is a monthly CAD series of 10.00 whose charges all carry the USD cell 7.50 (rate 1.3333).
func cadSeriesWithUSDCells() store.Charges {
	c := monthlyCharges("Gym", 1000, 1000, 1000, 1000)
	for i := range c.Rows {
		c.Rows[i].AmountCAD = new(int64(1000))
		c.Rows[i].AmountUSD = new(int64(750))
		c.Rows[i].USDCAD = money.Rate(1_333_333)
	}
	c.FirstRate = time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	return c
}

func Test_recurring_lists_amounts_in_the_currency_the_config_names(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeRecurringIn(t, money.USD, fakeReportStore{charges: cadSeriesWithUSDCells()}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in USD\n\n")
	assert.Contains(t, stdout.String(), "Gym    USD (CAD)  month    7.50")
}

func Test_recurring_flag_beats_the_config_and_native_converts_nothing(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeRecurringIn(t, money.USD, fakeReportStore{charges: cadSeriesWithUSDCells()}, &stdout, &stderr, "--currency", "native")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "in all accounts\n\n")
	assert.NotContains(t, stdout.String(), "amounts in")
	assert.Contains(t, stdout.String(), "Gym    CAD       month   10.00")
}

func Test_recurring_warns_of_a_series_dated_before_the_first_rate_in_the_config_currency(t *testing.T) {
	charges := monthlyCharges("Gym", 1000, 1000, 1000, 1000)
	charges.FirstRate = time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	var stdout, stderr bytes.Buffer

	err := executeRecurringIn(t, money.USD, fakeReportStore{charges: charges}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: 1 series with a charge dated before 2026-04-01, the first exchange rate in the store, "+
		"is listed in CAD, not converted to USD\n", stderr.String())
	assert.Contains(t, stdout.String(), "Gym    CAD       month   10.00")
}

func Test_recurring_prints_the_left_out_account_warning_before_the_unconverted_series_warning(t *testing.T) {
	charges := chargesOn(chequingID)
	for i := range charges.Rows {
		charges.Rows[i].Currency = "USD"
	}
	charges.FirstRate = time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	want := []string{
		leftOutRecurringWarning("Old Card"),
		"1 series with a charge dated before 2026-04-01, the first exchange rate in the store, is listed in USD, not converted to CAD",
	}
	args := []string{"--since", "2000", "--account", "Old Card", "--account", chequingID}

	t.Run("text prints them on stderr in that order", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		err := executeRecurring(t, withCharges(namedAccounts(), charges), &stdout, &stderr, args...)

		require.NoError(t, err)
		assert.Equal(t, warningLines(want), stderr.String())
	})

	t.Run("json lists them in that order in warnings", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		err := executeRecurring(t, withCharges(namedAccounts(), charges), &stdout, &stderr, append([]string{"--json"}, args...)...)

		require.NoError(t, err)
		var doc struct {
			Warnings []string `json:"warnings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
		assert.Equal(t, want, doc.Warnings)
	})
}
