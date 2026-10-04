package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/require"
)

const holdingSpansText = `SELECT coalesce(string_agg(
	account_id || ' ' || security_id || ' ' || CAST(from_date AS VARCHAR) || '..' || coalesce(CAST(to_date AS VARCHAR), '') || ' ' || CAST(shares AS VARCHAR),
	'; ' ORDER BY account_id, security_id, from_date), '')
FROM holding_shares`

func marchDay(n int) time.Time { return day(2026, time.March, n) }

func Test_replace_records_holding_share_spans(t *testing.T) {
	t.Parallel()
	twelveThenSplit := func(buySource, splitSource int64) []store.InvestmentTransaction {
		return []store.InvestmentTransaction{
			buy(acctOne, secAcme, buySource, marchDay(1), 12*oneShare),
			splitOf(acctOne, secAcme, splitSource, marchDay(2), 1, 12),
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
				buy(acctOne, secAcme, 1, marchDay(1), 3*oneShare), buy(acctOne, secAcme, 2, marchDay(1), -oneShare),
			},
			want: "acct-1 sec-1 2026-03-01.. 2.000000",
		},
		{
			name: "the same rows on two dates are two changes",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), 3*oneShare), buy(acctOne, secAcme, 2, marchDay(2), -oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 3.000000; acct-1 sec-1 2026-03-02.. 2.000000",
		},
		{
			name: "a buy and sell of one date that cancel store nothing",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(1), -oneShare),
			},
			want: "",
		},
		{
			name: "a split multiplies the running count",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), 10*oneShare), splitOf(acctOne, secAcme, 2, marchDay(2), 2, 1),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 10.000000; acct-1 sec-1 2026-03-02.. 20.000000",
		},
		{
			name: "a change below one millionth opens no span",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), oneShare), splitOf(acctOne, secAcme, 2, marchDay(2), 3_000_001, 3_000_000),
			},
			want: "acct-1 sec-1 2026-03-01.. 1.000000",
		},
		{
			name: "a count that rounds to zero stores no span",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), 1), splitOf(acctOne, secAcme, 2, marchDay(2), 1, 3),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 0.000001",
		},
		{
			name: "sold to zero then bought again leaves a gap",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(2), -oneShare),
				buy(acctOne, secAcme, 3, marchDay(4), 4*oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 1.000000; acct-1 sec-1 2026-03-04.. 4.000000",
		},
		{
			name: "a negative count is stored",
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, marchDay(1), -oneShare)},
			want: "acct-1 sec-1 2026-03-01.. -1.000000",
		},
		{
			name: "changes on consecutive days give one-day spans",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(2), oneShare),
				buy(acctOne, secAcme, 3, marchDay(3), oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 1.000000; acct-1 sec-1 2026-03-02..2026-03-02 2.000000; acct-1 sec-1 2026-03-03.. 3.000000",
		},
		{
			name: "a future date opens its span as recorded",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, day(2999, time.January, 1), 2*oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2998-12-31 1.000000; acct-1 sec-1 2999-01-01.. 3.000000",
		},
		{
			name: "two securities of one account and one security of two accounts are kept apart",
			txns: []store.InvestmentTransaction{
				buy(acctTwo, secAcme, 3, marchDay(1), 3*oneShare), buy(acctOne, secBeta, 2, marchDay(1), 2*oneShare),
				buy(acctOne, secAcme, 1, marchDay(1), oneShare),
			},
			want: "acct-1 sec-1 2026-03-01.. 1.000000; acct-1 sec-2 2026-03-01.. 2.000000; acct-2 sec-1 2026-03-01.. 3.000000",
		},
		{
			name: "a short position bought back to positive closes the negative span",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), -oneShare), buy(acctOne, secAcme, 2, marchDay(2), 3*oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 -1.000000; acct-1 sec-1 2026-03-02.. 2.000000",
		},
		{
			name: "a short position bought back to exactly zero closes the negative span and opens none",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), -oneShare), buy(acctOne, secAcme, 2, marchDay(2), oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 -1.000000",
		},
		{
			name: "a one-day crossing from long to short closes the long span and opens the short one",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(2), -2*oneShare),
			},
			want: "acct-1 sec-1 2026-03-01..2026-03-01 1.000000; acct-1 sec-1 2026-03-02.. -1.000000",
		},
		{
			name: "a net-zero day in mid-history leaves one open span",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(2), oneShare),
				buy(acctOne, secAcme, 3, marchDay(2), -oneShare),
			},
			want: "acct-1 sec-1 2026-03-01.. 1.000000",
		},
		{
			name: "a change of exactly one millionth opens a new span",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secAcme, 2, marchDay(2), 1),
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
