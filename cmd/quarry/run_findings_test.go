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
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addUncategorizedPayeeSplits adds two uncategorized charges by payee in account, on 2026-03-01 and 2026-03-02:
// one uncategorized finding for the payee.
func addUncategorizedPayeeSplits(b *v9fixture.Builder, account, payee int64) {
	for i, amount := range []string{"-10.00", "-20.00"} {
		posted := time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC)
		txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &posted, Payee: payee})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
	}
}

// seedStoreWithoutFindings builds quarry's store under home from one categorized fuel charge: no finding is open.
func seedStoreWithoutFindings(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-1", 1)},
		spendSplit{id: "1", account: "acct-1", category: "cat-fuel", payee: "payee-costco", currency: "CAD", day: day(2026, 9, 1), cents: -4500}))
}

func Test_run_findings_lists_open_findings_by_type_with_their_fix(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CHECKING", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	hydroNetworksPK := b.Payee(v9fixture.PayeeRow{Name: "HYDRO ONE NETWORKS"})
	paymentPK := b.Payee(v9fixture.PayeeRow{Name: "Payment"})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	first := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17")
	second := categorizedPayeeTxn(b, chequingPK, hydroNetworksPK, billsPK, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-142.17")
	transferDay := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	transferTxn := b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "1200.00", PostedDate: &transferDay, Payee: paymentPK})
	transferLeg := b.Entry(v9fixture.EntryRow{Parent: transferTxn, Amount: "1200.00", QuickenID: 3001, Transfer: "Savings"})
	addUncategorizedPayeeSplits(b, chequingPK, amazonPK)
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Possible duplicates (1): delete the extra one in Quicken, or ignore the pair if both are real
  duplicate:txn-%d+txn-%d
    2026-08-03  Chequing (CAD)  Hydro One           -142.17
    2026-08-05  Chequing (CAD)  HYDRO ONE NETWORKS  -142.17

One-sided transfers (1): re-enter each as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file
  one-sided-transfer:xfer-%d  2024-02-01  Visa (CAD)  Payment  1,200.00  other account: Savings (not in this file)

Uncategorized (1 payee, 2 splits): give each payee's splits a category in Quicken
  uncategorized:payee-%d  Amazon  2 splits  2026-03-01 to 2026-03-02

3 open findings
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, first, second, transferLeg, amazonPK, configShown), stdout.String())
}

func Test_run_findings_says_no_open_findings_when_the_store_has_none(t *testing.T) {
	home := newHome(t)
	seedStoreWithoutFindings(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "No open findings\n", stdout.String())
}

func Test_run_findings_json_with_none_open_prints_empty_lists_not_null(t *testing.T) {
	home := newHome(t)
	seedStoreWithoutFindings(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.JSONEq(t, `{
  "status": "open",
  "type": null,
  "counts": {"open": 0, "ignored": 0, "fixed": 0, "new": 0, "newly_fixed": 0},
  "findings": [],
  "warnings": []
}`, stdout.String())
}

func Test_run_findings_refuses_a_store_built_before_findings_existed(t *testing.T) {
	home := newHome(t)
	writeStoreFixture(t, home, phaseOneImportRunsDDL+
		"CREATE TABLE store_info (format_version INTEGER, quarry_version VARCHAR, built_at TIMESTAMP);"+
		"INSERT INTO store_info VALUES (3, '0.3.0', TIMESTAMP '2026-09-27 14:30:05');")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: the store at "+abbreviated(t, storePathUnder(home), home)+
		" was built by another version of quarry; run quarry sync --from 20260927T143005Z to rebuild it\n",
		stderr.String())
}

// categorizedPayeeTxn adds a categorized transaction of amount by payee dated posted, so the only finding it can raise is a duplicate.
func categorizedPayeeTxn(b *v9fixture.Builder, account, payee, category int64, posted time.Time, amount string) int64 {
	txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &posted, Payee: payee})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: category})
	return txn
}

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

			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

// pinFirstFoundAt sets the build time and every finding's first_found_at to one fixed moment, so the findings stay new.
func pinFirstFoundAt(t *testing.T, home string) {
	t.Helper()
	editStore(t, home, "UPDATE findings SET first_found_at = TIMESTAMP '2026-09-30 14:15:02'; UPDATE store_info SET built_at = TIMESTAMP '2026-09-30 14:15:02'")
}

func Test_run_findings_json_prints_the_ruled_document_for_one_open_duplicate(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	first := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17")
	second := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "-142.17")
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	pinFirstFoundAt(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.JSONEq(t, fmt.Sprintf(`{
  "status": "open",
  "type": null,
  "counts": {"open": 1, "ignored": 0, "fixed": 0, "new": 1, "newly_fixed": 0},
  "findings": [
    {
      "id": "duplicate:txn-%[1]d+txn-%[2]d",
      "type": "duplicate",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Delete the extra one in Quicken, or ignore the pair if both are real",
      "items": [
        {"transaction_id": "txn-%[1]d", "split_id": null, "payee_id": null, "category_id": null,
         "date": "2026-08-03", "account_id": "acct-%[3]d", "account": "Chequing", "currency": "CAD",
         "payee": "Hydro One", "category": null, "amount": "-142.17",
         "other_account": null, "other_account_id": null, "transactions": null, "splits": null, "investment_transaction_id": null, "security_id": null, "security": null, "shares": null},
        {"transaction_id": "txn-%[2]d", "split_id": null, "payee_id": null, "category_id": null,
         "date": "2026-08-05", "account_id": "acct-%[3]d", "account": "Chequing", "currency": "CAD",
         "payee": "Hydro One", "category": null, "amount": "-142.17",
         "other_account": null, "other_account_id": null, "transactions": null, "splits": null, "investment_transaction_id": null, "security_id": null, "security": null, "shares": null}
      ]
    }
  ],
  "warnings": []
}`, first, second, chequingPK), stdout.String())
}

