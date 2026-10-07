package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_networth_refuses_an_as_of_it_cannot_use_before_opening_the_store(t *testing.T) {
	const conflict = "--as-of cannot be combined with --since or --until; pass --as-of for one day, or --since and --until for month ends"
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "not a date", args: []string{"--as-of", "2024-13"}, want: `--as-of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{
			name: "after today", args: []string{"--as-of", "2999-01"},
			want: "--as-of 2999-01 is after today; net worth is valued up to today only, so pass an earlier --as-of",
		},
		{name: "beside a since", args: []string{"--as-of", "2025-01", "--since", "2024"}, want: conflict},
		{name: "beside an empty until", args: []string{"--as-of", "2025-01", "--until", ""}, want: conflict},
		{
			name: "a since after today", args: []string{"--since", "2999"},
			want: "--since 2999 is after today; net worth is valued up to today only, so pass an earlier --since",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := cli.Execute(t.Context(), append([]string{"networth"}, c.args...), refusedEnv(&stdout, &stderr))

			require.EqualError(t, err, c.want)
			assert.Empty(t, stdout.String())
		})
	}
}

func executeNetWorth(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := reportEnv(fake, stdout, stderr, atSpendNow)
	return cli.Execute(t.Context(), append([]string{"networth", "--as-of", "2026-03-12"}, args...), env)
}

// leftOutNetWorth is a net worth read with one chequing row, whose brokerage holds one unpriced security on 2026-03-12.
func leftOutNetWorth() fakeReportStore {
	day := time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC)
	return fakeReportStore{netWorth: store.NetWorth{Rows: []store.NetWorthRow{{
		Date: day, Type: "chequing", Currency: "CAD", Accounts: 1, Balance: big.NewInt(100), BalanceCAD: big.NewInt(100),
	}}, Unvalued: []store.UnvaluedHolding{{
		Date: day, AccountID: "acct-1", Account: "Brokerage", SecurityID: "sec-1", Security: "Acme", Currency: new("CAD"),
	}}}}
}

const leftOutNetWorthWarning = `"Brokerage" holds 1 security with no price on or before 2026-03-12, ` +
	`so its balance leaves it out; enter a price in Quicken, then run quarry sync`

func Test_networth_writes_the_holdings_warnings_to_stderr_after_the_result(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeNetWorth(t, leftOutNetWorth(), &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, stderrOf([]string{leftOutNetWorthWarning}), stderr.String())
}

func Test_networth_writes_no_holdings_warning_when_stdout_fails(t *testing.T) {
	var stderr bytes.Buffer

	err := executeNetWorth(t, leftOutNetWorth(), failingWriter{err: errNoSpace}, &stderr)

	require.ErrorIs(t, err, errNoSpace)
	assert.Empty(t, stderr.String())
}

const emptyNetWorthWarning = "no account has a balance on 2026-03-12; no account in Quicken's reports has transactions or holdings"

func Test_networth_prints_the_config_warning_before_the_empty_result_warning(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"networth", "--as-of", "2026-03-12"}, loaderEnv(&stdout, &stderr, warningConfig))

	require.NoError(t, err)
	assert.Equal(t, stderrOf([]string{unknownKeyShown, emptyNetWorthWarning}), stderr.String())
}

func Test_networth_json_lists_the_config_warning_before_the_empty_result_warning(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"networth", "--as-of", "2026-03-12", "--json"}, loaderEnv(&stdout, &stderr, warningConfig))

	require.NoError(t, err)
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{unknownKeyAbsolute, emptyNetWorthWarning}, doc.Warnings)
}
