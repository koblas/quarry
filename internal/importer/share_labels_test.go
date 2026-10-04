package importer_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// holdingOf is the share check's raw answer for one holding: ids only, as the store reports it.
func holdingOf(acctPK, secPK int64) store.ShareMismatch {
	return store.ShareMismatch{AccountID: fmt.Sprintf("acct-%d", acctPK), SecurityID: fmt.Sprintf("sec-%d", secPK), Quarry: 1, Quicken: 0}
}

func importShareMismatches(t *testing.T, b *v9fixture.Builder, mismatched ...store.ShareMismatch) []store.ShareMismatch {
	t.Helper()
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{shareCheck: store.ShareCheck{Checked: len(mismatched), Mismatched: mismatched}}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, store.ErrValidationFailed)
	return result.Validation.Shares.Mismatched
}

func holdingIDs(mismatched []store.ShareMismatch) []string {
	ids := make([]string, len(mismatched))
	for i, m := range mismatched {
		ids[i] = m.AccountID + "/" + m.SecurityID
	}
	return ids
}

// fillerAccounts adds n accounts so the next account's source id is n+1.
func fillerAccounts(b *v9fixture.Builder, n int) {
	for i := range n {
		b.Account(v9fixture.AccountRow{Name: fmt.Sprintf("Filler %d", i), Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	}
}

// fillerSecurities adds n securities so the next security's source id is n+1.
func fillerSecurities(b *v9fixture.Builder, n int) {
	for i := range n {
		b.Security(v9fixture.SecurityRow{Name: fmt.Sprintf("Filler %d", i)})
	}
}

func Test_import_labels_a_mismatched_holding_with_its_closed_inactive_account_and_security(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	fillerAccounts(b, 2)
	acctPK := b.Account(v9fixture.AccountRow{Name: "RRSP", Type: "BROKERAGENORMAL", Currency: "CAD", Closed: true})
	fillerSecurities(b, 3)
	secPK := b.Security(v9fixture.SecurityRow{Name: "iShares Core Equity ETF", Ticker: "XEQT"})
	holding := holdingOf(acctPK, secPK)
	holding.Quarry, holding.Quicken = 120500000, 110500000

	got := importShareMismatches(t, b, holding)

	assert.Equal(t, []store.ShareMismatch{{
		AccountID: "acct-3", SecurityID: "sec-4", Quarry: 120500000, Quicken: 110500000, Difference: 10000000,
		Account: "RRSP", Currency: "CAD", Closed: true, Active: false, AccountSourceID: 3,
		Security: "iShares Core Equity ETF", Ticker: new("XEQT"), SecuritySourceID: 4,
	}}, got)
}

func Test_import_labels_a_mismatched_holding_in_an_open_active_account_with_a_security_that_has_no_ticker(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "USD", Active: true})
	secPK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund"})

	got := importShareMismatches(t, b, holdingOf(acctPK, secPK))

	require.Len(t, got, 1)
	assert.False(t, got[0].Closed)
	assert.True(t, got[0].Active)
	assert.Equal(t, "USD", got[0].Currency)
	assert.Nil(t, got[0].Ticker)
}

func Test_import_leaves_labels_empty_when_a_mismatched_holding_resolves_to_no_row(t *testing.T) {
	t.Parallel()

	got := importShareMismatches(t, v9fixture.NewBuilder(), holdingOf(7, 8))

	assert.Equal(t, []store.ShareMismatch{{AccountID: "acct-7", SecurityID: "sec-8", Quarry: 1, Difference: 1}}, got)
}

