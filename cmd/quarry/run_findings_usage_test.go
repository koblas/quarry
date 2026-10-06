// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_run_findings_rejects_usage_it_cannot_use(t *testing.T) {
	const types = "duplicate, one-sided-transfer, unlinked-transfer, uncategorized, mixed-categories, payee-variants, similar-categories, unused-category, unclassified-account or shares-without-cost"
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name: "a positional argument", args: []string{"findings", "extra"},
			wantStderr: "quarry: findings takes no arguments; to ignore a finding add its id to findings.ignore in " +
				"~/Library/Application Support/quarry/config.toml; Run 'quarry findings --help' for usage.\n",
		},
		{name: "a status that is not one of the four", args: []string{"findings", "--status", "maybe"}, wantStderr: "quarry: --status must be open, ignored, fixed or all\n"},
		{name: "a type that is not a finding type", args: []string{"findings", "--type", "bogus"}, wantStderr: "quarry: --type must be " + types + "\n"},
		{
			name: "--csv with --json", args: []string{"findings", "--csv", "--json"},
			wantStderr: "quarry: --csv and --json cannot be used together; choose one output format\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}
