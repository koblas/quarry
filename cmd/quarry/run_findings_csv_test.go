// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
	ids := syncCSVDuplicates(t, home, "DocumentsA", true)
	syncCSVDuplicates(t, home, "DocumentsB", false)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--status", "all", "--csv"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const fix = `"Delete the extra one in Quicken, or ignore the pair if both are real"`
	assert.Equal(t, "finding_id,type,status,date,account,currency,payee,category,amount,other_account,transactions,splits,transaction_id,split_id,payee_id,category_id,fix\n"+
		ids.open+",duplicate,open,2026-08-03,Chequing,CAD,Hydro One,,-142.17,,,,"+ids.openFirst+",,,,"+fix+"\n"+
		ids.open+",duplicate,open,2026-08-05,Chequing,CAD,Hydro One,,-142.17,,,,"+ids.openSecond+",,,,"+fix+"\n"+
		ids.fixed+",duplicate,fixed,,,,,,,,,,,,,,"+fix+"\n", stdout.String())
}
