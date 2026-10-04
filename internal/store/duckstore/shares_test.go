package duckstore_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acctOne  = "acct-1"
	acctTwo  = "acct-2"
	secAcme  = "sec-1"
	secBeta  = "sec-2"
	oneShare = 1_000_000
)

var (
	shareDay       = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	shareNext      = time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	errScratchBoom = errors.New("scratch boom")
)

func buy(account, security string, source int64, day time.Time, millionths int64) store.InvestmentTransaction {
	return store.InvestmentTransaction{
		ID: fmt.Sprintf("itxn-%d", source), SourceID: source, AccountID: account, SecurityID: &security,
		Date: day, Action: "buy", Shares: &millionths, Amount: 100, Currency: "CAD",
	}
}

func splitOf(account, security string, source int64, day time.Time, newShares, oldShares int64) store.InvestmentTransaction {
	txn := buy(account, security, source, day, 0)
	txn.Action, txn.Shares = "split", nil
	txn.SplitNewShares, txn.SplitOldShares = &newShares, &oldShares
	return txn
}

func quickenCount(account, security string, millionths int64) store.QuickenShare {
	return store.QuickenShare{AccountID: account, SecurityID: security, Millionths: big.NewInt(millionths)}
}

func checkShares(t *testing.T, txns []store.InvestmentTransaction, quicken []store.QuickenShare) store.ShareCheck {
	t.Helper()
	result, err := duckstore.New(t.TempDir()).CheckShares(t.Context(), store.Rows{InvestmentTransactions: txns, QuickenShares: quicken})
	require.NoError(t, err)
	return result
}

func Test_check_shares_walks_a_holding_in_date_order_not_source_id_order(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{
		buy(acctOne, secAcme, 2, shareDay, 12*oneShare),
		splitOf(acctOne, secAcme, 1, shareNext, 1, 12),
	}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, oneShare)})

	assert.Equal(t, store.ShareCheck{Checked: 1}, result)
}

func Test_check_shares_breaks_a_same_date_tie_by_numeric_source_id(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{
		splitOf(acctOne, secAcme, 10, shareDay, 1, 12),
		buy(acctOne, secAcme, 9, shareDay, 12*oneShare),
	}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, oneShare)})

	assert.Equal(t, store.ShareCheck{Checked: 1}, result)
}

func Test_check_shares_multiplies_the_running_count_by_the_split_ratio(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, shareDay, 120*oneShare),
		splitOf(acctOne, secAcme, 2, shareNext, 1, 12),
	}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, 10*oneShare)})

	assert.Equal(t, store.ShareCheck{Checked: 1}, result)
}

func Test_check_shares_adds_nothing_at_a_split_row_that_carries_shares(t *testing.T) {
	t.Parallel()
	split := splitOf(acctOne, secAcme, 2, shareNext, 1, 12)
	split.Shares = new(int64(5 * oneShare))
	txns := []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, 120*oneShare), split}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, 10*oneShare)})

	assert.Equal(t, store.ShareCheck{Checked: 1}, result)
}

func Test_check_shares_matches_within_one_millionth(t *testing.T) {
	t.Parallel()
	oneBought := []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, oneShare)}
	hundredSplitTwelveWays := []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, shareDay, 100*oneShare),
		splitOf(acctOne, secAcme, 2, shareNext, 1, 12),
	}
	cases := []struct {
		name           string
		txns           []store.InvestmentTransaction
		quicken        int64
		wantMismatches int
	}{
		{name: "equal", txns: oneBought, quicken: oneShare, wantMismatches: 0},
		{name: "exactly 0.000001 above", txns: oneBought, quicken: oneShare + 1, wantMismatches: 0},
		{name: "exactly 0.000001 below", txns: oneBought, quicken: oneShare - 1, wantMismatches: 0},
		{name: "0.000002 above", txns: oneBought, quicken: oneShare + 2, wantMismatches: 1},
		{name: "0.000002 below", txns: oneBought, quicken: oneShare - 2, wantMismatches: 1},
		{name: "split-produced 8.3333333.. against 8.333333", txns: hundredSplitTwelveWays, quicken: 8_333_333, wantMismatches: 0},
		{name: "split-produced 8.3333333.. against 8.333332", txns: hundredSplitTwelveWays, quicken: 8_333_332, wantMismatches: 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			result := checkShares(t, c.txns, []store.QuickenShare{quickenCount(acctOne, secAcme, c.quicken)})

			assert.Len(t, result.Mismatched, c.wantMismatches)
		})
	}
}

