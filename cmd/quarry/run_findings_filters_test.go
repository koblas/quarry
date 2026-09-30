// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	for i, amount := range []string{"-10.00", "-20.00"} {
		posted := day(time.March, 1+i)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &posted, Payee: amazonPK})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
	}
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	inZone(t, time.FixedZone("UTC-5", -5*60*60))
	ids := syncFiltersStore(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--status", "all", "--type", "duplicate"}, &stdout, &stderr)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
	inZone(t, time.UTC)
	ids := syncFiltersStore(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--status", "fixed"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf("Possible duplicates (1)\n  %s  fixed 2026-10-02\n\n1 fixed finding\n", ids.fixed), stdout.String())
}

func Test_run_findings_status_ignored_lists_only_the_ignored_finding_without_a_marker_or_hint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ids := syncFiltersStore(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--status", "ignored"}, &stdout, &stderr)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
	sync := syncFiltersBundle(t, home, "DocumentsA", true)
	const unmatched = "duplicate:txn-9998+txn-9999"
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%s]\n", quotedList([]string{sync.uncategorized, unmatched})))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--type", "duplicate"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+configShown+": findings.ignore lists \""+unmatched+
		"\", which is not a finding in quarry's store; quarry skips it\n", stderr.String())
}

func Test_run_findings_json_status_all_type_duplicate_prints_each_finding_with_its_status_and_fixed_at(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ids := syncFiltersStore(t, home)
	pinFirstFoundAt(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--json", "--status", "all", "--type", "duplicate"}, &stdout, &stderr)

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
