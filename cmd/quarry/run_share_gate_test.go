package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shareMismatchRow renders one "!" row the way the failed-validation block does.
func shareMismatchRow(accountWidth, securityWidth, quarryWidth, quickenWidth, diffWidth int, account, security, quarry, quicken, diff string) string {
	return fmt.Sprintf("  ! %-*s  %-*s  quarry %*s  Quicken %*s  difference %*s",
		accountWidth, account, securityWidth, security, quarryWidth, quarry, quickenWidth, quicken, diffWidth, diff)
}

// objectKeysInOrder returns the top-level keys of the JSON object in raw, in document order.
func objectKeysInOrder(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	_, err := dec.Token()
	require.NoError(t, err)
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		require.NoError(t, err)
		keys = append(keys, key.(string))
		var skip json.RawMessage
		require.NoError(t, dec.Decode(&skip))
	}
	return keys
}

// shareGateBundle holds one cash account that matches its statement, a
// matching holding (Acme, 10 shares), a holding with transactions and no lots
// in an open account (Bare Fund, 5 shares), and a closed RRSP holding whose
// derived 120.5 shares differ from its 110.5 lot units.
func shareGateBundle(t *testing.T, home string) (bundle v9fixture.Bundle, brokeragePK, rrspPK, barePK, ishares int64) {
	t.Helper()
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK = b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	rrspPK = b.Account(v9fixture.AccountRow{Name: "RRSP", Type: "RETIREMENTIRA", Currency: "CAD", Closed: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	barePK = b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Currency: "CAD"})
	ishares = b.Security(v9fixture.SecurityRow{Name: "iShares Core Equity ETF", Ticker: "XEQT", Currency: "CAD"})
	acmePosition := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	barePosition := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: barePK})
	isharesPosition := b.Position(v9fixture.PositionRow{Account: rrspPK, Security: ishares})

	cashPK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: cashPK, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &day, EndingBalance: "100.00"})
	buy := func(account, position int64, units, amount string) {
		pk := b.InvestmentTransaction(v9fixture.TransactionRow{
			Account: account, Position: position, Type: new(int64(3)), Units: units, Amount: amount, PostedDate: &day,
		})
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: amount})
	}
	buy(brokeragePK, acmePosition, "10", "-1000.00")
	buy(brokeragePK, barePosition, "5", "-50.00")
	buy(rrspPK, isharesPosition, "120.5", "-1205.00")
	b.Lot(v9fixture.LotRow{Position: acmePosition, LatestUnits: "10"})
	b.Lot(v9fixture.LotRow{Position: isharesPosition, LatestUnits: "110.5"})

	return b.WriteBundle(t, filepath.Join(home, "Documents")), brokeragePK, rrspPK, barePK, ishares
}

func Test_run_sync_fails_when_holdings_share_counts_differ_from_quicken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	storePath := storePathUnder(home)
	require.NoError(t, os.MkdirAll(filepath.Dir(storePath), 0o700))
	sentinel := []byte("previous store bytes, untouched by a failing sync")
	require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))
	bundle, brokeragePK, rrspPK, barePK, isharesPK := shareGateBundle(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	snapshotPath := onlyFileWithSuffix(t, filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"), ".sqlite")
	assert.Contains(t, stdout.String(), fmt.Sprintf("%-10s%s\n", "Store", "NOT REBUILT ("+abbreviated(t, storePath, home)+" unchanged)"))
	accountWidth := maxLen("Brokerage (CAD)", "RRSP (CAD, closed)")
	securityWidth := maxLen("Bare Fund", "iShares Core Equity ETF (XEQT)")
	wantBlock := fmt.Sprintf("%-10s%s\n%s\n%s\n",
		"Shares", "DIFFER for 2 of 3 holdings",
		shareMismatchRow(accountWidth, securityWidth, 5, 5, 2, "Brokerage (CAD)", "Bare Fund", "5", "0", "5"),
		shareMismatchRow(accountWidth, securityWidth, 5, 5, 2, "RRSP (CAD, closed)", "iShares Core Equity ETF (XEQT)", "120.5", "110.5", "10"))
	assert.Contains(t, stdout.String(), wantBlock)
	assert.Equal(t,
		"quarry: validation failed: 2 of 3 holdings do not match Quicken's share counts; "+
			abbreviated(t, storePath, home)+" was not changed; each difference is listed on stdout; "+
			"quarry read the holding's transactions differently from Quicken, so run quarry sync --from "+
			snapshotID(snapshotPath)+" after updating quarry\n",
		stderr.String())
	got, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.Equal(t, sentinel, got)

	var jsonOut, jsonErr bytes.Buffer

	exitCode = run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &jsonOut, &jsonErr)

	assert.Equal(t, 1, exitCode)
	var doc struct {
		Store struct {
			Built  bool `json:"built"`
			Shares struct {
				Checked    int               `json:"checked"`
				Mismatched []json.RawMessage `json:"mismatched"`
			} `json:"shares"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc))
	assert.False(t, doc.Store.Built)
	assert.Equal(t, 3, doc.Store.Shares.Checked)
	require.Len(t, doc.Store.Shares.Mismatched, 2)
	wantKeys := []string{"account_id", "account", "currency", "closed", "active", "security_id", "security", "ticker", "quarry", "quicken", "difference"}
	assert.Equal(t, wantKeys, objectKeysInOrder(t, doc.Store.Shares.Mismatched[0]))
	assert.Equal(t, wantKeys, objectKeysInOrder(t, doc.Store.Shares.Mismatched[1]))
	assert.JSONEq(t, fmt.Sprintf(`{"account_id":"acct-%d","account":"Brokerage","currency":"CAD","closed":false,"active":true,`+
		`"security_id":"sec-%d","security":"Bare Fund","ticker":null,"quarry":"5.000000","quicken":"0.000000","difference":"5.000000"}`,
		brokeragePK, barePK), string(doc.Store.Shares.Mismatched[0]))
	assert.JSONEq(t, fmt.Sprintf(`{"account_id":"acct-%d","account":"RRSP","currency":"CAD","closed":true,"active":false,`+
		`"security_id":"sec-%d","security":"iShares Core Equity ETF","ticker":"XEQT","quarry":"120.500000","quicken":"110.500000","difference":"10.000000"}`,
		rrspPK, isharesPK), string(doc.Store.Shares.Mismatched[1]))
}

func Test_run_sync_joins_a_share_failure_to_a_balance_failure_in_one_line(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	cashPK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: cashPK, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &day, EndingBalance: "100.01"})
	buyPK := b.InvestmentTransaction(v9fixture.TransactionRow{
		Account: brokeragePK, Position: positionPK, Type: new(int64(3)), Units: "10", Amount: "-1000.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: buyPK, Amount: "-1000.00"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "9"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	snapshotPath := onlyFileWithSuffix(t, filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"), ".sqlite")
	assert.Equal(t,
		"quarry: validation failed: 1 of 1 account does not match Quicken's last reconciled balance and "+
			"1 of 1 holding does not match Quicken's share count; "+
			abbreviated(t, storePathUnder(home), home)+" was not changed; each difference is listed on stdout; "+
			"fix them in Quicken and run quarry sync, or run quarry sync --from "+
			snapshotID(snapshotPath)+" after updating quarry\n",
		stderr.String())
}
