// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store/duckstore"
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

// syncFindingsBundleIn syncs b written under home/dir and returns sync's Findings line, leaving no store connection open.
func syncFindingsBundleIn(t *testing.T, home, dir string, b *v9fixture.Builder) string {
	t.Helper()
	bundle := b.WriteBundle(t, filepath.Join(home, dir))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode, stderr.String())
	return findingsLine(t, stdout.String())
}

// findingsLine is the Findings line of sync's stdout.
func findingsLine(t *testing.T, stdout string) string {
	t.Helper()
	lines := strings.Split(stdout, "\n")
	at := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, "Findings  ") })
	require.GreaterOrEqual(t, at, 0, stdout)
	return lines[at]
}

// uncategorizedPayeeBundle mints the same payee key on every call, so the finding id is the same in every sync.
func uncategorizedPayeeBundle(categorized bool, amount string) (*v9fixture.Builder, int64) {
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: foodPK})
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
	home := newHome(t)

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

	require.Equal(t, "Findings  3 open; run quarry findings to list them", findingsLine(t, stdout))
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
	home := newHome(t)

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
	home := newHome(t)

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
	exitCode, stdout, stderr := runCapture(context.Background(), []string{"cashflow", "--json", "--by", "year", "--since", "2026", "--until", "2099"})
	require.Equal(t, 0, exitCode, stderr.String())
	var flow cashFlowTotals
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &flow))
	require.Len(t, flow.Totals, 1)
	assert.Equal(t, "CAD", flow.Totals[0].Currency)
	assert.Equal(t, map[string]string{"income": "4020.00", "spent": "301.00"}, itemTotals)
	assert.Equal(t, map[string]string{"income": flow.Totals[0].Income, "spent": flow.Totals[0].Spent}, itemTotals)
}

func Test_run_sync_marks_a_finding_no_longer_found_fixed_at_the_build_time(t *testing.T) {
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
	first, _ := twoPayeeBundle(false, true)
	second, _ := twoPayeeBundle(false, false)
	syncFindingsBundleIn(t, home, "DocumentsA", first)

	line := syncFindingsBundleIn(t, home, "DocumentsB", second)

	assert.Equal(t, "Findings  2 open (1 new); run quarry findings to list them", line)
}

func Test_run_sync_json_counts_a_finding_fixed_since_the_last_sync(t *testing.T) {
	home := newHome(t)
	first, _ := uncategorizedPayeeBundle(false, "-10.00")
	second, _ := uncategorizedPayeeBundle(true, "-10.00")
	syncFindingsBundleIn(t, home, "DocumentsA", first)
	bundle := second.WriteBundle(t, filepath.Join(home, "DocumentsB"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"})

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
	b, xPK, _ := twoPayeeBundleKeys(xCategorized, yCategorized)
	return b, xPK
}

// twoPayeeBundleKeys is twoPayeeBundle that also returns the second payee's key.
func twoPayeeBundleKeys(xCategorized, yCategorized bool) (*v9fixture.Builder, int64, int64) {
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	xPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	yPK := b.Payee(v9fixture.PayeeRow{Name: "Landlord"})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: foodPK})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	// The amounts differ so the two transactions are not duplicates of each other.
	for _, p := range []struct {
		payee       int64
		categorized bool
		amount      string
	}{{xPK, xCategorized, "-10.00"}, {yPK, yCategorized, "-11.00"}} {
		payee, categorized := p.payee, p.categorized
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: p.amount, PostedDate: &day, Payee: payee})
		entry := v9fixture.EntryRow{Parent: txn, Amount: p.amount}
		if categorized {
			entry.CategoryTag = foodPK
		}
		b.Entry(entry)
	}
	return b, xPK, yPK
}