func Test_run_findings_json_items_of_a_one_sided_transfer_and_an_uncategorized_payee_carry_their_own_fields(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CHECKING", Currency: "USD", Active: true})
	paymentPK := b.Payee(v9fixture.PayeeRow{Name: "Payment"})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	transferDay := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	transferTxn := b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "-1200.50", PostedDate: &transferDay, Payee: paymentPK})
	transferLeg := b.Entry(v9fixture.EntryRow{Parent: transferTxn, Amount: "-1200.50", QuickenID: 3001, Transfer: "Savings"})
	uncategorizedDay := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	uncategorizedTxn := b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "-10.00", PostedDate: &uncategorizedDay, Payee: amazonPK})
	uncategorizedSplit := b.Entry(v9fixture.EntryRow{Parent: uncategorizedTxn, Amount: "-10.00"})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	pinFirstFoundAt(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.JSONEq(t, fmt.Sprintf(`{
  "status": "open",
  "type": null,
  "counts": {"open": 2, "ignored": 0, "fixed": 0, "new": 2, "newly_fixed": 0},
  "findings": [
    {
      "id": "one-sided-transfer:xfer-%[1]d",
      "type": "one-sided-transfer",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Re-enter it as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file",
      "items": [
        {"transaction_id": "txn-%[2]d", "split_id": "split-%[1]d", "payee_id": null, "category_id": null,
         "date": "2024-02-01", "account_id": "acct-%[3]d", "account": "Visa", "currency": "USD",
         "payee": "Payment", "category": null, "amount": "-1200.50",
         "other_account": "Savings", "other_account_id": null, "transactions": null, "splits": null, "investment_transaction_id": null, "security_id": null, "security": null, "shares": null}
      ]
    },
    {
      "id": "uncategorized:payee-%[4]d",
      "type": "uncategorized",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Give this payee's splits a category in Quicken",
      "items": [
        {"transaction_id": "txn-%[5]d", "split_id": "split-%[6]d", "payee_id": null, "category_id": null,
         "date": "2026-03-01", "account_id": "acct-%[3]d", "account": "Visa", "currency": "USD",
         "payee": "Amazon", "category": null, "amount": "-10.00",
         "other_account": null, "other_account_id": null, "transactions": null, "splits": null, "investment_transaction_id": null, "security_id": null, "security": null, "shares": null}
      ]
    }
  ],
  "warnings": []
}`, transferLeg, transferTxn, visaPK, amazonPK, uncategorizedTxn, uncategorizedSplit), stdout.String())
}

func Test_run_findings_json_lists_a_config_warning_without_the_prefix_and_prints_it_to_stderr(t *testing.T) {
	home := newHome(t)
	seedStoreWithoutFindings(t, home)
	writeConfig(t, home, "snapshot.keep = 3\n")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Findings []any    `json:"findings"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []any{}, doc.Findings)
	assert.Equal(t, []string{configPath(home) + ": unknown key snapshot.keep; quarry ignores it"}, doc.Warnings)
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n", stderr.String())
}

func Test_run_findings_reports_a_failed_stdout_write(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "text", args: []string{"findings"}},
		{name: "json", args: []string{"findings", "--json"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			seedStoreWithoutFindings(t, home)
			var stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, failingWriter{err: errNoSpace}, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Equal(t, "quarry: cannot write the result to stdout: no space left on device\n", stderr.String())
		})
	}
}

// filtersFixtureIDs are the ids of the three duplicate pairs and the uncategorized payee syncFiltersBundle raises.
type filtersFixtureIDs struct {
	open, ignored, fixed, uncategorized string
}

// syncFiltersBundle syncs three duplicate pairs and one uncategorized payee into dir; the pair
// that is fixed is only there when withFixed, so a second sync without it fixes that finding.
func syncFiltersBundle(t *testing.T, home, dir string, withFixed bool) filtersFixtureIDs {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	hydroNetworksPK := b.Payee(v9fixture.PayeeRow{Name: "HYDRO ONE NETWORKS"})
	rogersPK := b.Payee(v9fixture.PayeeRow{Name: "Rogers"})
	rogersWirelessPK := b.Payee(v9fixture.PayeeRow{Name: "ROGERS WIRELESS"})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	netflixPK := b.Payee(v9fixture.PayeeRow{Name: "Netflix"})
	netflixComPK := b.Payee(v9fixture.PayeeRow{Name: "NETFLIX.COM"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	day := func(month time.Month, d int) time.Time { return time.Date(2026, month, d, 0, 0, 0, 0, time.UTC) }
	openFirst := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, day(time.August, 3), "-142.17")
	openSecond := categorizedPayeeTxn(b, chequingPK, hydroNetworksPK, billsPK, day(time.August, 5), "-142.17")
	ignoredFirst := categorizedPayeeTxn(b, chequingPK, rogersPK, billsPK, day(time.August, 20), "-55.00")
	ignoredSecond := categorizedPayeeTxn(b, chequingPK, rogersWirelessPK, billsPK, day(time.August, 21), "-55.00")
	addUncategorizedPayeeSplits(b, chequingPK, amazonPK)
	ids := filtersFixtureIDs{
		open:          fmt.Sprintf("duplicate:txn-%d+txn-%d", openFirst, openSecond),
		ignored:       fmt.Sprintf("duplicate:txn-%d+txn-%d", ignoredFirst, ignoredSecond),
		uncategorized: fmt.Sprintf("uncategorized:payee-%d", amazonPK),
	}
	if withFixed {
		fixedFirst := categorizedPayeeTxn(b, chequingPK, netflixPK, billsPK, day(time.July, 1), "-31.25")
		fixedSecond := categorizedPayeeTxn(b, chequingPK, netflixComPK, billsPK, day(time.July, 2), "-31.25")
		ids.fixed = fmt.Sprintf("duplicate:txn-%d+txn-%d", fixedFirst, fixedSecond)
	}
	exitCode, _, stderr := syncNewBundle(t, home, dir, b)
	require.Equal(t, 0, exitCode, stderr)
	return ids
}

// inZone sets the local time zone to zone for the test, so a local date differs from the UTC one.
func inZone(t *testing.T, zone *time.Location) {
	t.Helper()
	saved := time.Local                      //nolint:gosmopolitan // the test swaps the process-local zone; Cleanup restores it
	time.Local = zone                        //nolint:gosmopolitan // see above
	t.Cleanup(func() { time.Local = saved }) //nolint:gosmopolitan // restores the zone
}

// syncFiltersStore builds the filters fixture under home: an open, an ignored (by findings.ignore) and a
// fixed duplicate pair, fixed at 03:00 UTC on 2026-10-02, plus an open uncategorized payee.
func syncFiltersStore(t *testing.T, home string, extraIgnore ...string) filtersFixtureIDs {
	t.Helper()
	ids := syncFiltersBundle(t, home, "DocumentsA", true)
	syncFiltersBundle(t, home, "DocumentsB", false)
	editStore(t, home, fmt.Sprintf("UPDATE findings SET fixed_at = TIMESTAMP '2026-10-02 03:00:00' WHERE id = '%s'", ids.fixed))
	ignore := append([]string{ids.ignored}, extraIgnore...)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%s]\n", quotedList(ignore)))
	return ids
}

// quotedList is items as TOML basic strings separated by commas.
func quotedList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return strings.Join(quoted, ", ")
}

func Test_run_findings_status_all_type_duplicate_marks_ignored_shows_fixed_as_a_date_line_and_counts_duplicates_only(t *testing.T) {
	home := newHome(t)
	inZone(t, time.FixedZone("UTC-5", -5*60*60))
	ids := syncFiltersStore(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--status", "all", "--type", "duplicate"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Possible duplicates (3): delete the extra one in Quicken, or ignore the pair if both are real
  %s
    2026-08-03  Chequing (CAD)  Hydro One           -142.17
    2026-08-05  Chequing (CAD)  HYDRO ONE NETWORKS  -142.17
  %s  ignored
    2026-08-20  Chequing (CAD)  Rogers           -55.00
    2026-08-21  Chequing (CAD)  ROGERS WIRELESS  -55.00
  %s  fixed 2026-10-01

3 findings: 1 open, 1 ignored, 1 fixed
`, ids.open, ids.ignored, ids.fixed), stdout.String())
}

