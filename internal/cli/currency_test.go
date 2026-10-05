package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
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
var currencyCommands = []string{"spend", "cashflow", "recurring", "anomalies", "accounts", "holdings", "networth"}

// flagSkipsConfigCommands are the currencyCommands that read no config when --currency is given;
// accounts always reads it, for the account classification.
var flagSkipsConfigCommands = []string{"spend", "cashflow", "recurring", "anomalies", "holdings", "networth"}

// cadConfig is a ConfigLoader for a config file that leaves reporting.currency at its CAD default.
func cadConfig(string) (config.Config, error) { return config.Config{Currency: money.CAD}, nil }

// currencyEnv is an Env whose report opens an empty store and whose config leaves the currency at CAD.
func currencyEnv(stdout, stderr *bytes.Buffer) cli.Env {
	return cli.Env{
		Stdout: stdout, Stderr: stderr,
		Now: func() time.Time { return spendNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fakeReportStore{})), nil
		},
		LoadConfig: cadConfig,
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

const (
	unknownKeyShown    = "~/Library/Application Support/quarry/config.toml: unknown key \"colour\"; quarry ignores it"
	unknownKeyAbsolute = "/Users/me/Library/Application Support/quarry/config.toml: unknown key \"colour\"; quarry ignores it"
)

var errConfigRead = errors.New("cannot read config.toml: permission denied")

// loaderEnv is currencyEnv whose config loader is load.
func loaderEnv(stdout, stderr *bytes.Buffer, load cli.ConfigLoader) cli.Env {
	env := currencyEnv(stdout, stderr)
	env.LoadConfig = load

	return env
}

// warningConfig is a ConfigLoader for a config file with one unknown key.
func warningConfig(string) (config.Config, error) {
	return config.Config{
		Currency:         money.CAD,
		Warnings:         []string{unknownKeyShown},
		WarningsAbsolute: []string{unknownKeyAbsolute},
	}, nil
}

func Test_read_commands_read_the_config_once_and_only_without_the_currency_flag(t *testing.T) {
	for _, command := range currencyCommands {
		t.Run(command+" without the flag", func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			var asked []string
			env := loaderEnv(&stdout, &stderr, func(name string) (config.Config, error) {
				asked = append(asked, name)

				return cadConfig(name)
			})

			err := cli.Execute(t.Context(), []string{command}, env)

			require.NoError(t, err)
			assert.Equal(t, []string{command}, asked)
		})
	}
	for _, command := range flagSkipsConfigCommands {
		t.Run(command+" with the flag", func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			calls := 0
			env := loaderEnv(&stdout, &stderr, func(string) (config.Config, error) {
				calls++

				return config.Config{}, errConfigRead
			})

			err := cli.Execute(t.Context(), []string{command, "--currency", "usd"}, env)

			require.NoError(t, err)
			assert.Zero(t, calls)
		})
	}
}

// configAlwaysReadCommands are the currencyCommands that read the config even when --currency is given.
var configAlwaysReadCommands = []string{"accounts", "acb"}

func Test_config_always_read_commands_read_the_config_once_even_with_the_currency_flag(t *testing.T) {
	for _, command := range configAlwaysReadCommands {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			var asked []string
			env := loaderEnv(&stdout, &stderr, func(name string) (config.Config, error) {
				asked = append(asked, name)

				return cadConfig(name)
			})

			err := cli.Execute(t.Context(), []string{command, "--currency", "native"}, env)

			require.NoError(t, err)
			assert.Equal(t, []string{command}, asked)
		})
	}
}

func Test_config_always_read_commands_refuse_an_unreadable_config_as_a_runtime_error_even_with_the_currency_flag(t *testing.T) {
	for _, command := range configAlwaysReadCommands {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			env := loaderEnv(&stdout, &stderr, func(string) (config.Config, error) { return config.Config{}, errConfigRead })

			err := cli.Execute(t.Context(), []string{command, "--currency", "native"}, env)

			require.ErrorIs(t, err, errConfigRead)
			assert.NotErrorAs(t, err, new(cli.UsageError))
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_config_always_read_commands_print_the_configs_warnings_once_even_with_the_currency_flag(t *testing.T) {
	for _, command := range configAlwaysReadCommands {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := cli.Execute(t.Context(), []string{command, "--currency", "native"}, loaderEnv(&stdout, &stderr, warningConfig))

			require.NoError(t, err)
			assert.Equal(t, 1, strings.Count(stderr.String(), "quarry: warning: "+unknownKeyShown+"\n"))
		})
	}
}