func Test_run_sync_from_an_older_snapshot_reopens_and_fixes_findings(t *testing.T) {
	home := newHome(t)
	first, xPK := twoPayeeBundle(false, true)
	second, _ := twoPayeeBundle(true, false)
	syncFindingsBundleIn(t, home, "DocumentsA", first)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	firstID := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
	xID := fmt.Sprintf("uncategorized:payee-%d", xPK)
	firstFoundAt := importRunQuery(t, home, "SELECT id, CAST(first_found_at AS VARCHAR) FROM findings")[xID]
	syncFindingsBundleIn(t, home, "DocumentsB", second)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--from", firstID})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Findings  1 open, 1 fixed since the last sync; run quarry findings to list them", findingsLine(t, stdout.String()))
	assert.Equal(t, firstFoundAt+"|NULL", importRunQuery(t, home,
		"SELECT id, CAST(first_found_at AS VARCHAR) || '|' || COALESCE(CAST(fixed_at AS VARCHAR), 'NULL') FROM findings")[xID])
}

// syncIgnoringTheNewFinding syncs x alone, then x and y under a config ignoring y, the finding first seen by the second sync.
func syncIgnoringTheNewFinding(t *testing.T, home string, extra ...string) (int, string, string) {
	t.Helper()
	first, _, _ := twoPayeeBundleKeys(false, true)
	second, _, yPK := twoPayeeBundleKeys(false, false)
	syncFindingsBundleIn(t, home, "DocumentsA", first)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [\"uncategorized:payee-%d\"]\n", yPK))
	return syncNewBundle(t, home, "DocumentsB", second, extra...)
}

func Test_run_sync_counts_an_ignored_finding_as_ignored_not_open_or_new(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := syncIgnoringTheNewFinding(t, home)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Findings  1 open, 1 ignored; run quarry findings to list them", findingsLine(t, stdout))
}

func Test_run_sync_json_counts_an_ignored_finding_as_ignored_not_open_or_new(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := syncIgnoringTheNewFinding(t, home, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var parsed struct {
		Store struct {
			Findings json.RawMessage `json:"findings"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	assert.JSONEq(t, `{"open":1,"ignored":1,"fixed":0,"new":0,"newly_fixed":0}`, string(parsed.Store.Findings))
}

func Test_run_sync_counts_a_new_open_finding_beside_an_ignored_one(t *testing.T) {
	home := newHome(t)
	first, xPK, _ := twoPayeeBundleKeys(false, true)
	second, _, _ := twoPayeeBundleKeys(false, false)
	syncFindingsBundleIn(t, home, "DocumentsA", first)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [\"uncategorized:payee-%d\"]\n", xPK))

	exitCode, stdout, stderr := syncNewBundle(t, home, "DocumentsB", second)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Findings  1 open (1 new), 1 ignored; run quarry findings to list them", findingsLine(t, stdout))
}

func Test_run_sync_from_counts_an_ignored_finding_as_ignored_not_open(t *testing.T) {
	home := newHome(t)
	first, xPK, _ := twoPayeeBundleKeys(false, true)
	syncFindingsBundleIn(t, home, "DocumentsA", first)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	firstID := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [\"uncategorized:payee-%d\"]\n", xPK))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--from", firstID})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Findings  none open, 1 ignored", findingsLine(t, stdout.String()))
}

func Test_run_sync_says_nothing_about_an_ignored_id_that_is_not_a_finding(t *testing.T) {
	home := newHome(t)
	first, _, _ := twoPayeeBundleKeys(false, true)
	writeConfig(t, home, "[findings]\nignore = [\"uncategorized:payee-999\"]\n")

	exitCode, stdout, stderr := syncNewBundle(t, home, "DocumentsA", first, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	var parsed struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	assert.Empty(t, parsed.Warnings)
}

const unclassifiedOpenLine = "Findings  1 open; run quarry findings to list them"

func Test_run_sync_counts_an_unclassified_account_open_and_never_new(t *testing.T) {
	home := newHome(t)
	first, _ := unclassifiedAccountBundle()
	second, _ := unclassifiedAccountBundle()

	firstLine := syncFindingsBundleIn(t, home, "DocumentsA", first)
	_, stdout, _ := syncNewBundle(t, home, "DocumentsB", second)

	assert.Equal(t, unclassifiedOpenLine, firstLine)
	assert.Equal(t, unclassifiedOpenLine, findingsLine(t, stdout))
}

func Test_run_sync_leaves_an_account_the_config_classifies_out_of_the_findings(t *testing.T) {
	home := newHome(t)
	b, id := unclassifiedAccountBundle()
	writeConfig(t, home, fmt.Sprintf("[accounts]\nregistered = [%q]\n", id))

	line := syncFindingsBundleIn(t, home, "Documents", b)

	assert.Equal(t, "Findings  none open", line)
}

func Test_run_sync_counts_an_ignored_unclassified_account_as_ignored(t *testing.T) {
	home := newHome(t)
	b, id := unclassifiedAccountBundle()
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [\"unclassified-account:%s\"]\n", id))

	line := syncFindingsBundleIn(t, home, "Documents", b)

	assert.Equal(t, "Findings  none open, 1 ignored", line)
}

func Test_run_sync_json_counts_an_unclassified_account_open_and_not_new(t *testing.T) {
	home := newHome(t)
	b, _ := unclassifiedAccountBundle()

	exitCode, stdout, stderr := syncNewBundle(t, home, "Documents", b, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var parsed struct {
		Store struct {
			Findings json.RawMessage `json:"findings"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	assert.JSONEq(t, `{"open":1,"ignored":0,"fixed":0,"new":0,"newly_fixed":0}`, string(parsed.Store.Findings))
}

func Test_run_sync_from_counts_an_unclassified_account_open(t *testing.T) {
	home := newHome(t)
	b, _ := unclassifiedAccountBundle()
	syncFindingsBundleIn(t, home, "Documents", b)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--from", id})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, unclassifiedOpenLine, findingsLine(t, stdout.String()))
}

// reconcileStatus is ZRECONCILESTATUS for a reconciled transaction.
const reconcileStatus = 2

// categorizedTxn adds a categorized transaction of amount dated day, so the only finding it can raise is a duplicate.
func categorizedTxn(b *v9fixture.Builder, account, category int64, day time.Time, amount string, status *int64) int64 {
	txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &day, Status: status})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: category})
	return txn
}