func Test_check_shares_counts_rows_of_every_date_including_future_and_before_2001(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, time.Date(1999, 12, 31, 0, 0, 0, 0, time.UTC), 3*oneShare),
		buy(acctOne, secAcme, 2, shareDay, 5*oneShare),
		buy(acctOne, secAcme, 3, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), 7*oneShare),
	}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, 15*oneShare)})

	assert.Equal(t, store.ShareCheck{Checked: 1}, result)
}

func Test_check_shares_counts_the_holdings_it_compares(t *testing.T) {
	t.Parallel()
	cashOnly := buy(acctOne, secAcme, 3, shareDay, 0)
	cashOnly.SecurityID, cashOnly.Shares = nil, nil
	nullShares := buy(acctOne, secAcme, 4, shareDay, 0)
	nullShares.Shares = nil
	cases := []struct {
		name    string
		txns    []store.InvestmentTransaction
		quicken []store.QuickenShare
		want    int
	}{
		{name: "no rows"},
		{name: "transactions and no lot", txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, oneShare)}, want: 1},
		{name: "lot and no transactions", quicken: []store.QuickenShare{quickenCount(acctOne, secAcme, oneShare)}, want: 1},
		{name: "a lot of zero units and no transactions", quicken: []store.QuickenShare{quickenCount(acctOne, secAcme, 0)}, want: 1},
		{name: "a row of zero shares", txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, 0)}, want: 1},
		{name: "a row of NULL shares", txns: []store.InvestmentTransaction{nullShares}, want: 1},
		{name: "a row with no security", txns: []store.InvestmentTransaction{cashOnly}},
		{
			name: "transactions and a lot of one holding", want: 1,
			txns:    []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, oneShare)},
			quicken: []store.QuickenShare{quickenCount(acctOne, secAcme, oneShare)},
		},
		{
			name: "one security in two accounts", want: 2,
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, oneShare), buy(acctTwo, secAcme, 2, shareDay, oneShare)},
		},
		{
			name: "two securities in one account", want: 2,
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, oneShare), buy(acctOne, secBeta, 2, shareDay, oneShare)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			result := checkShares(t, c.txns, c.quicken)

			assert.Equal(t, c.want, result.Checked)
		})
	}
}

func Test_check_shares_reports_a_holding_with_transactions_and_no_lot_against_zero(t *testing.T) {
	t.Parallel()

	result := checkShares(t, []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, 5*oneShare)}, nil)

	assert.Equal(t, []store.ShareMismatch{{AccountID: acctOne, SecurityID: secAcme, Quarry: 5 * oneShare, Quicken: 0}}, result.Mismatched)
}

func Test_check_shares_reports_a_holding_with_a_lot_and_no_transactions_against_zero(t *testing.T) {
	t.Parallel()

	result := checkShares(t, nil, []store.QuickenShare{quickenCount(acctOne, secAcme, 3*oneShare)})

	assert.Equal(t, []store.ShareMismatch{{AccountID: acctOne, SecurityID: secAcme, Quarry: 0, Quicken: 3 * oneShare}}, result.Mismatched)
}