func Test_config_always_read_commands_name_the_configs_warnings_absolutely_in_json_even_with_the_currency_flag(t *testing.T) {
	for _, command := range configAlwaysReadCommands {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := cli.Execute(t.Context(), []string{command, "--currency", "native", "--json"}, loaderEnv(&stdout, &stderr, warningConfig))

			require.NoError(t, err)
			var doc struct {
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
			require.NotEmpty(t, doc.Warnings)
			assert.Equal(t, unknownKeyAbsolute, doc.Warnings[0])
		})
	}
}

func Test_read_commands_refuse_an_unreadable_config_as_a_runtime_error(t *testing.T) {
	for _, command := range currencyCommands {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			env := loaderEnv(&stdout, &stderr, func(string) (config.Config, error) { return config.Config{}, errConfigRead })

			err := cli.Execute(t.Context(), []string{command}, env)

			require.ErrorIs(t, err, errConfigRead)
			assert.NotErrorAs(t, err, new(cli.UsageError))
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_read_commands_print_the_configs_warnings_in_home_form_on_stderr_before_the_result(t *testing.T) {
	for _, command := range currencyCommands {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := cli.Execute(t.Context(), []string{command}, loaderEnv(&stdout, &stderr, warningConfig))

			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(stderr.String(), "quarry: warning: "+unknownKeyShown+"\n"), stderr.String())
		})
	}
}

func Test_read_commands_name_the_configs_warnings_absolutely_first_in_json_and_in_home_form_on_stderr(t *testing.T) {
	for _, command := range currencyCommands {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := cli.Execute(t.Context(), []string{command, "--json"}, loaderEnv(&stdout, &stderr, warningConfig))

			require.NoError(t, err)
			var doc struct {
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
			require.NotEmpty(t, doc.Warnings)
			assert.Equal(t, unknownKeyAbsolute, doc.Warnings[0])
			assert.True(t, strings.HasPrefix(stderr.String(), "quarry: warning: "+unknownKeyShown+"\n"), stderr.String())
			assert.NotContains(t, stderr.String(), unknownKeyAbsolute)
		})
	}
}

func Test_spend_prints_the_configs_warnings_before_its_own_and_lists_them_once_on_stderr(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"spend"}, loaderEnv(&stdout, &stderr, warningConfig))

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "quarry: warning: "+unknownKeyShown, lines[0])
	assert.Contains(t, lines[1], "no spending")
}

func Test_read_commands_refuse_their_own_bad_flags_before_reading_the_config(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "spend --by", args: []string{"spend", "--by", "bogus"}},
		{name: "spend --since", args: []string{"spend", "--since", "2024-13"}},
		{name: "cashflow --by", args: []string{"cashflow", "--by", "bogus"}},
		{name: "cashflow --since", args: []string{"cashflow", "--since", "2024-13"}},
		{name: "recurring --since", args: []string{"recurring", "--since", "2024-13"}},
		{name: "anomalies --since", args: []string{"anomalies", "--since", "2024-13"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			calls := 0
			env := loaderEnv(&stdout, &stderr, func(string) (config.Config, error) {
				calls++

				return config.Config{}, errConfigRead
			})

			err := cli.Execute(t.Context(), c.args, env)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			assert.Zero(t, calls)
		})
	}
}

func Test_accounts_lists_the_configs_warnings_before_the_all_closed_note(t *testing.T) {
	const note = "all 2 accounts are closed; pass --all to list them"
	newEnv := func(stdout, stderr *bytes.Buffer) cli.Env {
		env := loaderEnv(stdout, stderr, warningConfig)
		env.NewReport = func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fakeReportStore{accounts: closedAccounts(2)})), nil
		}

		return env
	}

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		err := cli.Execute(t.Context(), []string{"accounts", "--json"}, newEnv(&stdout, &stderr))

		require.NoError(t, err)
		var doc struct {
			Warnings []string `json:"warnings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
		assert.Equal(t, []string{unknownKeyAbsolute, note}, doc.Warnings)
	})
	t.Run("text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		err := cli.Execute(t.Context(), []string{"accounts"}, newEnv(&stdout, &stderr))

		require.NoError(t, err)
		assert.Equal(t, "quarry: warning: "+unknownKeyShown+"\nquarry: warning: "+note+"\n", stderr.String())
	})
}
