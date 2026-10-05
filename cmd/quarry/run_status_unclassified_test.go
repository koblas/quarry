package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_status_counts_an_unclassified_account_until_the_config_classifies_it(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	var before, after, stderr bytes.Buffer

	require.Equal(t, 0, run(context.Background(), []string{"status"}, &before, &stderr), stderr.String())
	writeConfig(t, home, fmt.Sprintf("[accounts]\nregistered = [\"acct-%d\"]\n", brokeragePK))
	require.Equal(t, 0, run(context.Background(), []string{"status"}, &after, &stderr), stderr.String())

	assert.Contains(t, before.String(), "\nFindings  1 open; run quarry findings to list them\n")
	assert.Contains(t, after.String(), "\nFindings  none open\n")
	assert.Empty(t, stderr.String())
}
