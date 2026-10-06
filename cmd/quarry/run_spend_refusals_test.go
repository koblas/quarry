// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// HOME holds no store: exit 2 (not the missing-store 1) shows each check runs first.
func Test_run_spend_rejects_a_period_it_cannot_use(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "a since that is not a date",
			args:       []string{"spend", "--since", "2024-13"},
			wantStderr: "quarry: --since \"2024-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name:       "an until that is not a date",
			args:       []string{"spend", "--until", "yesterday"},
			wantStderr: "quarry: --until \"yesterday\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name:       "a since after until",
			args:       []string{"spend", "--since", "2025", "--until", "2024"},
			wantStderr: "quarry: --since 2025 is after --until 2024\n",
		},
		{
			name:       "a since after today",
			args:       []string{"spend", "--since", "2099"},
			wantStderr: "quarry: --since 2099 is after today; pass --until to include future-dated transactions\n",
		},
		{
			name:       "a grouping that does not exist",
			args:       []string{"spend", "--by", "vendor"},
			wantStderr: "quarry: --by must be category, payee, tag or month\n",
		},
		{
			name:       "a positional argument",
			args:       []string{"spend", "extra"},
			wantStderr: "quarry: spend takes no arguments\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			env := spendEnv(&stdout, &stderr)

			exitCode := runWith(context.Background(), c.args, env)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_report_commands_refuse_when_home_is_unset(t *testing.T) {
	cases := []struct {
		command    string
		wantStderr string
	}{
		{command: "spend", wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry spend again\n"},
		{
			command:    "cashflow",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry cashflow again\n",
		},
		{
			command:    "recurring",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry recurring again\n",
		},
		{
			command:    "anomalies",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry anomalies again\n",
		},
		{
			command:    "search",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry search again\n",
		},
		{
			command:    "holdings",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry holdings again\n",
		},
		{
			command:    "networth",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry networth again\n",
		},
		{
			command:    "acb",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry acb again\n",
		},
		{
			command:    "summary",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry summary again\n",
		},
	}

	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			t.Setenv("HOME", "")
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{c.command}, &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}