func Test_run_sync_records_two_same_amount_transactions_within_three_days_as_a_duplicate(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	first := categorizedTxn(b, chequingPK, foodPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17", nil)
	second := categorizedTxn(b, chequingPK, foodPK, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-142.17", nil)

	db, _ := syncFindingsBundle(t, home, b)

	id := fmt.Sprintf("duplicate:txn-%d+txn-%d", first, second)
	assert.Equal(t, map[string]string{id: "duplicate"}, stringMap(t, db, "SELECT id, type FROM findings WHERE type = 'duplicate'"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", first): id, fmt.Sprintf("txn-%d", second): id,
	}, stringMap(t, db, "SELECT transaction_id, finding_id FROM finding_items WHERE finding_id LIKE 'duplicate:%'"))
}

// The reconciled/uncleared pair is the control: one reconciled side is still flagged.
func Test_run_sync_does_not_flag_two_reconciled_look_alikes_as_a_duplicate(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	reconciled := new(int64(reconcileStatus))
	day := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	categorizedTxn(b, chequingPK, foodPK, day, "-142.17", reconciled)
	categorizedTxn(b, chequingPK, foodPK, day.AddDate(0, 0, 1), "-142.17", reconciled)
	controlReconciled := categorizedTxn(b, chequingPK, foodPK, day, "-9.99", reconciled)
	controlUncleared := categorizedTxn(b, chequingPK, foodPK, day.AddDate(0, 0, 1), "-9.99", nil)

	db, _ := syncFindingsBundle(t, home, b)

	assert.Equal(t, map[string]string{
		fmt.Sprintf("duplicate:txn-%d+txn-%d", controlReconciled, controlUncleared): "duplicate",
	}, stringMap(t, db, "SELECT id, type FROM findings WHERE type = 'duplicate'"))
}

// Shell is the control: it moves from Auto to Auto:Fuel once and stays there.
func Test_run_sync_records_costco_as_mixed_categories_and_not_shell(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	costcoPK := b.Payee(v9fixture.PayeeRow{Name: "Costco"})
	shellPK := b.Payee(v9fixture.PayeeRow{Name: "Shell"})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	householdPK := b.Category(v9fixture.TagRow{Name: "Household", Type: new(int64(1))})
	autoPK := b.Category(v9fixture.TagRow{Name: "Auto", Type: new(int64(1))})
	fuelPK := b.Category(v9fixture.TagRow{Name: "Fuel", Type: new(int64(1)), ParentCategory: autoPK})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	spend := func(payee, category int64, amount string) {
		day = day.AddDate(0, 0, 10)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: payee})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: category})
	}
	spend(costcoPK, groceriesPK, "-101.00")
	spend(costcoPK, householdPK, "-102.00")
	spend(costcoPK, groceriesPK, "-103.00")
	spend(costcoPK, fuelPK, "-104.00")
	spend(shellPK, autoPK, "-51.00")
	spend(shellPK, autoPK, "-52.00")
	spend(shellPK, fuelPK, "-53.00")
	spend(shellPK, fuelPK, "-54.00")

	db, _ := syncFindingsBundle(t, home, b)

	id := fmt.Sprintf("mixed-categories:payee-%d", costcoPK)
	assert.Equal(t, map[string]string{id: "mixed-categories"}, stringMap(t, db, "SELECT id, type FROM findings WHERE type = 'mixed-categories'"))
	assert.Equal(t, map[string]string{
		"Groceries": "Costco|NULL", "Household": "Costco|NULL", "Fuel": "Costco|NULL",
	}, stringMap(t, db, `SELECT c.name, p.name || '|' || COALESCE(fi.transaction_id, 'NULL')
		FROM finding_items fi
		JOIN categories c ON c.id = fi.category_id
		JOIN payees p ON p.id = fi.payee_id
		WHERE fi.finding_id LIKE 'mixed-categories:%'`))
}