func Test_run_findings_status_fixed_lists_only_the_fixed_finding_as_a_date_line(t *testing.T) {
	home := newHome(t)
	inZone(t, time.UTC)
	ids := syncFiltersStore(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--status", "fixed"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf("Possible duplicates (1)\n  %s  fixed 2026-10-02\n\n1 fixed finding\n", ids.fixed), stdout.String())
}

func Test_run_findings_status_ignored_lists_only_the_ignored_finding_without_a_marker_or_hint(t *testing.T) {
	home := newHome(t)
	ids := syncFiltersStore(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--status", "ignored"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Possible duplicates (1)
  %s
    2026-08-20  Chequing (CAD)  Rogers           -55.00
    2026-08-21  Chequing (CAD)  ROGERS WIRELESS  -55.00

1 ignored finding
`, ids.ignored), stdout.String())
}

func Test_run_findings_type_duplicate_warns_about_an_unmatched_id_but_not_one_the_type_filters_out(t *testing.T) {
	home := newHome(t)
	sync := syncFiltersBundle(t, home, "DocumentsA", true)
	const unmatched = "duplicate:txn-9998+txn-9999"
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%s]\n", quotedList([]string{sync.uncategorized, unmatched})))

	exitCode, _, stderr := runCapture(context.Background(), []string{"findings", "--type", "duplicate"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+configShown+": findings.ignore lists \""+unmatched+
		"\", which is not a finding in quarry's store; quarry skips it\n", stderr.String())
}

func Test_run_findings_json_status_all_type_duplicate_prints_each_finding_with_its_status_and_fixed_at(t *testing.T) {
	home := newHome(t)
	ids := syncFiltersStore(t, home)
	pinFirstFoundAt(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json", "--status", "all", "--type", "duplicate"})

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Status   string          `json:"status"`
		Type     *string         `json:"type"`
		Counts   json.RawMessage `json:"counts"`
		Findings []struct {
			ID      string  `json:"id"`
			Status  string  `json:"status"`
			FixedAt *string `json:"fixed_at"`
			Items   []any   `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, "all", doc.Status)
	assert.Equal(t, "duplicate", *doc.Type)
	assert.JSONEq(t, `{"open": 1, "ignored": 1, "fixed": 1, "new": 1, "newly_fixed": 0}`, string(doc.Counts))
	require.Len(t, doc.Findings, 3)
	fixedAt := "2026-10-02T03:00:00Z"
	assert.Equal(t, ids.open, doc.Findings[0].ID)
	assert.Equal(t, "open", doc.Findings[0].Status)
	assert.Nil(t, doc.Findings[0].FixedAt)
	assert.Len(t, doc.Findings[0].Items, 2)
	assert.Equal(t, ids.ignored, doc.Findings[1].ID)
	assert.Equal(t, "ignored", doc.Findings[1].Status)
	assert.Nil(t, doc.Findings[1].FixedAt)
	assert.Len(t, doc.Findings[1].Items, 2)
	assert.Equal(t, ids.fixed, doc.Findings[2].ID)
	assert.Equal(t, "fixed", doc.Findings[2].Status)
	assert.Equal(t, &fixedAt, doc.Findings[2].FixedAt)
	assert.Equal(t, []any{}, doc.Findings[2].Items)
}

// csvDuplicateIDs names the open Hydro One duplicate (finding id and its two transactions) and the Netflix one syncCSVDuplicates fixes.
type csvDuplicateIDs struct {
	open, openFirst, openSecond, fixed string
}

// syncCSVDuplicates syncs an open Hydro One duplicate pair into dir, plus a Netflix pair when
// withFixed; syncing again without the Netflix pair fixes that finding.
func syncCSVDuplicates(t *testing.T, home, dir string, withFixed bool) csvDuplicateIDs {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	netflixPK := b.Payee(v9fixture.PayeeRow{Name: "Netflix"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	day := func(month time.Month, d int) time.Time { return time.Date(2026, month, d, 0, 0, 0, 0, time.UTC) }
	first := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, day(time.August, 3), "-142.17")
	second := categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, day(time.August, 5), "-142.17")
	ids := csvDuplicateIDs{
		open:      fmt.Sprintf("duplicate:txn-%d+txn-%d", first, second),
		openFirst: fmt.Sprintf("txn-%d", first), openSecond: fmt.Sprintf("txn-%d", second),
	}
	if withFixed {
		fixedFirst := categorizedPayeeTxn(b, chequingPK, netflixPK, billsPK, day(time.July, 1), "-31.25")
		fixedSecond := categorizedPayeeTxn(b, chequingPK, netflixPK, billsPK, day(time.July, 2), "-31.25")
		ids.fixed = fmt.Sprintf("duplicate:txn-%d+txn-%d", fixedFirst, fixedSecond)
	}
	exitCode, _, stderr := syncNewBundle(t, home, dir, b)
	require.Equal(t, 0, exitCode, stderr)
	return ids
}

func Test_run_findings_status_all_csv_prints_a_row_per_duplicate_item_and_one_row_for_the_fixed_finding(t *testing.T) {
	home := newHome(t)
	ids := syncCSVDuplicates(t, home, "DocumentsA", true)
	syncCSVDuplicates(t, home, "DocumentsB", false)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--status", "all", "--csv"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const fix = `"Delete the extra one in Quicken, or ignore the pair if both are real"`
	assert.Equal(t, "finding_id,type,status,date,account,currency,payee,category,amount,other_account,transactions,splits,transaction_id,split_id,payee_id,category_id,fix,"+
		"investment_transaction_id,security_id,security,shares\n"+
		ids.open+",duplicate,open,2026-08-03,Chequing,CAD,Hydro One,,-142.17,,,,"+ids.openFirst+",,,,"+fix+",,,,\n"+
		ids.open+",duplicate,open,2026-08-05,Chequing,CAD,Hydro One,,-142.17,,,,"+ids.openSecond+",,,,"+fix+",,,,\n"+
		ids.fixed+",duplicate,fixed,,,,,,,,,,,,,,"+fix+",,,,\n", stdout.String())
}

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
	addUncategorizedPayeeSplits(b, chequingPK, amazonPK)
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return ignoreFixtureIDs{
		duplicate:     fmt.Sprintf("duplicate:txn-%d+txn-%d", first, second),
		uncategorized: fmt.Sprintf("uncategorized:payee-%d", amazonPK),
	}
}

