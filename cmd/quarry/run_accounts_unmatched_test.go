package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_accounts_and_findings_warn_a_listed_id_that_names_no_account(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	writeConfig(t, home, "[accounts]\nregistered = [\"acct-99\"]\nnon-registered = [\"RBC 12345678\"]\n")
	const warningLead = "quarry: warning: " + configShown + ": accounts."
	const warningTail = ", which is not an account in quarry's store; quarry skips it\n"

	for _, command := range []string{"accounts", "findings"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{command}, &stdout, &stderr)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Contains(t, stderr.String(), warningLead+`registered lists "acct-99"`+warningTail)
			assert.Contains(t, stderr.String(), warningLead+`non-registered lists "RBC ****5678"`+warningTail)
		})
	}
}