// "Tim Hortons Cafe" is the control: its key (tim-hortons-cafe) differs.
func Test_run_sync_records_tim_hortons_variants_and_not_unrelated_payees(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	numberedPK := b.Payee(v9fixture.PayeeRow{Name: "TIM HORTONS #1234"})
	plainPK := b.Payee(v9fixture.PayeeRow{Name: "Tim Hortons"})
	cafePK := b.Payee(v9fixture.PayeeRow{Name: "Tim Hortons Cafe"})
	shellPK := b.Payee(v9fixture.PayeeRow{Name: "Shell"})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	spend := func(payee int64, amount string) {
		day = day.AddDate(0, 0, 10)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: payee})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: groceriesPK})
	}
	spend(numberedPK, "-3.01")
	spend(numberedPK, "-3.02")
	spend(plainPK, "-4.01")
	spend(cafePK, "-5.01")
	spend(shellPK, "-6.01")

	db, _ := syncFindingsBundle(t, home, b)

	assert.Equal(t, map[string]string{"payee-variants:tim-hortons": "payee-variants"}, stringMap(t, db,
		"SELECT id, type FROM findings WHERE type = 'payee-variants'"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("payee-%d", numberedPK): "TIM HORTONS #1234|NULL|NULL",
		fmt.Sprintf("payee-%d", plainPK):    "Tim Hortons|NULL|NULL",
	}, stringMap(t, db, `SELECT fi.payee_id, p.name || '|' || COALESCE(fi.transaction_id, 'NULL') || '|' || COALESCE(fi.category_id, 'NULL')
		FROM finding_items fi
		JOIN payees p ON p.id = fi.payee_id
		WHERE fi.finding_id = 'payee-variants:tim-hortons'`))
}

// "Auto" is the control: its key differs. The income "Grocery" shares the
// expense key but is a different kind, so it joins no group.
func Test_run_sync_records_similar_expense_categories_and_not_the_income_one(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	groceryPK := b.Category(v9fixture.TagRow{Name: "Grocery", Type: new(int64(1))})
	b.Category(v9fixture.TagRow{Name: "Grocery", Type: new(int64(2))})
	b.Category(v9fixture.TagRow{Name: "Auto", Type: new(int64(1))})

	db, _ := syncFindingsBundle(t, home, b)

	assert.Equal(t, map[string]string{"similar-categories:grocery": "similar-categories"}, stringMap(t, db,
		"SELECT id, type FROM findings WHERE type = 'similar-categories'"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("cat-%d", groceriesPK): "NULL|NULL",
		fmt.Sprintf("cat-%d", groceryPK):   "NULL|NULL",
	}, stringMap(t, db, `SELECT fi.category_id, COALESCE(fi.transaction_id, 'NULL') || '|' || COALESCE(fi.payee_id, 'NULL')
		FROM finding_items fi
		WHERE fi.finding_id = 'similar-categories:grocery'`))
}

