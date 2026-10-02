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

// executeAnomaliesIn runs anomalies over fake with a config whose reporting.currency is cfg.
func executeAnomaliesIn(t *testing.T, cfg money.Currency, fake fakeReportStore, stdout, stderr *bytes.Buffer, args ...string) error {
	t.Helper()
	env := cli.Env{
		Stdout: stdout, Stderr: stderr,
		Now:        func() time.Time { return spendNow },
		LoadConfig: func(string) (config.Config, error) { return config.Config{Currency: cfg}, nil },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), append([]string{"anomalies"}, args...), env)
}

// hardwareCharges is a CAD Hardware payee whose 500.00 charge on 2026-03-02 is 5 times its 100.00 history,
// every charge carrying the USD cell 375.00 / 75.00 (rate 1.3333).
func hardwareCharges(firstRate time.Time) store.Charges {
	rows := payeeHistory("Hardware", time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC), 50000, 10000, 10000, 10000)
	for i := range rows {
		rows[i].USDCAD = money.Rate(1_333_333)
		rows[i].AmountUSD = new(int64(7500))
	}
	rows[len(rows)-1].AmountUSD = new(int64(37500))
	return store.Charges{Rows: onAccount(rows, chequingID), FirstRate: firstRate}
}

func Test_anomalies_lists_amounts_in_the_currency_the_config_names(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rated := hardwareCharges(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC))

	err := executeAnomaliesIn(t, money.USD, fakeReportStore{charges: rated}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in USD\n\n")
	assert.Contains(t, stdout.String(), "375.00  75.00")
}

func Test_anomalies_flag_beats_the_config_and_native_converts_nothing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rated := hardwareCharges(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC))

	err := executeAnomaliesIn(t, money.USD, fakeReportStore{charges: rated}, &stdout, &stderr, "--currency", "native")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "in all accounts\n\n")
	assert.NotContains(t, stdout.String(), "amounts in")
	assert.Contains(t, stdout.String(), "500.00  100.00")
}

func Test_anomalies_warns_of_a_charge_dated_before_the_first_rate_in_the_config_currency(t *testing.T) {
	var stdout, stderr bytes.Buffer
	early := hardwareCharges(time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC))
	for i := range early.Rows {
		early.Rows[i].AmountUSD, early.Rows[i].USDCAD = nil, 0
	}

	err := executeAnomaliesIn(t, money.USD, fakeReportStore{charges: early}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: 1 charge dated before 2026-04-01, the first exchange rate in the store, "+
		"is listed in CAD, not converted to USD\n", stderr.String())
	assert.Contains(t, stdout.String(), "CAD 500.00  CAD 100.00")
}

func Test_anomalies_prints_the_left_out_account_warning_before_the_unconverted_charge_warning(t *testing.T) {
	early := hardwareCharges(time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC))
	for i := range early.Rows {
		early.Rows[i].AmountUSD, early.Rows[i].USDCAD = nil, 0
	}
	want := []string{
		leftOutAnomaliesWarning("Old Card"),
		"1 charge dated before 2026-04-01, the first exchange rate in the store, is listed in CAD, not converted to USD",
	}
	args := []string{"--account", "Old Card", "--account", chequingID}

	t.Run("text prints them on stderr in that order", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		err := executeAnomaliesIn(t, money.USD, withCharges(namedAccounts(), early), &stdout, &stderr, args...)

		require.NoError(t, err)
		assert.Equal(t, warningLines(want), stderr.String())
	})

	t.Run("json lists them in that order in warnings", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		err := executeAnomaliesIn(t, money.USD, withCharges(namedAccounts(), early), &stdout, &stderr, append([]string{"--json"}, args...)...)

		require.NoError(t, err)
		var doc struct {
			Warnings []string `json:"warnings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
		assert.Equal(t, want, doc.Warnings)
	})
}

func Test_anomalies_says_the_store_has_no_rates_when_a_charge_needs_one(t *testing.T) {
	var stdout, stderr bytes.Buffer
	unrated := hardwareCharges(time.Time{})
	for i := range unrated.Rows {
		unrated.Rows[i].AmountUSD, unrated.Rows[i].USDCAD = nil, 0
	}

	err := executeAnomaliesIn(t, money.USD, fakeReportStore{charges: unrated}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: the store has no exchange rates, so amounts are listed in each account's own currency; "+
		"run quarry sync to fetch them\n", stderr.String())
}
