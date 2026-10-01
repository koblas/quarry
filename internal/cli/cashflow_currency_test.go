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
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_cashflow_reads_in_the_currency_the_resolver_picks(t *testing.T) {
	cases := []struct {
		name   string
		config money.Currency
		args   []string
		want   money.Currency
	}{
		{name: "the config's currency without the flag", config: money.USD, want: money.USD},
		{name: "the flag", config: money.CAD, args: []string{"--currency", "usd"}, want: money.USD},
		{name: "the flag beats the config", config: money.USD, args: []string{"--currency", "CAD"}, want: money.CAD},
		{name: "native by flag", config: money.CAD, args: []string{"--currency", "native"}, want: money.Native},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got store.CashFlowParams
			var stdout, stderr bytes.Buffer
			env := cli.Env{
				Stdout: &stdout, Stderr: &stderr,
				Now:        func() time.Time { return spendNow },
				LoadConfig: func(string) (config.Config, error) { return config.Config{Currency: c.config}, nil },
				NewReport: func(context.Context, string) (*report.Server, error) {
					return report.NewServer(report.WithStore(fakeReportStore{gotCashFlow: &got})), nil
				},
			}

			err := cli.Execute(t.Context(), append([]string{"cashflow"}, c.args...), env)

			require.NoError(t, err)
			assert.Equal(t, c.want, got.Currency)
		})
	}
}
