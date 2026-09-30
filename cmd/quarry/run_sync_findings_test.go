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

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncFindingsBundle writes b under home, syncs it and returns the store opened read-only with sync's stdout.
func syncFindingsBundle(t *testing.T, home string, b *v9fixture.Builder) (*duckdb.DB, string) {
	t.Helper()
	stdout := syncFindingsBundleIn(t, home, "Documents", b)
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, stdout
}

// syncFindingsBundleIn syncs b written under home/dir and returns sync's last stdout line, leaving no store connection open.
func syncFindingsBundleIn(t *testing.T, home, dir string, b *v9fixture.Builder) string {
	t.Helper()
	bundle := b.WriteBundle(t, filepath.Join(home, dir))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	return lines[len(lines)-1]
}

// uncategorizedPayeeBundle mints the same payee key on every call, so the finding id is the same in every sync.
func uncategorizedPayeeBundle(categorized bool, amount string) (*v9fixture.Builder, int64) {
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: amazonPK})
	entry := v9fixture.EntryRow{Parent: txn, Amount: amount}
	if categorized {
		entry.CategoryTag = foodPK
	}
	b.Entry(entry)
	return b, amazonPK
}

func Test_run_sync_records_findings_and_prints_the_findings_line(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, amount := range []string{"-10.00", "-20.00"} {
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: amazonPK})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
	}
	noPayeeTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: noPayeeTxn, Amount: "-5.00"})
	transferTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-100.00", PostedDate: &day})
	transferLeg := b.Entry(v9fixture.EntryRow{Parent: transferTxn, Amount: "-100.00", QuickenID: 3003, Transfer: "Old Visa"})

	db, stdout := syncFindingsBundle(t, home, b)

	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	require.Equal(t, "Findings  3 open; run quarry findings to list them", lines[len(lines)-1])
	assert.Equal(t, map[string]string{
		fmt.Sprintf("uncategorized:payee-%d", amazonPK):        "uncategorized",
		"uncategorized:no-payee":                               "uncategorized",
		fmt.Sprintf("one-sided-transfer:xfer-%d", transferLeg): "one-sided-transfer",
	}, stringMap(t, db, "SELECT id, type FROM findings"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("uncategorized:payee-%d", amazonPK):        "true",
		"uncategorized:no-payee":                               "true",
		fmt.Sprintf("one-sided-transfer:xfer-%d", transferLeg): "true",
	}, stringMap(t, db, "SELECT id, CAST(first_found_at = (SELECT built_at FROM store_info) AS VARCHAR) FROM findings"))
}

func Test_run_sync_records_a_one_sided_transfer_with_its_from_split_as_the_item(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	landlordPK := b.Payee(v9fixture.PayeeRow{Name: "Landlord"})
	rentPK := b.Category(v9fixture.TagRow{Name: "Rent", Type: new(int64(1))})
	day := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	transferTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-500.00", PostedDate: &day, Payee: landlordPK})
	transferLeg := b.Entry(v9fixture.EntryRow{
		Parent: transferTxn, Amount: "-500.00", QuickenID: 3002, Transfer: "Savings", CategoryTag: rentPK,
	})

	db, _ := syncFindingsBundle(t, home, b)

	findingID := fmt.Sprintf("one-sided-transfer:xfer-%d", transferLeg)
	assert.Equal(t, map[string]string{
		findingID: fmt.Sprintf("txn-%d|split-%d|NULL|NULL", transferTxn, transferLeg),
	}, stringMap(t, db, `SELECT finding_id,
		transaction_id || '|' || split_id || '|' || COALESCE(payee_id, 'NULL') || '|' || COALESCE(category_id, 'NULL')
		FROM finding_items WHERE finding_id LIKE 'one-sided-transfer:%'`))
}

// cashFlowTotals is the part of cashflow --json the comparison reads.
type cashFlowTotals struct {
	Totals []struct {
		Currency string `json:"currency"`
		Income   string `json:"income"`
		Spent    string `json:"spent"`
	} `json:"totals"`
}

