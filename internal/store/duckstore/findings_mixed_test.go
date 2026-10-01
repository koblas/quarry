package duckstore_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	mixedPayee      = "payee-1"
	mixedOtherPayee = "payee-2"
	mixedThirdPayee = "payee-3"
)

// mixedFixture is one transaction with its splits, for a mixed-categories store.
type mixedFixture struct {
	txn    store.Transaction
	splits []store.Split
}

// mixedTxn is a transaction of payee (empty for none) in acct-1 on date with source id n and one split in category
// cat-<cat>.
func mixedTxn(n int64, payee, cat string, date time.Time) mixedFixture {
	f := mixedFixture{txn: store.Transaction{
		ID: fmt.Sprintf("txn-%d", n), SourceID: n, AccountID: "acct-1", Date: date, Amount: -1000, Currency: "CAD", Status: "uncleared",
	}}
	if payee != "" {
		f.txn.PayeeID = &payee
	}
	return f.splitInto(cat)
}

// splitInto replaces the splits with one of -10.00 per category letter; "" makes an uncategorized split.
func (f mixedFixture) splitInto(cats ...string) mixedFixture {
	f.splits = nil
	for i, cat := range cats {
		s := store.Split{
			ID: fmt.Sprintf("split-%d-%d", f.txn.SourceID, i), SourceID: f.txn.SourceID*10 + int64(i), TransactionID: f.txn.ID, Amount: -1000,
		}
		if cat != "" {
			s.CategoryID = new("cat-" + cat)
		}
		f.splits = append(f.splits, s)
	}
	f.txn.Amount = -1000 * int64(len(cats))
	return f
}

func (f mixedFixture) inAccount(id string) mixedFixture {
	f.txn.AccountID = id
	return f
}

// mixedSeq is one payee's transactions, one per letter (the category), a day apart from 2026-01-01, source ids from first.
func mixedSeq(payee, letters string, first int64) []mixedFixture {
	out := make([]mixedFixture, 0, len(letters))
	for i, l := range letters {
		out = append(out, mixedTxn(first+int64(i), payee, string(l), day(2026, 1, 1+i)))
	}
	return out
}

// mixedRows is the rows of fixtures over the categories a-i (f to i share the path "Same") and the payees payee-1..3.
func mixedRows(fixtures ...mixedFixture) store.Rows {
	rows := minimalRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: "acct-2", SourceID: 2, Name: "Old Visa", Type: "credit", Currency: "CAD", Closed: true, NotInReports: true})
	rows.Categories = nil
	for letter, path := range map[string]string{"a": "Groceries", "b": "Household", "c": "Fuel", "d": "apple", "e": "Banana", "f": "Same", "g": "Same", "h": "Same", "i": "Same"} {
		rows.Categories = append(rows.Categories, store.Category{
			ID: "cat-" + letter, SourceID: int64(len(rows.Categories) + 1), Name: path, FullPath: path, Kind: "expense",
		})
	}
	rows.Payees = []store.Payee{
		{ID: mixedPayee, SourceID: 1, Name: "Costco"}, {ID: mixedOtherPayee, SourceID: 2, Name: "Shell"}, {ID: mixedThirdPayee, SourceID: 3, Name: "Bakery"},
	}
	rows.Transactions, rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil, nil
	for _, f := range fixtures {
		rows.Transactions = append(rows.Transactions, f.txn)
		rows.Splits = append(rows.Splits, f.splits...)
	}
	return rows
}

// mixedStore builds the store of fixtures and returns its read connection.
func mixedStore(t *testing.T, fixtures ...mixedFixture) *duckdb.DB {
	t.Helper()
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), mixedRows(fixtures...))
	require.NoError(t, err)
	return openReadOnly(t, replaced.Path)
}

// mixedIDs is the mixed-categories finding ids of the store built from fixtures, comma-joined in id order.
func mixedIDs(t *testing.T, fixtures ...mixedFixture) string {
	t.Helper()
	var ids string
	require.NoError(t, mixedStore(t, fixtures...).QueryRows(t.Context(),
		`SELECT COALESCE(string_agg(id, ',' ORDER BY id), '') FROM findings WHERE type = 'mixed-categories'`, nil,
		func(scan func(dest ...any) error) error { return scan(&ids) }))
	return ids
}

func Test_replace_flags_costco_as_mixed_categories_and_not_shell(t *testing.T) {
	t.Parallel()

	got := mixedIDs(t, append(mixedSeq(mixedPayee, "abac", 1), mixedSeq(mixedOtherPayee, "aacc", 10)...)...)

	assert.Equal(t, "mixed-categories:payee-1", got)
}

func Test_replace_flags_a_payee_only_when_a_category_is_revisited(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		seq  string
		want string
	}{
		{"two transactions, below the minimum", "ab", ""},
		{"a then b twice, one change per new category", "aab", ""},
		{"a twice then b, one change per new category", "abb", ""},
		{"three categories in runs, two changes", "aabbc", ""},
		{"one category five times", "aaaaa", ""},
		{"a b a, the minimum revisiting one", "aba", mixedIDOf(mixedPayee)},
		{"a b a c, a revisit among three categories", "abac", mixedIDOf(mixedPayee)},
		{"two categories sharing a path are still two", "fgf", mixedIDOf(mixedPayee)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := mixedIDs(t, mixedSeq(mixedPayee, c.seq, 1)...)

			assert.Equal(t, c.want, got)
		})
	}
}

func mixedIDOf(payee string) string { return "mixed-categories:" + payee }

