package cli_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const badCurrencyFlag = "--currency must be CAD, USD or native"

// currencyCommands are the commands that take --currency.
var currencyCommands = []string{"spend", "cashflow", "recurring", "anomalies", "accounts"}

// currencyEnv is an Env whose report opens an empty store and whose config leaves the currency at CAD.
func currencyEnv(stdout, stderr *bytes.Buffer) cli.Env {
	return cli.Env{
		Stdout: stdout, Stderr: stderr,
		Now: func() time.Time { return spendNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fakeReportStore{})), nil
		},
		LoadConfig: func(string) (config.Config, error) { return config.Config{Currency: money.CAD}, nil },
	}
}

// refusedEnv is an Env whose report factory fails, so a command that gets that far is not usage-refused.
func refusedEnv(stdout, stderr *bytes.Buffer) cli.Env {
	env := currencyEnv(stdout, stderr)
	env.NewReport = func(context.Context, string) (*report.Server, error) { return nil, errStoreRead }

	return env
}

func Test_currency_flag_is_checked_only_when_given(t *testing.T) {
	for _, command := range currencyCommands {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			absent := cli.Execute(t.Context(), []string{command}, currencyEnv(&stdout, &stderr))
			empty := cli.Execute(t.Context(), []string{command, "--currency="}, currencyEnv(&stdout, &stderr))

			require.NoError(t, absent)
			var usage cli.UsageError
			require.ErrorAs(t, empty, &usage)
			assert.EqualError(t, empty, badCurrencyFlag)
		})
	}
}

func Test_currency_flag_accepts_each_currency_in_any_letter_case(t *testing.T) {
	for _, command := range currencyCommands {
		for _, code := range []string{"usd", "CAD", "Native"} {
			t.Run(command+" "+code, func(t *testing.T) {
				var stdout, stderr bytes.Buffer

				err := cli.Execute(t.Context(), []string{command, "--currency", code}, currencyEnv(&stdout, &stderr))

				assert.NoError(t, err)
			})
		}
	}
}

func Test_currency_flag_refuses_a_value_it_cannot_read_without_echoing_it(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{name: "another currency", code: "EUR"},
		{name: "leading space", code: " CAD"},
		{name: "truncated native", code: "nativ"},
	}

	for _, c := range cases {
		for _, command := range currencyCommands {
			t.Run(command+" "+c.name, func(t *testing.T) {
				var stdout, stderr bytes.Buffer

				err := cli.Execute(t.Context(), []string{command, "--currency", c.code}, refusedEnv(&stdout, &stderr))

				var usage cli.UsageError
				require.ErrorAs(t, err, &usage)
				assert.EqualError(t, err, badCurrencyFlag)
			})
		}
	}
}

func Test_currency_flag_is_refused_before_the_command_checks_its_other_flags(t *testing.T) {
	cases := []struct {
		command string
		other   []string
	}{
		{command: "spend", other: []string{"--by", "bogus"}},
		{command: "cashflow", other: []string{"--by", "bogus"}},
		{command: "recurring", other: []string{"--since", "2024-13"}},
		{command: "anomalies", other: []string{"--since", "2024-13"}},
	}

	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{c.command, "--currency", "EUR"}, c.other...)

			err := cli.Execute(t.Context(), args, refusedEnv(&stdout, &stderr))

			assert.EqualError(t, err, badCurrencyFlag)
		})
	}
}

func Test_currency_flag_is_refused_after_the_check_for_a_positional_argument(t *testing.T) {
	for _, command := range currencyCommands {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := cli.Execute(t.Context(), []string{command, "extra", "--currency", "EUR"}, refusedEnv(&stdout, &stderr))

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			assert.EqualError(t, err, command+" takes no arguments")
		})
	}
}
