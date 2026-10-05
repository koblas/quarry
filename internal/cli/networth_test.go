package cli_test

import (
	"bytes"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_networth_returns_a_failed_report_open(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"networth"}, refusedEnv(&stdout, &stderr))

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

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
