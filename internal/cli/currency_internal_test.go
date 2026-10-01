package cli

import (
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveWith calls resolve on a spend-shaped command bound with args.
func resolveWith(t *testing.T, load ConfigLoader, args ...string) (money.Currency, []string, error) {
	t.Helper()
	var f currencyFlag
	cmd := &cobra.Command{Use: "spend"}
	f.bind(cmd, reportCurrencyHelp)
	require.NoError(t, cmd.ParseFlags(args))

	return f.resolve(cmd, load)
}

func loaderOf(cfg config.Config) ConfigLoader {
	return func(string) (config.Config, error) { return cfg, nil }
}

func Test_resolve_prefers_the_flag_over_the_configs_currency(t *testing.T) {
	got, _, err := resolveWith(t, loaderOf(config.Config{Currency: money.USD}), "--currency", "native")

	require.NoError(t, err)
	assert.Equal(t, money.Native, got)
}

func Test_resolve_uses_the_configs_currency_when_the_flag_is_absent(t *testing.T) {
	for _, want := range []money.Currency{money.CAD, money.USD, money.Native} {
		t.Run(want.String(), func(t *testing.T) {
			got, _, err := resolveWith(t, loaderOf(config.Config{Currency: want}))

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func Test_resolve_reads_a_flag_in_any_letter_case(t *testing.T) {
	got, _, err := resolveWith(t, nil, "--currency", "Usd")

	require.NoError(t, err)
	assert.Equal(t, money.USD, got)
}

func Test_resolve_asks_the_loader_for_the_commands_own_name(t *testing.T) {
	var asked string
	load := func(name string) (config.Config, error) {
		asked = name

		return config.Config{Currency: money.CAD}, nil
	}

	_, _, err := resolveWith(t, load)

	require.NoError(t, err)
	assert.Equal(t, "spend", asked)
}

var errConfigRead = errors.New("cannot read config.toml")

func Test_resolve_wraps_a_config_read_failure_as_a_runtime_error(t *testing.T) {
	_, _, err := resolveWith(t, func(string) (config.Config, error) { return config.Config{}, errConfigRead })

	var runtime *runtimeError
	require.ErrorAs(t, err, &runtime)
	assert.ErrorIs(t, err, errConfigRead)
}

func Test_withConfigWarnings_lists_the_configs_first_and_is_never_nil(t *testing.T) {
	assert.Equal(t, []string{"cfg", "own"}, withConfigWarnings([]string{"cfg"}, []string{"own"}))
	assert.NotNil(t, withConfigWarnings(nil, nil))
}