func Test_import_sorts_mismatched_holdings_by_account_name_before_account_source_id(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	zeta := b.Account(v9fixture.AccountRow{Name: "Zeta", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	alpha := b.Account(v9fixture.AccountRow{Name: "Alpha", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	sec := b.Security(v9fixture.SecurityRow{Name: "Fund"})

	got := importShareMismatches(t, b, holdingOf(zeta, sec), holdingOf(alpha, sec))

	assert.Equal(t, []string{"acct-2/sec-1", "acct-1/sec-1"}, holdingIDs(got))
}

func Test_import_sorts_mismatched_holdings_of_equally_named_accounts_by_numeric_account_source_id(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	fillerAccounts(b, 8)
	nine := b.Account(v9fixture.AccountRow{Name: "Same", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	ten := b.Account(v9fixture.AccountRow{Name: "Same", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	sec := b.Security(v9fixture.SecurityRow{Name: "Fund"})

	got := importShareMismatches(t, b, holdingOf(ten, sec), holdingOf(nine, sec))

	assert.Equal(t, []string{"acct-9/sec-1", "acct-10/sec-1"}, holdingIDs(got))
}

func Test_import_sorts_mismatched_holdings_of_one_account_by_security_name_before_security_source_id(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acct := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	zeta := b.Security(v9fixture.SecurityRow{Name: "Zeta Fund"})
	alpha := b.Security(v9fixture.SecurityRow{Name: "Alpha Fund"})

	got := importShareMismatches(t, b, holdingOf(acct, zeta), holdingOf(acct, alpha))

	assert.Equal(t, []string{"acct-1/sec-2", "acct-1/sec-1"}, holdingIDs(got))
}

func Test_import_sorts_mismatched_holdings_of_equally_named_securities_by_numeric_security_source_id(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acct := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	fillerSecurities(b, 8)
	nine := b.Security(v9fixture.SecurityRow{Name: "Same Fund"})
	ten := b.Security(v9fixture.SecurityRow{Name: "Same Fund"})

	got := importShareMismatches(t, b, holdingOf(acct, ten), holdingOf(acct, nine))

	assert.Equal(t, []string{"acct-1/sec-9", "acct-1/sec-10"}, holdingIDs(got))
}

func Test_import_sorts_mismatched_holdings_by_account_name_even_when_security_names_order_the_other_way(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	zetaAcct := b.Account(v9fixture.AccountRow{Name: "Zeta", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	alphaAcct := b.Account(v9fixture.AccountRow{Name: "Alpha", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	zetaSec := b.Security(v9fixture.SecurityRow{Name: "Zeta Fund"})
	alphaSec := b.Security(v9fixture.SecurityRow{Name: "Alpha Fund"})

	got := importShareMismatches(t, b, holdingOf(zetaAcct, alphaSec), holdingOf(alphaAcct, zetaSec))

	assert.Equal(t, []string{"acct-2/sec-1", "acct-1/sec-2"}, holdingIDs(got))
}

func Test_import_sorts_mismatched_holdings_by_account_source_id_even_when_security_names_order_the_other_way(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	fillerAccounts(b, 8)
	nine := b.Account(v9fixture.AccountRow{Name: "Same", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	ten := b.Account(v9fixture.AccountRow{Name: "Same", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	zetaSec := b.Security(v9fixture.SecurityRow{Name: "Zeta Fund"})
	alphaSec := b.Security(v9fixture.SecurityRow{Name: "Alpha Fund"})

	got := importShareMismatches(t, b, holdingOf(ten, alphaSec), holdingOf(nine, zetaSec))

	assert.Equal(t, []string{"acct-9/sec-1", "acct-10/sec-2"}, holdingIDs(got))
}

func Test_import_reports_a_mismatched_holdings_difference_as_quarry_minus_quicken_clamped_to_int64(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		quarry, quicken int64
		want            int64
	}{
		{name: "quarry above quicken is positive", quarry: 5, quicken: 3, want: 2},
		{name: "quarry below quicken is negative", quarry: 3, quicken: 5, want: -2},
		{name: "largest count less a negative one clamps instead of wrapping", quarry: math.MaxInt64, quicken: -1, want: math.MaxInt64},
		{name: "smallest count less a positive one clamps instead of wrapping", quarry: math.MinInt64, quicken: 1, want: math.MinInt64},
		{name: "a difference that lands exactly on the smallest count is kept", quarry: math.MinInt64 + 1, quicken: 1, want: math.MinInt64},
		{name: "a difference that lands exactly on the largest count is kept", quarry: math.MaxInt64 - 1, quicken: -1, want: math.MaxInt64},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			holding := holdingOf(7, 8)
			holding.Quarry, holding.Quicken = c.quarry, c.quicken

			got := importShareMismatches(t, v9fixture.NewBuilder(), holding)

			require.Len(t, got, 1)
			assert.Equal(t, c.want, got[0].Difference)
		})
	}
}