func Test_check_shares_reports_only_the_holdings_that_differ_in_account_then_security_order(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{
		buy(acctTwo, secBeta, 1, shareDay, 4*oneShare),
		buy(acctOne, secBeta, 2, shareDay, 2*oneShare),
		buy(acctOne, secAcme, 3, shareDay, 9*oneShare),
		buy(acctTwo, secAcme, 4, shareDay, 6*oneShare),
	}
	quicken := []store.QuickenShare{
		quickenCount(acctOne, secAcme, 8*oneShare),
		quickenCount(acctOne, secBeta, 2*oneShare),
		quickenCount(acctTwo, secAcme, 6*oneShare),
		quickenCount(acctTwo, secBeta, 5*oneShare),
	}

	result := checkShares(t, txns, quicken)

	assert.Equal(t, 4, result.Checked)
	assert.Equal(t, []store.ShareMismatch{
		{AccountID: acctOne, SecurityID: secAcme, Quarry: 9 * oneShare, Quicken: 8 * oneShare},
		{AccountID: acctTwo, SecurityID: secBeta, Quarry: 4 * oneShare, Quicken: 5 * oneShare},
	}, result.Mismatched)
}

func Test_check_shares_rounds_a_derived_count_to_the_nearest_millionth_with_ties_to_even(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		shares       int64
		newSide, old int64
		want         int64
	}{
		{name: "1.5 millionths rounds to the even 2", shares: 3, newSide: 1, old: 2, want: 2},
		{name: "2.5 millionths rounds to the even 2", shares: 5, newSide: 1, old: 2, want: 2},
		{name: "3.5 millionths rounds to the even 4", shares: 7, newSide: 1, old: 2, want: 4},
		{name: "-1.5 millionths rounds to the even -2", shares: -3, newSide: 1, old: 2, want: -2},
		{name: "-2.5 millionths rounds to the even -2", shares: -5, newSide: 1, old: 2, want: -2},
		{name: "4.33 millionths rounds down to 4", shares: 13, newSide: 1, old: 3, want: 4},
		{name: "4.67 millionths rounds up to 5", shares: 14, newSide: 1, old: 3, want: 5},
		{name: "-4.67 millionths rounds to -5", shares: -14, newSide: 1, old: 3, want: -5},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			txns := []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, shareDay, c.shares),
				splitOf(acctOne, secAcme, 2, shareNext, c.newSide, c.old),
			}

			result := checkShares(t, txns, nil)

			require.Len(t, result.Mismatched, 1)
			assert.Equal(t, c.want, result.Mismatched[0].Quarry)
		})
	}
}

func Test_check_shares_reports_a_count_beyond_64_bits_as_the_nearest_it_can_hold(t *testing.T) {
	t.Parallel()
	const largest = 999_999_999_999_999_999
	txns := make([]store.InvestmentTransaction, 0, 20)
	for source := range int64(10) {
		txns = append(txns,
			buy(acctOne, secAcme, source, shareDay, largest),
			buy(acctOne, secBeta, 10+source, shareDay, -largest))
	}
	beyond := new(big.Int).Mul(big.NewInt(math.MaxInt64), big.NewInt(2))
	quicken := []store.QuickenShare{
		{AccountID: acctTwo, SecurityID: secAcme, Millionths: beyond},
		{AccountID: acctTwo, SecurityID: secBeta, Millionths: new(big.Int).Neg(beyond)},
	}

	result := checkShares(t, txns, quicken)

	assert.Equal(t, []store.ShareMismatch{
		{AccountID: acctOne, SecurityID: secAcme, Quarry: math.MaxInt64, Quicken: 0},
		{AccountID: acctOne, SecurityID: secBeta, Quarry: math.MinInt64, Quicken: 0},
		{AccountID: acctTwo, SecurityID: secAcme, Quarry: 0, Quicken: math.MaxInt64},
		{AccountID: acctTwo, SecurityID: secBeta, Quarry: 0, Quicken: math.MinInt64},
	}, result.Mismatched)
}