func Test_run_findings_leaves_an_ignored_id_off_the_list_and_counts_it_in_the_footer(t *testing.T) {
	home := newHome(t)
	ids := syncIgnoreFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n", ids.duplicate))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Uncategorized (1 payee, 2 splits): give each payee's splits a category in Quicken
  %s  Amazon  2 splits  2026-03-01 to 2026-03-02

1 open finding; 1 ignored not shown (--status all)
`, ids.uncategorized), stdout.String())
}

func Test_run_findings_warns_about_an_ignored_id_that_is_not_a_finding(t *testing.T) {
	home := newHome(t)
	syncIgnoreFixture(t, home)
	writeConfig(t, home, "[findings]\nignore = [\"uncategorized:payee-999\"]\n")

	exitCode, _, stderr := runCapture(context.Background(), []string{"findings"})

	require.Equal(t, 0, exitCode)
	assert.Equal(t, "quarry: warning: "+configShown+
		": findings.ignore lists \"uncategorized:payee-999\", which is not a finding in quarry's store; quarry skips it\n",
		stderr.String())
}

func Test_run_findings_json_lists_an_unmatched_ignore_id_after_the_config_warning(t *testing.T) {
	home := newHome(t)
	syncIgnoreFixture(t, home)
	writeConfig(t, home, "colour = \"red\"\n[findings]\nignore = [\"uncategorized:payee-999\"]\n")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	unknownKey := configShown + ": unknown key colour; quarry ignores it"
	unmatched := configShown + ": findings.ignore lists \"uncategorized:payee-999\", which is not a finding in quarry's store; quarry skips it"
	assert.Equal(t, "quarry: warning: "+unknownKey+"\nquarry: warning: "+unmatched+"\n", stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{
		strings.Replace(unknownKey, configShown, configPath(home), 1),
		strings.Replace(unmatched, configShown, configPath(home), 1),
	}, doc.Warnings)
}

func Test_run_findings_quotes_and_orders_every_unmatched_ignore_id_in_stderr_and_json_warnings(t *testing.T) {
	home := newHome(t)
	ids := syncIgnoreFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [\"\", \"a\\\"b\\\\c\\n\\u0001\", \"\", \"last:1\", %q]\n", ids.duplicate))
	lists := func(quoted string) string {
		return configShown + ": findings.ignore lists " + quoted + ", which is not a finding in quarry's store; quarry skips it"
	}
	want := []string{lists(`""`), lists(`"a\"b\\c\n\u0001"`), lists(`""`), lists(`"last:1"`)}
	wantJSON := make([]string, len(want))
	for i, line := range want {
		wantJSON[i] = strings.Replace(line, configShown, configPath(home), 1)
	}

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+strings.Join(want, "\nquarry: warning: ")+"\n", stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, wantJSON, doc.Warnings)
}

func Test_run_findings_shows_the_hint_when_findings_ignore_is_an_empty_list(t *testing.T) {
	home := newHome(t)
	syncIgnoreFixture(t, home)
	writeConfig(t, home, "[findings]\nignore = []\n")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "2 open findings\nIgnore a finding by adding its id to findings.ignore in "+configShown)
}

func Test_run_findings_hides_the_hint_when_findings_ignore_lists_only_unmatched_ids(t *testing.T) {
	home := newHome(t)
	syncIgnoreFixture(t, home)
	writeConfig(t, home, "[findings]\nignore = [\"uncategorized:payee-999\"]\n")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.True(t, strings.HasSuffix(stdout.String(), "\n2 open findings\n"), stdout.String())
}

func Test_run_findings_counts_an_ignored_finding_that_is_fixed_as_fixed(t *testing.T) {
	home := newHome(t)
	ids := syncIgnoreFixture(t, home)
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	hydroPK := b.Payee(v9fixture.PayeeRow{Name: "Hydro One"})
	b.Payee(v9fixture.PayeeRow{Name: "HYDRO ONE NETWORKS"})
	amazonPK := b.Payee(v9fixture.PayeeRow{Name: "Amazon"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	categorizedPayeeTxn(b, chequingPK, hydroPK, billsPK, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "-142.17")
	addUncategorizedPayeeSplits(b, chequingPK, amazonPK)
	exitCode, _, syncErr := syncNewBundle(t, home, "DocumentsB", b)
	require.Equal(t, 0, exitCode, syncErr)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n", ids.duplicate))
	var stdout, stderr bytes.Buffer

	exitCode = run(context.Background(), []string{"findings"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.True(t, strings.HasSuffix(stdout.String(), "\n1 open finding; 1 fixed not shown (--status all)\n"), stdout.String())
}

func Test_run_sync_refuses_a_findings_ignore_that_is_not_a_list_before_taking_a_snapshot(t *testing.T) {
	home := newHome(t)
	writeStatusFixtureBundle(t, home)
	quarryDir := storeDirUnder(home)
	var goodStdout, goodStderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync"}, &goodStdout, &goodStderr), goodStderr.String())
	storeBefore := fileDigest(t, filepath.Join(quarryDir, "quarry.duckdb"))
	writeConfig(t, home, "findings.ignore = \"duplicate:txn-1+txn-2\"\n")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync"})

	assert.Equal(t, 1, countFilesWithSuffix(t, filepath.Join(quarryDir, "snapshots"), ".sqlite"))
	assert.Equal(t, storeBefore, fileDigest(t, filepath.Join(quarryDir, "quarry.duckdb")))
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: "+configShown+": findings.ignore must be a list of finding ids in quotes, "+
		"such as [\"duplicate:txn-4410+txn-4412\"], got \"duplicate:txn-1+txn-2\""+configFix+"\n", stderr.String())
	assert.Equal(t, 1, exitCode)
}

const (
	findingsRepeatWarning = "cannot carry findings forward from the previous store " +
		"(its findings table repeats an id); findings history starts again with this sync"
	importRunsTooLargeWarning = "cannot carry import history forward from the previous store " +
		"(its import_runs table has an id too large to follow); import_runs starts again with this sync"
	importRunsIDTooLarge = "UPDATE import_runs SET id = 9223372036854775807"
)

// findingsTableRepeating replaces the findings table with one that has no
// PRIMARY KEY and holds id twice.
func findingsTableRepeating(id string) string {
	return fmt.Sprintf(`DROP TABLE findings;
CREATE TABLE findings (id VARCHAR, type VARCHAR NOT NULL, first_found_at TIMESTAMP NOT NULL, fixed_at TIMESTAMP);
INSERT INTO findings VALUES
	('%[1]s', 'uncategorized', TIMESTAMP '2026-03-01 00:00:00', NULL),
	('%[1]s', 'uncategorized', TIMESTAMP '2026-03-01 00:00:00', NULL)`, id)
}

// syncNewBundle writes b under home/dir and runs sync --quicken on it with extra args.
func syncNewBundle(t *testing.T, home, dir string, b *v9fixture.Builder, extra ...string) (int, string, string) {
	t.Helper()
	bundle := b.WriteBundle(t, filepath.Join(home, dir))
	exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"sync", "--quicken", bundle.Dir}, extra...))
	return exitCode, stdout.String(), stderr.String()
}

// syncWithFaultedFindings syncs an open uncategorized finding for one payee, damages the store with
// stmt built from that finding's id, then syncs a bundle whose only open finding is the other payee's.
func syncWithFaultedFindings(t *testing.T, home string, stmt func(findingID string) string, extra ...string) (int, string, string) {
	t.Helper()
	first, xPK := twoPayeeBundle(false, true)
	second, _ := twoPayeeBundle(true, false)
	syncFindingsBundleIn(t, home, "DocumentsA", first)
	editStore(t, home, stmt(fmt.Sprintf("uncategorized:payee-%d", xPK)))
	return syncNewBundle(t, home, "DocumentsB", second, extra...)
}

func Test_run_sync_from_warns_and_restarts_findings_history_when_the_previous_findings_table_repeats_an_id(t *testing.T) {
	home := newHome(t)
	id, _ := syncThenWrite(t, home)
	editStore(t, home, findingsTableRepeating("uncategorized:payee-1"))

	exitCode, _, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: "+findingsRepeatWarning+"\n", stderr)
}

func Test_run_sync_after_a_findings_fault_prints_the_findings_line_without_new_or_fixed_clauses(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := syncWithFaultedFindings(t, home, findingsTableRepeating)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Findings  1 open; run quarry findings to list them", findingsLine(t, stdout))
}

func Test_run_sync_json_after_a_findings_fault_lists_the_findings_warning_and_counts_every_open_finding_as_new(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := syncWithFaultedFindings(t, home, findingsTableRepeating, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, []string{findingsRepeatWarning}, decodeSyncDoc(t, stdout).Warnings)
	assert.JSONEq(t, `{"open":1,"ignored":0,"fixed":0,"new":1,"newly_fixed":0}`, syncFindingsCounts(t, stdout))
}

func Test_run_sync_prints_the_import_history_line_then_the_findings_line_when_both_tables_are_faulty(t *testing.T) {
	home := newHome(t)

	exitCode, _, stderr := syncWithFaultedFindings(t, home, func(id string) string {
		return findingsTableRepeating(id) + ";" + importRunsIDTooLarge
	})

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: "+importRunsTooLargeWarning+"\nquarry: warning: "+findingsRepeatWarning+"\n", stderr)
}

func Test_run_sync_json_lists_the_import_history_warning_then_the_findings_warning_when_both_tables_are_faulty(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := syncWithFaultedFindings(t, home, func(id string) string {
		return findingsTableRepeating(id) + ";" + importRunsIDTooLarge
	}, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, []string{importRunsTooLargeWarning, findingsRepeatWarning}, decodeSyncDoc(t, stdout).Warnings)
}

// syncCostco syncs a Quicken file in which Costco is spent in Groceries, Household, Groceries, Household, Groceries
// and Auto:Fuel, and returns Costco's payee primary key.
func syncCostco(t *testing.T, home string) int64 {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	costcoPK := b.Payee(v9fixture.PayeeRow{Name: "Costco"})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	householdPK := b.Category(v9fixture.TagRow{Name: "Household", Type: new(int64(1))})
	autoPK := b.Category(v9fixture.TagRow{Name: "Auto", Type: new(int64(1))})
	fuelPK := b.Category(v9fixture.TagRow{Name: "Fuel", Type: new(int64(1)), ParentCategory: autoPK})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, category := range []int64{groceriesPK, householdPK, groceriesPK, householdPK, groceriesPK, fuelPK} {
		day = day.AddDate(0, 0, 10)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-10.00", PostedDate: &day, Payee: costcoPK})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-10.00", CategoryTag: category})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return costcoPK
}

func Test_run_findings_reports_a_mixed_categories_payee(t *testing.T) {
	home := newHome(t)
	costcoPK := syncCostco(t, home)

	t.Run("lists_the_payee_with_its_category_rows", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--type", "mixed-categories"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, fmt.Sprintf(`Payees in mixed categories (1): pick one category per payee in Quicken, or ignore a payee whose mix is intended
  mixed-categories:payee-%d  Costco  3 categories, 6 transactions
    Groceries  3 transactions
    Household  2 transactions
    Auto:Fuel   1 transaction

1 open finding
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, costcoPK, configShown), stdout.String())
	})

	t.Run("json_gives_the_item_its_payee_category_and_count", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json", "--type", "mixed-categories"})

		require.Equal(t, 0, exitCode, stderr.String())
		var doc struct {
			Findings []struct {
				Items []struct {
					Payee         string  `json:"payee"`
					Category      string  `json:"category"`
					Transactions  int     `json:"transactions"`
					PayeeID       string  `json:"payee_id"`
					TransactionID *string `json:"transaction_id"`
					Date          *string `json:"date"`
					Amount        *string `json:"amount"`
				} `json:"items"`
			} `json:"findings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
		require.Len(t, doc.Findings, 1)
		items := doc.Findings[0].Items
		require.Len(t, items, 3)
		assert.Equal(t, []string{"Groceries", "Household", "Auto:Fuel"}, []string{items[0].Category, items[1].Category, items[2].Category})
		assert.Equal(t, []int{3, 2, 1}, []int{items[0].Transactions, items[1].Transactions, items[2].Transactions})
		assert.Equal(t, []string{"Costco", fmt.Sprintf("payee-%d", costcoPK)}, []string{items[0].Payee, items[0].PayeeID})
		assert.Equal(t, []*string{nil, nil, nil}, []*string{items[0].TransactionID, items[0].Date, items[0].Amount})
	})
}

// syncTimHortons syncs a Quicken file in which "TIM HORTONS #1234" has two transactions and "Tim Hortons" one,
// and returns their payee primary keys.
func syncTimHortons(t *testing.T, home string) (int64, int64) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	numberedPK := b.Payee(v9fixture.PayeeRow{Name: "TIM HORTONS #1234"})
	plainPK := b.Payee(v9fixture.PayeeRow{Name: "Tim Hortons"})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, payee := range []int64{numberedPK, numberedPK, plainPK} {
		day = day.AddDate(0, 0, 10)
		amount := fmt.Sprintf("-%d.01", i+3)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day, Payee: payee})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: groceriesPK})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return numberedPK, plainPK
}

func Test_run_findings_reports_payee_variants(t *testing.T) {
	home := newHome(t)
	numberedPK, plainPK := syncTimHortons(t, home)

	t.Run("lists_a_row_per_payee", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--type", "payee-variants"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, fmt.Sprintf(`Payee variants (1 group): rename each group to one payee in Quicken and add a renaming rule
  payee-variants:tim-hortons  2 payees, 3 transactions
    TIM HORTONS #1234  2 transactions
    Tim Hortons         1 transaction

1 open finding
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, configShown), stdout.String())
	})

	t.Run("json_gives_the_item_its_payee_and_count_and_null_transaction_fields", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json", "--type", "payee-variants"})

		require.Equal(t, 0, exitCode, stderr.String())
		var doc struct {
			Findings []struct {
				ID    string `json:"id"`
				Items []struct {
					Payee         string  `json:"payee"`
					PayeeID       string  `json:"payee_id"`
					Transactions  int     `json:"transactions"`
					Category      *string `json:"category"`
					TransactionID *string `json:"transaction_id"`
					Date          *string `json:"date"`
					Amount        *string `json:"amount"`
				} `json:"items"`
			} `json:"findings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
		require.Len(t, doc.Findings, 1)
		assert.Equal(t, "payee-variants:tim-hortons", doc.Findings[0].ID)
		items := doc.Findings[0].Items
		require.Len(t, items, 2)
		assert.Equal(t, []string{"TIM HORTONS #1234", fmt.Sprintf("payee-%d", numberedPK), "Tim Hortons", fmt.Sprintf("payee-%d", plainPK)},
			[]string{items[0].Payee, items[0].PayeeID, items[1].Payee, items[1].PayeeID})
		assert.Equal(t, []int{2, 1}, []int{items[0].Transactions, items[1].Transactions})
		assert.Equal(t, []*string{nil, nil, nil, nil}, []*string{items[0].Category, items[0].TransactionID, items[0].Date, items[0].Amount})
	})
}

// syncSimilarCategories syncs a Quicken file in which expense "Groceries" has two splits and "Grocery" one, and the
// unused income "Gift" and "Gifts" differ only by a plural; it returns the expense categories' primary keys.
func syncSimilarCategories(t *testing.T, home string) (int64, int64) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	groceryPK := b.Category(v9fixture.TagRow{Name: "Grocery", Type: new(int64(1))})
	b.Category(v9fixture.TagRow{Name: "Gift", Type: new(int64(2))})
	b.Category(v9fixture.TagRow{Name: "Gifts", Type: new(int64(2))})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, category := range []int64{groceriesPK, groceriesPK, groceryPK} {
		day = day.AddDate(0, 0, 10)
		amount := fmt.Sprintf("-%d.01", i+3)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: category})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return groceriesPK, groceryPK
}

func Test_run_findings_reports_similar_categories(t *testing.T) {
	home := newHome(t)
	groceriesPK, groceryPK := syncSimilarCategories(t, home)

	t.Run("lists_a_row_per_category", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--type", "similar-categories"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, fmt.Sprintf(`Similar categories (2 groups): merge each group into one category in Quicken
  similar-categories:grocery      2 categories
    Groceries  2 splits
    Grocery     1 split
  similar-categories:income:gift  2 categories
    Gift   0 splits
    Gifts  0 splits

2 open findings
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, configShown), stdout.String())
	})

	t.Run("json_gives_the_item_its_category_and_splits_and_null_transaction_fields", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json", "--type", "similar-categories"})

		require.Equal(t, 0, exitCode, stderr.String())
		var doc struct {
			Findings []struct {
				ID    string `json:"id"`
				Items []struct {
					Category      string  `json:"category"`
					CategoryID    string  `json:"category_id"`
					Splits        int     `json:"splits"`
					Transactions  *int    `json:"transactions"`
					Payee         *string `json:"payee"`
					TransactionID *string `json:"transaction_id"`
					Date          *string `json:"date"`
					Amount        *string `json:"amount"`
				} `json:"items"`
			} `json:"findings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
		require.Len(t, doc.Findings, 2)
		assert.Equal(t, []string{"similar-categories:grocery", "similar-categories:income:gift"}, []string{doc.Findings[0].ID, doc.Findings[1].ID})
		items := doc.Findings[0].Items
		require.Len(t, items, 2)
		assert.Equal(t, []string{"Groceries", fmt.Sprintf("cat-%d", groceriesPK), "Grocery", fmt.Sprintf("cat-%d", groceryPK)},
			[]string{items[0].Category, items[0].CategoryID, items[1].Category, items[1].CategoryID})
		assert.Equal(t, []int{2, 1, 0}, []int{items[0].Splits, items[1].Splits, doc.Findings[1].Items[0].Splits})
		assert.Equal(t, []any{(*int)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil)},
			[]any{items[0].Transactions, items[0].Payee, items[0].TransactionID, items[0].Date, items[0].Amount})
	})
}

// unlinkedPairs is the txn primary keys of two unlinked pairs: -500.00 in Chequing categorized Bills with +500.00 in
// Visa split between Bills and Household, and on later dates -75.00 and +75.00 with no category and no payee.
type unlinkedPairs struct {
	outBig, inBig, outSmall, inSmall int64
}

// syncUnlinkedPairs syncs a Quicken file holding the two pairs into the store under home.
func syncUnlinkedPairs(t *testing.T, home string) unlinkedPairs {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CREDITCARD", Currency: "CAD", Active: true})
	visaPaymentPK := b.Payee(v9fixture.PayeeRow{Name: "Visa payment"})
	thankYouPK := b.Payee(v9fixture.PayeeRow{Name: "Payment thank you"})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	householdPK := b.Category(v9fixture.TagRow{Name: "Household", Type: new(int64(1))})
	var p unlinkedPairs
	p.outBig = categorizedPayeeTxn(b, chequingPK, visaPaymentPK, billsPK, time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), "-500.00")
	bigDay := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	p.inBig = b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "500.00", PostedDate: &bigDay, Payee: thankYouPK})
	b.Entry(v9fixture.EntryRow{Parent: p.inBig, Amount: "300.00", CategoryTag: billsPK})
	b.Entry(v9fixture.EntryRow{Parent: p.inBig, Amount: "200.00", CategoryTag: householdPK})
	outDay, inDay := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	p.outSmall = b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-75.00", PostedDate: &outDay})
	b.Entry(v9fixture.EntryRow{Parent: p.outSmall, Amount: "-75.00"})
	p.inSmall = b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "75.00", PostedDate: &inDay})
	b.Entry(v9fixture.EntryRow{Parent: p.inSmall, Amount: "75.00"})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return p
}

func Test_run_findings_reports_unlinked_transfer_pairs(t *testing.T) {
	home := newHome(t)
	p := syncUnlinkedPairs(t, home)

	t.Run("lists_a_pair_with_its_category_cells", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--type", "unlinked-transfer"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, fmt.Sprintf(`Unlinked transfers (2): make each pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts
  unlinked-transfer:txn-%[3]d+txn-%[4]d
    2026-07-10  Chequing (CAD)  (no payee)  -75.00  (uncategorized)
    2026-07-11  Visa (CAD)      (no payee)   75.00  (uncategorized)
  unlinked-transfer:txn-%[1]d+txn-%[2]d
    2026-07-02  Chequing (CAD)  Visa payment       -500.00  Bills
    2026-07-03  Visa (CAD)      Payment thank you   500.00  (split)

2 open findings
Ignore a finding by adding its id to findings.ignore in %[5]s; see quarry findings --help
`, p.outBig, p.inBig, p.outSmall, p.inSmall, configShown), stdout.String())
	})

	t.Run("json_gives_the_item_its_category_path_or_null", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json", "--type", "unlinked-transfer"})

		require.Equal(t, 0, exitCode, stderr.String())
		var doc struct {
			Findings []struct {
				ID    string `json:"id"`
				Items []struct {
					TransactionID string  `json:"transaction_id"`
					Category      *string `json:"category"`
					CategoryID    *string `json:"category_id"`
				} `json:"items"`
			} `json:"findings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
		require.Len(t, doc.Findings, 2)
		got := map[string]*string{}
		for _, f := range doc.Findings {
			for _, item := range f.Items {
				got[item.TransactionID] = item.Category
				assert.Nil(t, item.CategoryID)
			}
		}
		assert.Equal(t, map[string]*string{
			fmt.Sprintf("txn-%d", p.outBig): new("Bills"), fmt.Sprintf("txn-%d", p.inBig): nil,
			fmt.Sprintf("txn-%d", p.outSmall): nil, fmt.Sprintf("txn-%d", p.inSmall): nil,
		}, got)
	})
}