// The CAD/USD pair is the control: same magnitude, opposite signs, within the window, but different currencies.
func Test_run_sync_records_opposite_amounts_in_two_cad_accounts_as_an_unlinked_transfer_and_not_the_cad_usd_pair(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CREDITCARD", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	out := categorizedTxn(b, chequingPK, foodPK, time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), "-500.00", nil)
	in := categorizedTxn(b, visaPK, foodPK, time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC), "500.00", nil)
	categorizedTxn(b, chequingPK, foodPK, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "-75.00", nil)
	categorizedTxn(b, savingsPK, foodPK, time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), "75.00", nil)

	db, _ := syncFindingsBundle(t, home, b)

	id := fmt.Sprintf("unlinked-transfer:txn-%d+txn-%d", out, in)
	assert.Equal(t, map[string]string{id: "unlinked-transfer"}, stringMap(t, db, "SELECT id, type FROM findings WHERE type = 'unlinked-transfer'"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", out): id, fmt.Sprintf("txn-%d", in): id,
	}, stringMap(t, db, "SELECT transaction_id, finding_id FROM finding_items WHERE finding_id LIKE 'unlinked-transfer:%'"))
}

// "Brokerage Fees" is used only by an investment entry and "Charity" only by a
// budget line, neither of which the importer stores as a split.
func Test_run_sync_records_unused_categories_but_not_one_an_investment_or_budget_uses(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	parkingPK := b.Category(v9fixture.TagRow{Name: "Parking", Type: new(int64(1))})
	feesPK := b.Category(v9fixture.TagRow{Name: "Brokerage Fees", Type: new(int64(1))})
	charityPK := b.Category(v9fixture.TagRow{Name: "Charity", Type: new(int64(1))})
	b.Category(v9fixture.TagRow{Name: "Old", Type: new(int64(1)), Hidden: true})
	vacationPK := b.Category(v9fixture.TagRow{Name: "Vacation", Type: new(int64(1))})
	hotelPK := b.Category(v9fixture.TagRow{Name: "Hotel", Type: new(int64(1)), ParentCategory: vacationPK})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	buyPK := b.InvestmentTransaction(v9fixture.TransactionRow{
		Type: new(int64(3)), Account: brokeragePK, Amount: "-400.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: buyPK, Amount: "-400.00", CategoryTag: feesPK})
	b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: charityPK})

	db, _ := syncFindingsBundle(t, home, b)

	assert.Equal(t, map[string]string{
		fmt.Sprintf("unused-category:cat-%d", parkingPK):  "unused-category",
		fmt.Sprintf("unused-category:cat-%d", vacationPK): "unused-category",
	}, stringMap(t, db, "SELECT id, type FROM findings WHERE type = 'unused-category'"))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("cat-%d", vacationPK): "NULL|NULL",
		fmt.Sprintf("cat-%d", hotelPK):    "NULL|NULL",
	}, stringMap(t, db, fmt.Sprintf(`SELECT fi.category_id, COALESCE(fi.transaction_id, 'NULL') || '|' || COALESCE(fi.payee_id, 'NULL')
		FROM finding_items fi
		WHERE fi.finding_id = 'unused-category:cat-%d'`, vacationPK)))
}

// syncedStore syncs the bundle build describes under a fresh HOME and opens the store read-only.
func syncedStore(t *testing.T, build func(b *v9fixture.Builder)) *duckdb.DB {
	t.Helper()
	home := newHome(t)
	b := v9fixture.NewBuilder()
	build(b)
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	db, err := duckdb.OpenReadOnly(t.Context(), filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func Test_run_sync_records_which_transactions_are_excluded_from_reports(t *testing.T) {
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	var excludedPK, includedPK int64
	db := syncedStore(t, func(b *v9fixture.Builder) {
		acct := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
		excludedPK = b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-5.00", PostedDate: &day, ExcludeFromReports: new(int64(1))})
		b.Entry(v9fixture.EntryRow{Parent: excludedPK, Amount: "-5.00"})
		includedPK = b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-7.00", PostedDate: &day})
		b.Entry(v9fixture.EntryRow{Parent: includedPK, Amount: "-7.00"})
	})

	got := stringMap(t, db, "SELECT id, CAST(excluded_from_reports AS VARCHAR) FROM transactions")

	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", excludedPK): "true",
		fmt.Sprintf("txn-%d", includedPK): "false",
	}, got)
}

