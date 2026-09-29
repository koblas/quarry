// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accountRowJSON struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Currency    string  `json:"currency"`
	Institution *string `json:"institution"`
	Closed      bool    `json:"closed"`
	Active      bool    `json:"active"`
	Balance     *string `json:"balance"`
}

type accountsJSON struct {
	AsOf     string           `json:"as_of"`
	Accounts []accountRowJSON `json:"accounts"`
	Warnings []string         `json:"warnings"`
}

func Test_run_accounts_json_returns_accounts_as_a_document(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	bank := b.Institution(v9fixture.InstitutionRow{Name: "First Bank"})
	chequing := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Institution: bank, Active: true})
	rrsp := b.Account(v9fixture.AccountRow{Name: "RRSP", Type: "RETIREMENTIRA", Currency: "CAD", Institution: bank, Active: true})
	savings := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	past := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	addTransaction(b, chequing, "12400.00", past)
	addTransaction(b, chequing, "-54.33", past)
	addTransaction(b, rrsp, "1000.00", past)
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	syncBundle(t, bundle)
	var stdout, stderr bytes.Buffer
	before := time.Now()

	exitCode := run(context.Background(), []string{"accounts", "--json"}, &stdout, &stderr)

	after := time.Now()
	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	want := func(asOf time.Time) string {
		return fmt.Sprintf(`{
  "as_of": "%s",
  "accounts": [
    {
      "id": "acct-%d",
      "name": "Chequing",
      "type": "chequing",
      "currency": "CAD",
      "institution": "First Bank",
      "closed": false,
      "active": true,
      "balance": "12345.67"
    },
    {
      "id": "acct-%d",
      "name": "RRSP",
      "type": "retirement",
      "currency": "CAD",
      "institution": "First Bank",
      "closed": false,
      "active": true,
      "balance": null
    },
    {
      "id": "acct-%d",
      "name": "Savings",
      "type": "savings",
      "currency": "CAD",
      "institution": null,
      "closed": false,
      "active": true,
      "balance": "0.00"
    }
  ],
  "warnings": []
}
`, asOf.Format("2006-01-02"), chequing, rrsp, savings)
	}
	assert.Contains(t, []string{want(before), want(after)}, stdout.String())
}

func Test_run_accounts_json_reports_the_all_closed_note_in_both_streams(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncClosedAccountsFixture(t, home, 3)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var got accountsJSON
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Empty(t, got.Accounts)
	assert.Contains(t, stdout.String(), "  \"accounts\": [],\n")
	assert.Equal(t, []string{"all 3 accounts are closed; pass --all to list them"}, got.Warnings)
	assert.Equal(t, "quarry: all 3 accounts are closed; pass --all to list them\n", stderr.String())
}