func Test_check_shares_refuses_a_split_row_with_a_ratio_side_it_cannot_divide_by(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		new, old *int64
	}{
		{name: "NULL new side", new: nil, old: new(int64(12))},
		{name: "NULL old side", new: new(int64(1)), old: nil},
		{name: "zero new side", new: new(int64(0)), old: new(int64(12))},
		{name: "zero old side", new: new(int64(1)), old: new(int64(0))},
		{name: "negative old side", new: new(int64(1)), old: new(int64(-12))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			split := splitOf(acctOne, secAcme, 2, shareNext, 1, 12)
			split.SplitNewShares, split.SplitOldShares = c.new, c.old

			_, err := duckstore.New(t.TempDir()).CheckShares(t.Context(), store.Rows{
				InvestmentTransactions: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, 120*oneShare), split},
			})

			require.ErrorContains(t, err, "split")
		})
	}
}

// scratchOver returns a Store whose scratch database is a real in-memory one behind f.
func scratchOver(f *faultDB) *duckstore.Store {
	return duckstore.New(".", duckstore.WithScratch(func(ctx context.Context) (duckstore.ScratchDB, error) {
		db, err := duckdb.CreateInMemory(ctx)
		if err != nil {
			return nil, err
		}
		f.DB = db
		return f, nil
	}))
}

func Test_check_shares_fails_when_the_scratch_database_cannot_be_created(t *testing.T) {
	t.Parallel()
	st := duckstore.New(".", duckstore.WithScratch(func(context.Context) (duckstore.ScratchDB, error) {
		return nil, errScratchBoom
	}))

	_, err := st.CheckShares(t.Context(), store.Rows{})

	require.ErrorIs(t, err, errScratchBoom)
}

func Test_check_shares_fails_when_the_context_ends_before_the_scratch_database_opens(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := duckstore.New(t.TempDir()).CheckShares(ctx, store.Rows{})

	require.ErrorIs(t, err, context.Canceled)
}

func Test_check_shares_fails_when_a_step_on_the_scratch_database_fails(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fault faultDB
		want  string
	}{
		{name: "schema", fault: faultDB{execFaultOn: duckstore.SchemaDDL, execFault: errScratchBoom}, want: "create schema"},
		{name: "load", fault: faultDB{appendFaultTable: "investment_transactions", appendFault: errScratchBoom}, want: "load investment_transactions"},
		{name: "walk query", fault: faultDB{queryFaultOn: duckstore.HoldingWalkQuery, queryFault: errScratchBoom}, want: "walk holdings"},
		{name: "walk row", fault: faultDB{queryFaultOn: duckstore.HoldingWalkQuery, scanFault: errScratchBoom}, want: "walk holdings"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			txns := []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, oneShare)}

			_, err := scratchOver(&c.fault).CheckShares(t.Context(), store.Rows{InvestmentTransactions: txns})

			require.ErrorIs(t, err, errScratchBoom)
			require.ErrorContains(t, err, c.want)
		})
	}
}

func Test_check_shares_refuses_a_share_count_that_does_not_fit_the_stored_column(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{buy(acctOne, secAcme, 7, shareDay, 1_000_000_000_000_000_000)}

	_, err := duckstore.New(t.TempDir()).CheckShares(t.Context(), store.Rows{InvestmentTransactions: txns})

	require.ErrorContains(t, err, "itxn-7")
}

func Test_check_shares_closes_the_scratch_database(t *testing.T) {
	t.Parallel()
	f := &faultDB{}

	_, err := scratchOver(f).CheckShares(t.Context(), store.Rows{
		InvestmentTransactions: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, oneShare)},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, f.closes)
}

func Test_check_shares_closes_the_scratch_database_when_a_step_fails(t *testing.T) {
	t.Parallel()
	f := &faultDB{queryFaultOn: duckstore.HoldingWalkQuery, queryFault: errScratchBoom}

	_, err := scratchOver(f).CheckShares(t.Context(), store.Rows{})

	require.Error(t, err)
	assert.Equal(t, 1, f.closes)
}