func Test_run_sync_records_which_accounts_are_used_in_reports(t *testing.T) {
	var offPK, onPK, unsetPK int64
	db := syncedStore(t, func(b *v9fixture.Builder) {
		offPK = b.Account(v9fixture.AccountRow{Name: "Off", Type: "CHECKING", Currency: "CAD", Active: true, UsedInReports: new(int64(0))})
		onPK = b.Account(v9fixture.AccountRow{Name: "On", Type: "CHECKING", Currency: "CAD", Active: true, UsedInReports: new(int64(1))})
		unsetPK = b.Account(v9fixture.AccountRow{Name: "Unset", Type: "CHECKING", Currency: "CAD", Active: true})
	})

	got := stringMap(t, db, "SELECT id, CAST(in_reports AS VARCHAR) FROM accounts")

	assert.Equal(t, map[string]string{
		fmt.Sprintf("acct-%d", offPK):   "false",
		fmt.Sprintf("acct-%d", onPK):    "true",
		fmt.Sprintf("acct-%d", unsetPK): "true",
	}, got)
}

func Test_run_sync_dates_each_transaction_by_its_register_date(t *testing.T) {
	entered := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	posted := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	var bothPK, enteredOnlyPK int64
	db := syncedStore(t, func(b *v9fixture.Builder) {
		acct := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
		bothPK = b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-5.00", EnteredDate: &entered, PostedDate: &posted})
		b.Entry(v9fixture.EntryRow{Parent: bothPK, Amount: "-5.00"})
		enteredOnlyPK = b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-7.00", EnteredDate: &entered})
		b.Entry(v9fixture.EntryRow{Parent: enteredOnlyPK, Amount: "-7.00"})
	})

	dates := stringMap(t, db, "SELECT id, CAST(date AS VARCHAR) FROM transactions")
	postedDates := stringMap(t, db, "SELECT id, COALESCE(CAST(posted_date AS VARCHAR), 'NULL') FROM transactions")

	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", bothPK):        "2026-06-01",
		fmt.Sprintf("txn-%d", enteredOnlyPK): "2026-06-01",
	}, dates)
	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", bothPK):        "2026-05-31",
		fmt.Sprintf("txn-%d", enteredOnlyPK): "NULL",
	}, postedDates)
}

func Test_run_sync_stores_uncategorized_splits_with_no_category(t *testing.T) {
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	var uncategorizedSplitPK int64
	db := syncedStore(t, func(b *v9fixture.Builder) {
		acct := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
		uncategorizedPK := b.Category(v9fixture.TagRow{Name: "Uncategorized", Type: new(int64(0))})
		txn := b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-5.00", PostedDate: &day})
		uncategorizedSplitPK = b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-5.00", CategoryTag: uncategorizedPK})
	})

	got := stringMap(t, db, "SELECT id, COALESCE(category_id, 'NULL') FROM splits")

	assert.Equal(t, map[string]string{fmt.Sprintf("split-%d", uncategorizedSplitPK): "NULL"}, got)
}

