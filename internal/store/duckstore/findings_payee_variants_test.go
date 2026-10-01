package duckstore_test

import (
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// variantPayee is a payee with how many transactions use it, in acct-1 unless account is set.
type variantPayee struct {
	name    string
	txns    int
	account string
}

// variantRows is rows whose payees are payee-1.. in argument order, each with its transactions.
func variantRows(payees ...variantPayee) store.Rows {
	rows := mixedRows()
	source := int64(0)
	for i, p := range payees {
		id := fmt.Sprintf("payee-%d", i+1)
		rows.Payees = append(rows.Payees, store.Payee{ID: id, SourceID: int64(i + 1), Name: p.name})
		account := p.account
		if account == "" {
			account = "acct-1"
		}
		for range p.txns {
			source++
			rows.Transactions = append(rows.Transactions, store.Transaction{
				ID: fmt.Sprintf("txn-%d", source), SourceID: source, AccountID: account, Date: day(2026, 1, int(source)),
				PayeeID: new(id), Amount: -1000 * source, Currency: "CAD", Status: "uncleared",
			})
		}
	}
	rows.Payees = rows.Payees[3:]
	return rows
}

// variantStore builds a store from variantRows and returns its read connection.
func variantStore(t *testing.T, payees ...variantPayee) *duckdb.DB {
	t.Helper()
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), variantRows(payees...))
	require.NoError(t, err)
	return openReadOnly(t, replaced.Path)
}

// variantFindings lists each payee-variants finding with its items in stored order, as "id=payee,payee" joined by "; ".
func variantFindings(t *testing.T, payees ...variantPayee) string {
	t.Helper()
	var got string
	require.NoError(t, variantStore(t, payees...).QueryRows(t.Context(),
		`SELECT COALESCE(string_agg(entry, '; ' ORDER BY id), '') FROM (
			SELECT f.id, f.id || '=' || string_agg(i.payee_id, ',' ORDER BY i.rowid) AS entry
			FROM findings f JOIN finding_items i ON i.finding_id = f.id
			WHERE f.type = 'payee-variants' GROUP BY f.id)`, nil,
		func(scan func(dest ...any) error) error { return scan(&got) }))
	return got
}

func Test_replace_flags_payee_variants_only_with_two_payees_sharing_a_key(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		payees []variantPayee
		want   string
	}{
		{"one payee alone", []variantPayee{{name: "Tim Hortons", txns: 2}}, ""},
		{"two payees with different keys", []variantPayee{{name: "Tim Hortons", txns: 1}, {name: "Tim Hortons Cafe", txns: 1}}, ""},
		{"two payees sharing a key", []variantPayee{{name: "TIM HORTONS #1234", txns: 1}, {name: "Tim Hortons", txns: 1}}, "payee-variants:tim-hortons=payee-2,payee-1"},
		{"identical names share a key", []variantPayee{{name: "Amazon", txns: 1}, {name: "Amazon", txns: 1}}, "payee-variants:amazon=payee-1,payee-2"},
		{"numbered names share their word", []variantPayee{{name: "Shop 1", txns: 1}, {name: "Shop 2", txns: 1}}, "payee-variants:shop=payee-1,payee-2"},
		{"names with no key are never a group", []variantPayee{{name: "#1", txns: 1}, {name: "#2", txns: 1}}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.want, variantFindings(t, c.payees...))
		})
	}
}

func Test_replace_ignores_a_payee_no_transaction_uses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		txns int
		want string
	}{
		{"unused", 0, ""},
		{"used once", 1, "payee-variants:tim-hortons=payee-1,payee-2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := variantFindings(t, variantPayee{name: "Tim Hortons", txns: 1}, variantPayee{name: "TIM HORTONS", txns: c.txns})

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_replace_counts_a_payees_transaction_in_a_closed_account_outside_reports(t *testing.T) {
	t.Parallel()

	got := variantFindings(t, variantPayee{name: "Tim Hortons", txns: 1}, variantPayee{name: "TIM HORTONS", txns: 1, account: "acct-2"})

	assert.Equal(t, "payee-variants:tim-hortons=payee-1,payee-2", got)
}

func Test_replace_groups_three_payees_sharing_a_key_into_one_finding(t *testing.T) {
	t.Parallel()

	got := variantFindings(t, variantPayee{name: "Tim Hortons", txns: 1}, variantPayee{name: "TIM HORTONS", txns: 1}, variantPayee{name: "Tim Hortons #7", txns: 1})

	assert.Equal(t, "payee-variants:tim-hortons=payee-1,payee-2,payee-3", got)
}

func Test_replace_gives_each_shared_key_its_own_finding(t *testing.T) {
	t.Parallel()

	got := variantFindings(t,
		variantPayee{name: "Tim Hortons", txns: 1}, variantPayee{name: "Shell #1", txns: 1},
		variantPayee{name: "TIM HORTONS", txns: 1}, variantPayee{name: "Shell #2", txns: 1})

	assert.Equal(t, "payee-variants:shell=payee-2,payee-4; payee-variants:tim-hortons=payee-1,payee-3", got)
}

func Test_replace_lists_variants_by_transactions_then_name_ignoring_case_then_id(t *testing.T) {
	t.Parallel()

	got := variantFindings(t,
		variantPayee{name: "tim hortons", txns: 1}, variantPayee{name: "Tim Hortons", txns: 1}, variantPayee{name: "TIM HORTONS", txns: 1},
		variantPayee{name: "Tim-Hortons", txns: 1}, variantPayee{name: "Tim Hortons #5", txns: 3})

	assert.Equal(t, "payee-variants:tim-hortons=payee-5,payee-1,payee-2,payee-3,payee-4", got)
}
