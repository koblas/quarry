// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ignoreFixtureIDs are the ids of the two findings syncIgnoreFixture raises.
type ignoreFixtureIDs struct {
	duplicate     string
	uncategorized string
}

// syncIgnoreFixture syncs a file raising one duplicate pair and one uncategorized payee under home.
func syncIgnoreFixture(t *testing.T, home string) ignoreFixtureIDs {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	hydroNetworksPK := b.Payee(v9fixture.PayeeRow{Name: "HYDRO ONE NETWORKS"})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	first := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17")
	second := categorizedPayeeTxn(b, chequingPK, hydroNetworksPK, billsPK, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-142.17")
	for i, amount := range []string{"-10.00", "-20.00"} {
		day := time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: amazonPK})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return ignoreFixtureIDs{
		duplicate:     fmt.Sprintf("duplicate:txn-%d+txn-%d", first, second),
		uncategorized: fmt.Sprintf("uncategorized:payee-%d", amazonPK),
	}
}

func Test_run_findings_leaves_an_ignored_id_off_the_list_and_counts_it_in_the_footer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ids := syncIgnoreFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n", ids.duplicate))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Uncategorized (1 payee, 2 splits): give each payee's splits a category in Quicken
  %s  Amazon  2 splits  2026-03-01 to 2026-03-02

1 open finding; 1 ignored not shown (--status all)
`, ids.uncategorized), stdout.String())
}

func Test_run_findings_warns_about_an_ignored_id_that_is_not_a_finding(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncIgnoreFixture(t, home)
	writeConfig(t, home, "[findings]\nignore = [\"uncategorized:payee-999\"]\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, "quarry: warning: "+configShown+
		": findings.ignore lists \"uncategorized:payee-999\", which is not a finding in quarry's store; quarry skips it\n",
		stderr.String())
}

func Test_run_findings_json_lists_an_unmatched_ignore_id_after_the_config_warning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncIgnoreFixture(t, home)
	writeConfig(t, home, "colour = \"red\"\n[findings]\nignore = [\"uncategorized:payee-999\"]\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	unknownKey := configShown + ": unknown key colour; quarry ignores it"
	unmatched := configShown + ": findings.ignore lists \"uncategorized:payee-999\", which is not a finding in quarry's store; quarry skips it"
	assert.Equal(t, "quarry: warning: "+unknownKey+"\nquarry: warning: "+unmatched+"\n", stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{unknownKey, unmatched}, doc.Warnings)
}

func Test_run_findings_shows_the_hint_when_findings_ignore_is_an_empty_list(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncIgnoreFixture(t, home)
	writeConfig(t, home, "[findings]\nignore = []\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "2 open findings\nIgnore a finding by adding its id to findings.ignore in "+configShown)
}

func Test_run_findings_hides_the_hint_when_findings_ignore_lists_only_unmatched_ids(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncIgnoreFixture(t, home)
	writeConfig(t, home, "[findings]\nignore = [\"uncategorized:payee-999\"]\n")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.True(t, strings.HasSuffix(stdout.String(), "\n2 open findings\n"), stdout.String())
}

func Test_run_findings_counts_an_ignored_finding_that_is_fixed_as_fixed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ids := syncIgnoreFixture(t, home)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	b.Payee(v9fixture.PayeeRow{Name: "HYDRO ONE NETWORKS"})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17")
	for i, amount := range []string{"-10.00", "-20.00"} {
		day := time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: amazonPK})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
	}
	exitCode, _, syncErr := syncNewBundle(t, home, "DocumentsB", b)
	require.Equal(t, 0, exitCode, syncErr)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n", ids.duplicate))
	var stdout, stderr bytes.Buffer

	exitCode = run(context.Background(), []string{"findings"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.True(t, strings.HasSuffix(stdout.String(), "\n1 open finding; 1 fixed not shown (--status all)\n"), stdout.String())
}
