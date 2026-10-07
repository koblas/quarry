package duckstore_test

import (
	"context"
	"errors"
	"math"
	"math/big"
	"os"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errScratchBoom = errors.New("scratch boom")

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
		buy(acctOne, secAcme, 2, march(1), 12*oneShare),
		splitOf(acctOne, secAcme, 1, march(2), 1, 12),
	}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, oneShare)})

	assert.Equal(t, store.ShareCheck{Checked: 1}, result)
}

func Test_check_shares_breaks_a_same_date_tie_by_numeric_source_id(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{
		splitOf(acctOne, secAcme, 10, march(1), 1, 12),
		buy(acctOne, secAcme, 9, march(1), 12*oneShare),
	}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, oneShare)})

	assert.Equal(t, store.ShareCheck{Checked: 1}, result)
}

func Test_check_shares_multiplies_the_running_count_by_the_split_ratio(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, march(1), 120*oneShare),
		splitOf(acctOne, secAcme, 2, march(2), 1, 12),
	}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, 10*oneShare)})

	assert.Equal(t, store.ShareCheck{Checked: 1}, result)
}

func Test_check_shares_adds_nothing_at_a_split_row_that_carries_shares(t *testing.T) {
	t.Parallel()
	split := splitOf(acctOne, secAcme, 2, march(2), 1, 12)
	split.Shares = new(int64(5 * oneShare))
	txns := []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), 120*oneShare), split}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, 10*oneShare)})

	assert.Equal(t, store.ShareCheck{Checked: 1}, result)
}

func Test_check_shares_matches_within_one_millionth(t *testing.T) {
	t.Parallel()
	oneBought := []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare)}
	hundredSplitTwelveWays := []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, march(1), 100*oneShare),
		splitOf(acctOne, secAcme, 2, march(2), 1, 12),
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

func Test_check_shares_counts_a_fully_sold_holding_as_zero(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, march(1), oneShare),
		buy(acctOne, secAcme, 2, march(2), -oneShare),
	}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, oneShare)})

	require.Len(t, result.Mismatched, 1)
	assert.Equal(t, 1, result.Checked)
	assert.Zero(t, result.Mismatched[0].Quarry)
	assert.EqualValues(t, oneShare, result.Mismatched[0].Quicken)
}

func Test_check_shares_counts_rows_of_every_date_including_future_and_before_2001(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, time.Date(1999, 12, 31, 0, 0, 0, 0, time.UTC), 3*oneShare),
		buy(acctOne, secAcme, 2, march(1), 5*oneShare),
		buy(acctOne, secAcme, 3, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), 7*oneShare),
	}

	result := checkShares(t, txns, []store.QuickenShare{quickenCount(acctOne, secAcme, 15*oneShare)})

	assert.Equal(t, store.ShareCheck{Checked: 1}, result)
}

func Test_check_shares_counts_the_holdings_it_compares(t *testing.T) {
	t.Parallel()
	cashOnly := buy(acctOne, secAcme, 3, march(1), 0)
	cashOnly.SecurityID, cashOnly.Shares = nil, nil
	nullShares := buy(acctOne, secAcme, 4, march(1), 0)
	nullShares.Shares = nil
	cases := []struct {
		name    string
		txns    []store.InvestmentTransaction
		quicken []store.QuickenShare
		want    int
	}{
		{name: "no rows"},
		{name: "transactions and no lot", txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare)}, want: 1},
		{name: "lot and no transactions", quicken: []store.QuickenShare{quickenCount(acctOne, secAcme, oneShare)}, want: 1},
		{name: "a lot of zero units and no transactions", quicken: []store.QuickenShare{quickenCount(acctOne, secAcme, 0)}, want: 1},
		{name: "a row of zero shares", txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), 0)}, want: 1},
		{name: "a row of NULL shares", txns: []store.InvestmentTransaction{nullShares}, want: 1},
		{name: "a row with no security", txns: []store.InvestmentTransaction{cashOnly}},
		{
			name: "transactions and a lot of one holding", want: 1,
			txns:    []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare)},
			quicken: []store.QuickenShare{quickenCount(acctOne, secAcme, oneShare)},
		},
		{
			name: "one security in two accounts", want: 2,
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctTwo, secAcme, 2, march(1), oneShare)},
		},
		{
			name: "two securities in one account", want: 2,
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secBeta, 2, march(1), oneShare)},
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

	result := checkShares(t, []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), 5*oneShare)}, nil)

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
		buy(acctTwo, secBeta, 1, march(1), 4*oneShare),
		buy(acctOne, secBeta, 2, march(1), 2*oneShare),
		buy(acctOne, secAcme, 3, march(1), 9*oneShare),
		buy(acctTwo, secAcme, 4, march(1), 6*oneShare),
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
				buy(acctOne, secAcme, 1, march(1), c.shares),
				splitOf(acctOne, secAcme, 2, march(2), c.newSide, c.old),
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
			buy(acctOne, secAcme, source, march(1), largest),
			buy(acctOne, secBeta, 10+source, march(1), -largest))
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
			split := splitOf(acctOne, secAcme, 2, march(2), 1, 12)
			split.SplitNewShares, split.SplitOldShares = c.new, c.old

			_, err := duckstore.New(t.TempDir()).CheckShares(t.Context(), store.Rows{
				InvestmentTransactions: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), 120*oneShare), split},
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
			txns := []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare)}

			_, err := scratchOver(&c.fault).CheckShares(t.Context(), store.Rows{InvestmentTransactions: txns})

			require.ErrorIs(t, err, errScratchBoom)
			require.ErrorContains(t, err, c.want)
		})
	}
}