// syncUnusedCategories syncs a Quicken file with a used "Food" and the unused expense "Parking" and
// "Vacation", which has the unused subcategories "Hotel" and "Flights"; it returns their primary keys in that order.
func syncUnusedCategories(t *testing.T, home string) (int64, int64, int64) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	parkingPK := b.Category(v9fixture.TagRow{Name: "Parking", Type: new(int64(1))})
	vacationPK := b.Category(v9fixture.TagRow{Name: "Vacation", Type: new(int64(1))})
	hotelPK := b.Category(v9fixture.TagRow{Name: "Hotel", Type: new(int64(1)), ParentCategory: vacationPK})
	b.Category(v9fixture.TagRow{Name: "Flights", Type: new(int64(1)), ParentCategory: vacationPK})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-3.01", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-3.01", CategoryTag: foodPK})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return parkingPK, vacationPK, hotelPK
}

func Test_run_findings_reports_unused_categories(t *testing.T) {
	home := newHome(t)
	parkingPK, vacationPK, hotelPK := syncUnusedCategories(t, home)

	t.Run("lists_them_with_their_subcategory_count", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, fmt.Sprintf(`Unused categories (2): no transaction uses them; check that no scheduled transaction or budget does, then delete them in Quicken
  unused-category:cat-%d  Parking
  unused-category:cat-%d  Vacation (and 2 subcategories)

2 open findings
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, parkingPK, vacationPK, configShown), stdout.String())
	})

	t.Run("json_gives_the_item_its_category_and_null_everything_else", func(t *testing.T) {
		exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json", "--type", "unused-category"})

		require.Equal(t, 0, exitCode, stderr.String())
		var doc struct {
			Findings []struct {
				ID    string `json:"id"`
				Items []struct {
					Category      string  `json:"category"`
					CategoryID    string  `json:"category_id"`
					Splits        *int    `json:"splits"`
					Transactions  *int    `json:"transactions"`
					Payee         *string `json:"payee"`
					TransactionID *string `json:"transaction_id"`
					Date          *string `json:"date"`
					Amount        *string `json:"amount"`
				} `json:"items"`
			} `json:"findings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
		require.Len(t, doc.Findings, 2)
		vacation := doc.Findings[1]
		assert.Equal(t, fmt.Sprintf("unused-category:cat-%d", vacationPK), vacation.ID)
		require.Len(t, vacation.Items, 3)
		assert.Equal(t, []string{"Vacation", fmt.Sprintf("cat-%d", vacationPK), "Vacation:Hotel", fmt.Sprintf("cat-%d", hotelPK)},
			[]string{vacation.Items[0].Category, vacation.Items[0].CategoryID, vacation.Items[2].Category, vacation.Items[2].CategoryID})
		assert.Equal(t, []any{(*int)(nil), (*int)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil)},
			[]any{vacation.Items[0].Splits, vacation.Items[0].Transactions, vacation.Items[0].Payee, vacation.Items[0].TransactionID, vacation.Items[0].Date, vacation.Items[0].Amount})
	})
}