// Distinct magnitudes (1, 20, 300, 4000) make each total name the splits it sums.
func Test_run_sync_uncategorized_findings_hold_what_cashflow_counts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	closedPK := b.Account(v9fixture.AccountRow{Name: "Old Visa", Type: "CREDITCARD", Currency: "CAD", Closed: true})
	hiddenPK := b.Account(v9fixture.AccountRow{
		Name: "Side Account", Type: "SAVINGS", Currency: "CAD", Active: true, UsedInReports: new(int64(0)),
	})
	acmePK := b.Payee(v9fixture.PayeeRow{Name: "Acme"})
	betaPK := b.Payee(v9fixture.PayeeRow{Name: "Beta"})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	past := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2099, 6, 15, 0, 0, 0, 0, time.UTC)
	split := func(account int64, day time.Time, payee int64, amount string, category int64) int64 {
		txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &day, Payee: payee})
		return b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: category})
	}
	spentAcme := split(chequingPK, past, acmePK, "-1.00", 0)
	incomeAcme := split(chequingPK, past, acmePK, "20.00", 0)
	closedBeta := split(closedPK, past, betaPK, "-300.00", 0)
	futureBeta := split(chequingPK, future, betaPK, "4000.00", 0)
	split(hiddenPK, past, acmePK, "-50000.00", 0)
	split(hiddenPK, past, betaPK, "7.00", foodPK)

	db, _ := syncFindingsBundle(t, home, b)

	acme := fmt.Sprintf("uncategorized:payee-%d", acmePK)
	beta := fmt.Sprintf("uncategorized:payee-%d", betaPK)
	assert.Equal(t, map[string]string{
		fmt.Sprintf("split-%d", spentAcme): acme, fmt.Sprintf("split-%d", incomeAcme): acme,
		fmt.Sprintf("split-%d", closedBeta): beta, fmt.Sprintf("split-%d", futureBeta): beta,
	}, stringMap(t, db, `SELECT i.split_id, i.finding_id FROM finding_items i
		JOIN findings f ON f.id = i.finding_id WHERE f.type = 'uncategorized'`))

	itemTotals := stringMap(t, db, `
		SELECT 'income', CAST(sum(s.amount) FILTER (WHERE s.amount > 0) AS VARCHAR)
		FROM finding_items i JOIN splits s ON s.id = i.split_id JOIN findings f ON f.id = i.finding_id WHERE f.type = 'uncategorized'
		UNION ALL
		SELECT 'spent', CAST(-sum(s.amount) FILTER (WHERE s.amount < 0) AS VARCHAR)
		FROM finding_items i JOIN splits s ON s.id = i.split_id JOIN findings f ON f.id = i.finding_id WHERE f.type = 'uncategorized'`)
	var stdout, stderr bytes.Buffer
	exitCode := run(context.Background(), []string{"cashflow", "--json", "--by", "year", "--since", "2026", "--until", "2099"}, &stdout, &stderr)
	require.Equal(t, 0, exitCode, stderr.String())
	var flow cashFlowTotals
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &flow))
	require.Len(t, flow.Totals, 1)
	assert.Equal(t, "CAD", flow.Totals[0].Currency)
	assert.Equal(t, map[string]string{"income": "4020.00", "spent": "301.00"}, itemTotals)
	assert.Equal(t, map[string]string{"income": flow.Totals[0].Income, "spent": flow.Totals[0].Spent}, itemTotals)
}

func Test_run_sync_marks_a_finding_no_longer_found_fixed_at_the_build_time(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	first, amazonPK := uncategorizedPayeeBundle(false, "-10.00")
	second, _ := uncategorizedPayeeBundle(true, "-10.00")
	syncFindingsBundleIn(t, home, "DocumentsA", first)

	line := syncFindingsBundleIn(t, home, "DocumentsB", second)

	assert.Equal(t, "Findings  none open, 1 fixed since the last sync", line)
	assert.Equal(t, map[string]string{fmt.Sprintf("uncategorized:payee-%d", amazonPK): "true"},
		importRunQuery(t, home, "SELECT id, CAST(fixed_at = (SELECT built_at FROM store_info) AS VARCHAR) FROM findings"))
	assert.Equal(t, map[string]string{"items": "0"},
		importRunQuery(t, home, "SELECT 'items', CAST(count(*) AS VARCHAR) FROM finding_items"))
}