func Test_check_shares_refuses_a_share_count_that_does_not_fit_the_stored_column(t *testing.T) {
	t.Parallel()
	txns := []store.InvestmentTransaction{buy(acctOne, secAcme, 7, march(1), 1_000_000_000_000_000_000)}

	_, err := duckstore.New(t.TempDir()).CheckShares(t.Context(), store.Rows{InvestmentTransactions: txns})

	require.ErrorContains(t, err, "itxn-7")
}

func Test_check_shares_closes_the_scratch_database(t *testing.T) {
	t.Parallel()
	f := &faultDB{}

	_, err := scratchOver(f).CheckShares(t.Context(), store.Rows{
		InvestmentTransactions: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare)},
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

const holdingSpansText = `SELECT coalesce(string_agg(
	account_id || ' ' || security_id || ' ' || CAST(from_date AS VARCHAR) || '..' || coalesce(CAST(to_date AS VARCHAR), '') || ' ' || CAST(shares AS VARCHAR),
	'; ' ORDER BY account_id, security_id, from_date), '')
FROM holding_shares`

func Test_replace_records_holding_share_spans(t *testing.T) {
	t.Parallel()
	twelveThenSplit := func(buySource, splitSource int64) []store.InvestmentTransaction {
		return []store.InvestmentTransaction{
			buy(acctOne, secAcme, buySource, march(1), 12*oneShare),
			splitOf(acctOne, secAcme, splitSource, march(2), 1, 12),
		}
	}
	cases := []struct {
		name string
		txns []store.InvestmentTransaction
		want string
	}{
		{
			name: "a later date with the lower source id still comes after",
			txns: twelveThenSplit(2, 1),
			want: "acct-1 sec-1 2026-03-01..2026-03-01 12.000000; acct-1 sec-1 2026-03-02.. 1.000000",
		},
		{
			name: "an earlier date with the lower source id comes first",
			txns: twelveThenSplit(1, 2),
			want: "acct-1 sec-1 2026-03-01..2026-03-01 12.000000; acct-1 sec-1 2026-03-02.. 1.000000",
		},
		{
			name: "rows of one date are netted into one change",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), 3*oneShare), buy(acctOne, secAcme, 2, march(1), -oneShare),
			},
			want: "acct-1 sec-1 2026-03-01.. 2.000000",
		},
		{
			name: "the same rows on two dates are two changes",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), 3*oneShare), buy(acctOne, secAcme, 2, march(2), -oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 3.000000; acct-1 sec-1 2026-03-02.. 2.000000",
		},
		{
			name: "a buy and sell of one date that cancel store nothing",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(1), -oneShare),
			},
			want: "",
		},
		{
			name: "a split multiplies the running count",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), 10*oneShare), splitOf(acctOne, secAcme, 2, march(2), 2, 1),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 10.000000; acct-1 sec-1 2026-03-02.. 20.000000",
		},
		{
			name: "a change below one millionth opens no span",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), splitOf(acctOne, secAcme, 2, march(2), 3_000_001, 3_000_000),
			},
			want: "acct-1 sec-1 2026-03-01.. 1.000000",
		},
		{
			name: "a count that rounds to zero stores no span",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), 1), splitOf(acctOne, secAcme, 2, march(2), 1, 3),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 0.000001",
		},
		{
			name: "sold to zero then bought again leaves a gap",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(2), -oneShare),
				buy(acctOne, secAcme, 3, march(4), 4*oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 1.000000; acct-1 sec-1 2026-03-04.. 4.000000",
		},
		{
			name: "a negative count is stored",
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), -oneShare)},
			want: "acct-1 sec-1 2026-03-01.. -1.000000",
		},
		{
			name: "changes on consecutive days give one-day spans",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(2), oneShare),
				buy(acctOne, secAcme, 3, march(3), oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 1.000000; acct-1 sec-1 2026-03-02..2026-03-02 2.000000; acct-1 sec-1 2026-03-03.. 3.000000",
		},
		{
			name: "a future date opens its span as recorded",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, day(2999, time.January, 1), 2*oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2998-12-31 1.000000; acct-1 sec-1 2999-01-01.. 3.000000",
		},
		{
			name: "two securities of one account and one security of two accounts are kept apart",
			txns: []store.InvestmentTransaction{
				buy(acctTwo, secAcme, 3, march(1), 3*oneShare), buy(acctOne, secBeta, 2, march(1), 2*oneShare),
				buy(acctOne, secAcme, 1, march(1), oneShare),
			},
			want: "acct-1 sec-1 2026-03-01.. 1.000000; acct-1 sec-2 2026-03-01.. 2.000000; acct-2 sec-1 2026-03-01.. 3.000000",
		},
		{
			name: "a short position bought back to positive closes the negative span",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), -oneShare), buy(acctOne, secAcme, 2, march(2), 3*oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 -1.000000; acct-1 sec-1 2026-03-02.. 2.000000",
		},
		{
			name: "a short position bought back to exactly zero closes the negative span and opens none",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), -oneShare), buy(acctOne, secAcme, 2, march(2), oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 -1.000000",
		},
		{
			name: "a one-day crossing from long to short closes the long span and opens the short one",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(2), -2*oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 1.000000; acct-1 sec-1 2026-03-02.. -1.000000",
		},
		{
			name: "a net-zero day in mid-history leaves one open span",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(2), oneShare),
				buy(acctOne, secAcme, 3, march(2), -oneShare),
			},
			want: "acct-1 sec-1 2026-03-01.. 1.000000",
		},
		{
			name: "a change of exactly one millionth opens a new span",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(2), 1),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 1.000000; acct-1 sec-1 2026-03-02.. 1.000001",
		},
		{name: "no investment transactions store no spans", txns: nil, want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := minimalRows()
			rows.InvestmentTransactions = c.txns

			replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

			require.NoError(t, err)
			assertScalar(t, openReadOnly(t, replaced.Path), holdingSpansText, c.want)
		})
	}
}