func Test_run_findings_lists_an_unclassified_account_until_the_config_classifies_it(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	accountID := fmt.Sprintf("acct-%d", brokeragePK)
	var before, after bytes.Buffer
	var stderr bytes.Buffer

	require.Equal(t, 0, run(context.Background(), []string{"findings"}, &before, &stderr), stderr.String())
	writeConfig(t, home, fmt.Sprintf("[accounts]\nregistered = [%q]\n", accountID))
	require.Equal(t, 0, run(context.Background(), []string{"findings"}, &after, &stderr), stderr.String())

	assert.Equal(t, fmt.Sprintf(`Unclassified investment accounts (1): list each account's id (acct-…) in accounts.registered or accounts.non-registered in %s; see quarry findings --help
  unclassified-account:%s  Questrade TFSA  brokerage, CAD

1 open finding
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, configShown, accountID, configShown), before.String())
	assert.NotContains(t, after.String(), "unclassified-account")
	assert.Contains(t, after.String(), "No open findings")
	assert.Empty(t, stderr.String())
}

func Test_run_findings_json_reports_an_unclassified_account_without_found_or_fixed_times(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json", "--type", "unclassified-account"})

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Findings []struct {
			Status       string  `json:"status"`
			FirstFoundAt *string `json:"first_found_at"`
			FixedAt      *string `json:"fixed_at"`
			Items        []struct {
				AccountID string `json:"account_id"`
				Account   string `json:"account"`
				Currency  string `json:"currency"`
			} `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Findings, 1)
	entry := doc.Findings[0]
	assert.Equal(t, "open", entry.Status)
	assert.Nil(t, entry.FirstFoundAt)
	assert.Nil(t, entry.FixedAt)
	require.Len(t, entry.Items, 1)
	assert.Equal(t, fmt.Sprintf("acct-%d", brokeragePK), entry.Items[0].AccountID)
	assert.Equal(t, "Questrade TFSA", entry.Items[0].Account)
	assert.Equal(t, "CAD", entry.Items[0].Currency)
	assert.Empty(t, stderr.String())
}

func Test_run_findings_leaves_investment_cash_rows_out_of_duplicate_and_unlinked_transfer(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CREDITCARD", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	incomePK := b.Category(v9fixture.TagRow{Name: "Dividends", Type: new(int64(categoryKindIncome))})
	billsPK := b.Category(v9fixture.TagRow{Name: "Bills", Type: new(int64(1))})
	day := func(d int) time.Time { return time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC) }

	invest := func(code int64, posted time.Time, row v9fixture.TransactionRow) {
		row.Account = brokeragePK
		row.PostedDate = &posted
		row.Type = &code
		pk := b.InvestmentTransaction(row)
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount, CategoryTag: incomePK})
	}
	register := func(account int64, posted time.Time, amount string) int64 {
		pk := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &posted})
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: amount, CategoryTag: billsPK})
		return pk
	}
	invest(investmentCodeDividend, day(1), v9fixture.TransactionRow{Amount: "25.00"})
	invest(investmentCodeDividend, day(2), v9fixture.TransactionRow{Amount: "25.00"})
	invest(investmentCodeSell, day(10), v9fixture.TransactionRow{Position: positionPK, Units: "0", Amount: "500.00"})
	register(chequingPK, day(11), "-500.00")
	controlDuplicateFirst := register(chequingPK, day(15), "-40.00")
	controlDuplicateSecond := register(chequingPK, day(16), "-40.00")
	controlOut := register(savingsPK, day(20), "-75.00")
	controlIn := register(visaPK, day(21), "75.00")
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))

	findingIDs := func(typ string) []string {
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, run(context.Background(), []string{"findings", "--json", "--type", typ}, &stdout, &stderr), stderr.String())
		var doc struct {
			Findings []struct {
				ID string `json:"id"`
			} `json:"findings"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
		ids := make([]string, 0, len(doc.Findings))
		for _, f := range doc.Findings {
			ids = append(ids, f.ID)
		}
		return ids
	}

	assert.Equal(t, []string{fmt.Sprintf("duplicate:txn-%d+txn-%d", controlDuplicateFirst, controlDuplicateSecond)}, findingIDs("duplicate"))
	assert.Equal(t, []string{fmt.Sprintf("unlinked-transfer:txn-%d+txn-%d", controlOut, controlIn)}, findingIDs("unlinked-transfer"))
}

const sharesWithoutCostClause = "enter what each one cost on its Add Shares transaction in Quicken, " +
	"then run quarry sync; until then quarry acb counts those shares at no cost"

func Test_run_findings_lists_shares_added_with_no_cost_as_status_sync_and_mcp_count_them(t *testing.T) {
	const addShares = int64(2)
	var syncLine, emptyCostID, zeroCostID string
	ctx, peer := newStatusPeer(t, func(t *testing.T, home string) {
		t.Helper()
		b := v9fixture.NewBuilder()
		marginPK := b.Account(v9fixture.AccountRow{Name: "Questrade Margin", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
		tfsaPK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
		xeqtPK := b.Security(v9fixture.SecurityRow{Name: "XEQT", Ticker: "XEQT", Currency: "CAD"})
		marginPosition := b.Position(v9fixture.PositionRow{Account: marginPK, Security: xeqtPK})
		tfsaPosition := b.Position(v9fixture.PositionRow{Account: tfsaPK, Security: xeqtPK})
		add := func(account, position int64, day time.Time, row v9fixture.TransactionRow) string {
			row.Account, row.Position, row.PostedDate, row.Type, row.Amount = account, position, &day, new(addShares), "0"
			pk := b.InvestmentTransaction(row)
			b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "0"})
			return fmt.Sprintf("itxn-%d", pk)
		}
		emptyCostID = add(marginPK, marginPosition, time.Date(2016, 3, 1, 0, 0, 0, 0, time.UTC), v9fixture.TransactionRow{Units: "100"})
		zeroCostID = add(marginPK, marginPosition, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), v9fixture.TransactionRow{Units: "1", CostBasis: "0"})
		add(marginPK, marginPosition, time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC), v9fixture.TransactionRow{Units: "5", CostBasis: "300"})
		add(tfsaPK, tfsaPosition, time.Date(2021, 5, 1, 0, 0, 0, 0, time.UTC), v9fixture.TransactionRow{Units: "7"})
		b.Lot(v9fixture.LotRow{Position: marginPosition, LatestUnits: "106"})
		b.Lot(v9fixture.LotRow{Position: tfsaPosition, LatestUnits: "7"})
		writeConfig(t, home, fmt.Sprintf("[accounts]\nnon-registered = [\"acct-%d\"]\nregistered = [\"acct-%d\"]\n", marginPK, tfsaPK))
		syncLine = syncFindingsBundleIn(t, home, "Documents", b)
	})
	var findingsOut, statusOut, typeOut, stderr bytes.Buffer

	require.Equal(t, 0, run(ctx, []string{"findings"}, &findingsOut, &stderr), stderr.String())
	require.Equal(t, 0, run(ctx, []string{"status"}, &statusOut, &stderr), stderr.String())
	typeExit := run(ctx, []string{"findings", "--type", "shares-without-cost"}, &typeOut, &stderr)
	syncStatus := readSyncStatus(ctx, t, peer)
	result := callDataQuality(ctx, t, peer, map[string]any{})
	require.False(t, result.IsError, textOf(result))
	var listing dataQualityDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &listing))

	assert.Equal(t, fmt.Sprintf(`Shares added with no cost (2): %s
  shares-without-cost:%s  2026-02-01  Questrade Margin  XEQT  1 share
  shares-without-cost:%s  2016-03-01  Questrade Margin  XEQT  100 shares

2 open findings
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, sharesWithoutCostClause, zeroCostID, emptyCostID, configShown), findingsOut.String())
	assert.Equal(t, "Findings  2 open; run quarry findings to list them", syncLine)
	assert.Contains(t, statusOut.String(), "\nFindings  2 open; run quarry findings to list them\n")
	assert.Equal(t, 2, syncStatus.Findings.Open)
	assert.InDelta(t, 2, listing.Counts["open"], 0)
	assert.Equal(t, 0, typeExit, stderr.String())
	assert.Empty(t, stderr.String())
}