func Test_run_sync_reopens_a_fixed_finding_with_its_first_found_at_and_not_new(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	first, amazonPK := uncategorizedPayeeBundle(false, "-10.00")
	fixed, _ := uncategorizedPayeeBundle(true, "-10.00")
	reopened, _ := uncategorizedPayeeBundle(false, "-12.00")
	syncFindingsBundleIn(t, home, "DocumentsA", first)
	id := fmt.Sprintf("uncategorized:payee-%d", amazonPK)
	firstFoundAt := importRunQuery(t, home, "SELECT id, CAST(first_found_at AS VARCHAR) FROM findings")[id]
	syncFindingsBundleIn(t, home, "DocumentsB", fixed)

	line := syncFindingsBundleIn(t, home, "DocumentsC", reopened)

	assert.Equal(t, "Findings  1 open; run quarry findings to list them", line)
	assert.Equal(t, map[string]string{id: firstFoundAt + "|NULL"},
		importRunQuery(t, home, "SELECT id, CAST(first_found_at AS VARCHAR) || '|' || COALESCE(CAST(fixed_at AS VARCHAR), 'NULL') FROM findings"))
}

func Test_run_sync_prints_a_finding_first_seen_since_the_last_sync_as_new(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	first, _ := twoPayeeBundle(false, true)
	second, _ := twoPayeeBundle(false, false)
	syncFindingsBundleIn(t, home, "DocumentsA", first)

	line := syncFindingsBundleIn(t, home, "DocumentsB", second)

	assert.Equal(t, "Findings  2 open (1 new); run quarry findings to list them", line)
}

func Test_run_sync_json_counts_a_finding_fixed_since_the_last_sync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	first, _ := uncategorizedPayeeBundle(false, "-10.00")
	second, _ := uncategorizedPayeeBundle(true, "-10.00")
	syncFindingsBundleIn(t, home, "DocumentsA", first)
	bundle := second.WriteBundle(t, filepath.Join(home, "DocumentsB"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var parsed struct {
		Store struct {
			Findings json.RawMessage `json:"findings"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	assert.JSONEq(t, `{"open":0,"ignored":0,"fixed":1,"new":0,"newly_fixed":1}`, string(parsed.Store.Findings))
}

// twoPayeeBundle mints both payees in every call, so each finding id is the same in every sync.
func twoPayeeBundle(xCategorized, yCategorized bool) (*v9fixture.Builder, int64) {
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	xPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	yPK := b.Payee(v9fixture.PayeeRow{Name: "Landlord"})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, p := range []struct {
		payee       int64
		categorized bool
	}{{xPK, xCategorized}, {yPK, yCategorized}} {
		payee, categorized := p.payee, p.categorized
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-10.00", PostedDate: &day, Payee: payee})
		entry := v9fixture.EntryRow{Parent: txn, Amount: "-10.00"}
		if categorized {
			entry.CategoryTag = foodPK
		}
		b.Entry(entry)
	}
	return b, xPK
}

func Test_run_sync_from_an_older_snapshot_reopens_and_fixes_findings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	first, xPK := twoPayeeBundle(false, true)
	second, _ := twoPayeeBundle(true, false)
	syncFindingsBundleIn(t, home, "DocumentsA", first)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	firstID := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
	xID := fmt.Sprintf("uncategorized:payee-%d", xPK)
	firstFoundAt := importRunQuery(t, home, "SELECT id, CAST(first_found_at AS VARCHAR) FROM findings")[xID]
	syncFindingsBundleIn(t, home, "DocumentsB", second)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--from", firstID}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	assert.Equal(t, "Findings  1 open, 1 fixed since the last sync; run quarry findings to list them", lines[len(lines)-1])
	assert.Equal(t, firstFoundAt+"|NULL", importRunQuery(t, home,
		"SELECT id, CAST(first_found_at AS VARCHAR) || '|' || COALESCE(CAST(fixed_at AS VARCHAR), 'NULL') FROM findings")[xID])
}