func Test_replace_walks_a_payees_transactions_by_date_then_source_id(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		fixtures []mixedFixture
		want     string
	}{
		{"same date, the category changes by source id", []mixedFixture{
			mixedTxn(1, mixedPayee, "a", day(2026, 8, 1)), mixedTxn(3, mixedPayee, "a", day(2026, 8, 1)), mixedTxn(2, mixedPayee, "b", day(2026, 8, 1)),
		}, mixedIDOf(mixedPayee)},
		{"same date, source id order leaves the category in runs", []mixedFixture{
			mixedTxn(1, mixedPayee, "a", day(2026, 8, 1)), mixedTxn(3, mixedPayee, "b", day(2026, 8, 1)), mixedTxn(2, mixedPayee, "a", day(2026, 8, 1)),
		}, ""},
		{"a later date with a lower source id comes later", []mixedFixture{
			mixedTxn(1, mixedPayee, "a", day(2026, 8, 1)), mixedTxn(2, mixedPayee, "a", day(2026, 8, 3)), mixedTxn(3, mixedPayee, "b", day(2026, 8, 2)),
		}, mixedIDOf(mixedPayee)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := mixedIDs(t, c.fixtures...)

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_replace_judges_only_transactions_with_a_payee_and_exactly_one_categorized_reported_split(t *testing.T) {
	t.Parallel()
	d := func(n int) time.Time { return day(2026, 8, n) }
	cases := []struct {
		name     string
		fixtures []mixedFixture
		want     string
	}{
		{"control: a single-split middle transaction is judged", []mixedFixture{
			mixedTxn(1, mixedPayee, "a", d(1)), mixedTxn(2, mixedPayee, "b", d(2)), mixedTxn(3, mixedPayee, "a", d(3)),
		}, mixedIDOf(mixedPayee)},
		{"a middle transaction split across two categories", []mixedFixture{
			mixedTxn(1, mixedPayee, "a", d(1)), mixedTxn(2, mixedPayee, "b", d(2)).splitInto("b", "c"), mixedTxn(3, mixedPayee, "a", d(3)),
		}, ""},
		{"a middle transaction split twice across one category", []mixedFixture{
			mixedTxn(1, mixedPayee, "a", d(1)), mixedTxn(2, mixedPayee, "b", d(2)).splitInto("b", "b"), mixedTxn(3, mixedPayee, "a", d(3)),
		}, ""},
		{"a middle transaction with an uncategorized sibling split still counts", []mixedFixture{
			mixedTxn(1, mixedPayee, "a", d(1)), mixedTxn(2, mixedPayee, "b", d(2)).splitInto("b", ""), mixedTxn(3, mixedPayee, "a", d(3)),
		}, mixedIDOf(mixedPayee)},
		{"an uncategorized middle transaction", []mixedFixture{
			mixedTxn(1, mixedPayee, "a", d(1)), mixedTxn(2, mixedPayee, "", d(2)), mixedTxn(3, mixedPayee, "a", d(3)),
		}, ""},
		{"a middle transaction in an account outside reports", []mixedFixture{
			mixedTxn(1, mixedPayee, "a", d(1)), mixedTxn(2, mixedPayee, "b", d(2)).inAccount("acct-2"), mixedTxn(3, mixedPayee, "a", d(3)),
		}, ""},
		{"transactions with no payee", []mixedFixture{
			mixedTxn(1, "", "a", d(1)), mixedTxn(2, "", "b", d(2)), mixedTxn(3, "", "a", d(3)),
		}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := mixedIDs(t, c.fixtures...)

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_replace_judges_two_interleaved_payees_separately(t *testing.T) {
	t.Parallel()
	first := mixedSeq(mixedPayee, "aab", 1)
	second := mixedSeq(mixedOtherPayee, "abb", 10)
	third := mixedSeq(mixedThirdPayee, "aba", 20)
	fixtures := make([]mixedFixture, 0, 3*len(first))
	for i := range first {
		fixtures = append(fixtures, first[i], second[i], third[i])
	}

	got := mixedIDs(t, fixtures...)

	assert.Equal(t, mixedIDOf(mixedThirdPayee), got)
}

func Test_replace_records_one_item_per_category_in_count_then_path_then_id_order(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		seq  string
		want string
	}{
		{"the most transactions first, not the path", "abacab", "cat-a,cat-b,cat-c"},
		{"equal counts by path ignoring case", "eded", "cat-d,cat-e"},
		{"equal counts and paths by id", "ighfighf", "cat-f,cat-g,cat-h,cat-i"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var got string

			require.NoError(t, mixedStore(t, mixedSeq(mixedPayee, c.seq, 1)...).QueryRows(t.Context(),
				`SELECT string_agg(category_id, ',' ORDER BY rowid) FROM finding_items WHERE finding_id LIKE 'mixed-categories:%'`, nil,
				func(scan func(dest ...any) error) error { return scan(&got) }))

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_replace_records_a_mixed_item_with_its_payee_and_category_and_no_transaction_or_split(t *testing.T) {
	t.Parallel()
	db := mixedStore(t, mixedSeq(mixedPayee, "aba", 1)...)

	assertScalar(t, db, `SELECT string_agg(finding_id || '|' || payee_id || '|' || category_id || '|' || COALESCE(transaction_id, 'NULL') || '|' ||
		COALESCE(split_id, 'NULL'), '; ' ORDER BY rowid) FROM finding_items`,
		"mixed-categories:payee-1|payee-1|cat-a|NULL|NULL; mixed-categories:payee-1|payee-1|cat-b|NULL|NULL")
}