func Test_run_status_json_counts_an_ignored_shares_without_cost_as_ignored(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	marginPK := b.Account(v9fixture.AccountRow{Name: "Questrade Margin", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	xeqtPK := b.Security(v9fixture.SecurityRow{Name: "XEQT", Ticker: "XEQT", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: marginPK, Security: xeqtPK})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	addPK := b.InvestmentTransaction(v9fixture.TransactionRow{Account: marginPK, Position: positionPK, PostedDate: &day, Type: new(int64(2)), Amount: "0", Units: "5"})
	b.Entry(v9fixture.EntryRow{Parent: addPK, Amount: "0"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "5"})
	writeConfig(t, home, fmt.Sprintf("[accounts]\nnon-registered = [\"acct-%d\"]\n[findings]\nignore = [\"shares-without-cost:itxn-%d\"]\n", marginPK, addPK))
	syncFindingsBundleIn(t, home, "Documents", b)

	got := statusFindings(t)

	require.NotNil(t, got.Findings.Ignored)
	assert.Equal(t, []int{0, 1}, []int{got.Findings.Open, *got.Findings.Ignored})
}

func Test_run_status_sync_and_mcp_agree_on_an_unclassified_account_count(t *testing.T) {
	var syncLine string
	ctx, peer := newStatusPeer(t, func(t *testing.T, home string) {
		t.Helper()
		b := v9fixture.NewBuilder()
		listedPK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
		b.Account(v9fixture.AccountRow{Name: "Questrade Margin", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
		b.Account(v9fixture.AccountRow{Name: "Old RRSP", Type: "RETIREMENTIRA", Currency: "CAD", Active: true})
		writeConfig(t, home, fmt.Sprintf("[accounts]\nregistered = [\"acct-%d\"]\n", listedPK))
		syncLine = syncFindingsBundleIn(t, home, "Documents", b)
	})
	var statusOut, stderr bytes.Buffer
	require.Equal(t, 0, run(ctx, []string{"status"}, &statusOut, &stderr), stderr.String())

	syncStatus := readSyncStatus(ctx, t, peer)
	result := callDataQuality(ctx, t, peer, map[string]any{})
	require.False(t, result.IsError, textOf(result))
	var listing dataQualityDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &listing))

	assert.Equal(t, "Findings  2 open; run quarry findings to list them", syncLine)
	assert.Contains(t, statusOut.String(), "\nFindings  2 open; run quarry findings to list them\n")
	assert.Equal(t, 2, syncStatus.Findings.Open)
	assert.InDelta(t, 2, listing.Counts["open"], 0)
}