// versionFiveStoreDDL is a store as format 5 left it: the 19 Phase 1 import_runs columns, the nine optional ones
// and store_info, none of the four investment count columns, and one run (id 7) that skipped 13 investment transactions.
const versionFiveStoreDDL = `CREATE TABLE store_info (format_version INTEGER NOT NULL, quarry_version VARCHAR NOT NULL, built_at TIMESTAMP NOT NULL);
INSERT INTO store_info VALUES (5, 'v0.5.0', '2026-06-01 10:00:02');
CREATE TABLE import_runs (
	id BIGINT PRIMARY KEY, started_at TIMESTAMP NOT NULL, finished_at TIMESTAMP NOT NULL,
	snapshot_path VARCHAR NOT NULL, snapshot_sha256 VARCHAR NOT NULL, schema_fingerprint VARCHAR NOT NULL,
	accounts_rows BIGINT NOT NULL, categories_rows BIGINT NOT NULL, payees_rows BIGINT NOT NULL, tags_rows BIGINT NOT NULL,
	transactions_rows BIGINT NOT NULL, splits_rows BIGINT NOT NULL, split_tags_rows BIGINT NOT NULL, transfers_rows BIGINT NOT NULL,
	balances_checked BIGINT NOT NULL, balances_mismatched BIGINT NOT NULL, splits_mismatched BIGINT NOT NULL,
	transfers_one_sided BIGINT NOT NULL, investment_transactions_not_imported BIGINT NOT NULL,
	snapshot_taken_at TIMESTAMP, source_path VARCHAR, balances_never_reconciled BIGINT, investment_accounts BIGINT,
	transfers_paired BIGINT, transfers_cross_currency BIGINT, rates_checked_from DATE, rates_last DATE, rates_fetch_error VARCHAR);
INSERT INTO import_runs (id, started_at, finished_at, snapshot_path, snapshot_sha256, schema_fingerprint,
	accounts_rows, categories_rows, payees_rows, tags_rows, transactions_rows, splits_rows, split_tags_rows, transfers_rows,
	balances_checked, balances_mismatched, splits_mismatched, transfers_one_sided, investment_transactions_not_imported)
VALUES (7, '2026-06-01 10:00:00', '2026-06-01 10:00:02', '/snapshots/version-five.sqlite', 'abc', 'sha256:fp',
	3, 2, 1, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13);`

func Test_run_sync_twice_over_a_version_5_store_carries_import_history_forward(t *testing.T) {
	home := newHome(t)
	writeStoreFixture(t, home, versionFiveStoreDDL)
	bundle := writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing")
	var firstOut, firstErr, secondOut, secondErr bytes.Buffer

	first := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &firstOut, &firstErr)
	second := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &secondOut, &secondErr)

	assert.Equal(t, 0, first, firstErr.String())
	assert.Equal(t, 0, second, secondErr.String())
	assert.Empty(t, firstErr.String())
	assert.Empty(t, secondErr.String())
	assert.Equal(t, map[string]string{"7": "/snapshots/version-five.sqlite", "8": "new run", "9": "new run"},
		importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), CASE WHEN id = 7 THEN snapshot_path ELSE 'new run' END FROM import_runs"))
	assert.Equal(t, map[string]string{"7": "3 2 1 4 5 6 7 8 9 10 11 12 true"},
		importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), concat_ws(' ', accounts_rows, categories_rows, payees_rows, tags_rows, "+
			"transactions_rows, splits_rows, split_tags_rows, transfers_rows, balances_checked, balances_mismatched, splits_mismatched, "+
			"transfers_one_sided, CAST(securities_rows IS NULL AND prices_rows IS NULL AND investment_transactions_rows IS NULL "+
			"AND shares_checked IS NULL AS VARCHAR)) FROM import_runs WHERE id = 7"))
	assert.Equal(t, map[string]string{"format_version": strconv.Itoa(duckstore.FormatVersion)},
		importRunQuery(t, home, "SELECT 'format_version', CAST(format_version AS VARCHAR) FROM store_info"))
	assert.Empty(t, importRunQuery(t, home, "SELECT column_name, '' FROM duckdb_columns() "+
		"WHERE table_name = 'import_runs' AND column_name = 'investment_transactions_not_imported'"))
}

// recordingTransport answers every request with an empty Valet answer and keeps the request paths.
type recordingTransport struct{ paths []string }

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.paths = append(r.paths, req.URL.Path)
	return valetResponse(req, http.StatusOK, `{"observations":[]}`), nil
}

// Swaps http.DefaultTransport, which the shipped wiring's Valet client uses: no t.Parallel.
func Test_the_shipped_sync_fetches_exchange_rates_from_the_valet_series(t *testing.T) {
	home := newHome(t)
	transport := &recordingTransport{}
	saved := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = saved })
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-5.00"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := runProcess(t.Context(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, transport.paths, "/valet/observations/FXUSDCAD/json")
}