func realStore(dir string) *duckstore.Store { return duckstore.New(dir) }

func faultyStore(f *faultDB) func(dir string) *duckstore.Store {
	return func(dir string) *duckstore.Store { return newFaultStore(dir, f) }
}

func Test_replace_keeps_the_previous_store_when_holding_shares_cannot_be_loaded(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		txns     []store.InvestmentTransaction
		newStore func(dir string) *duckstore.Store
		want     string
	}{
		{
			name:     "walk query",
			txns:     []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare)},
			newStore: faultyStore(&faultDB{queryFaultOn: duckstore.HoldingWalkQuery, queryFault: errScratchBoom}),
			want:     "walk holdings",
		},
		{
			name: "append",
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare)},
			newStore: faultyStore(&faultDB{appendFaultTable: "holding_shares", appendFault: &duckdbdriver.Error{
				Type: duckdbdriver.ErrorTypeConstraint, Msg: "Constraint Error: Duplicate key violates primary key constraint.",
			}}),
			want: "load holding_shares",
		},
		{
			name:     "split with a zero ratio",
			txns:     []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare), splitOf(acctOne, secAcme, 2, march(2), 0, oneShare)},
			newStore: realStore,
			want:     "split of sec-1 in acct-1",
		},
		{
			name: "split pushing the count one millionth past the column",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), 333_333_333_333_333_334), splitOf(acctOne, secAcme, 2, march(2), 3*oneShare, oneShare),
			},
			newStore: realStore,
			want:     "holding_shares sec-1 in acct-1",
		},
		{
			name: "split pushing a short count one millionth past the column",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), -333_333_333_333_333_334), splitOf(acctOne, secAcme, 2, march(2), 3*oneShare, oneShare),
			},
			newStore: realStore,
			want:     "holding_shares sec-1 in acct-1",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
			require.NoError(t, err)
			before, err := os.ReadFile(replaced.Path)
			require.NoError(t, err)
			rows := minimalRows()
			rows.Transactions[0].Amount = 999
			rows.InvestmentTransactions = c.txns

			_, err = c.newStore(dir).Replace(t.Context(), rows)

			require.ErrorContains(t, err, c.want)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
			after, err := os.ReadFile(replaced.Path)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func Test_replace_stores_a_share_count_at_the_column_maximum(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, march(1), 333_333_333_333_333_333), splitOf(acctOne, secAcme, 2, march(2), 3*oneShare, oneShare),
	}

	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, "SELECT CAST(shares AS VARCHAR) FROM holding_shares WHERE to_date IS NULL", "999999999999.999999")
}

func Test_replace_stores_a_short_share_count_at_the_column_minimum(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, march(1), -333_333_333_333_333_333), splitOf(acctOne, secAcme, 2, march(2), 3*oneShare, oneShare),
	}

	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, "SELECT CAST(shares AS VARCHAR) FROM holding_shares WHERE to_date IS NULL", "-999999999999.999999")
}
