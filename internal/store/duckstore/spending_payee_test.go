package duckstore_test

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func payeeParams() store.SpendingParams {
	params := spendingParams()
	params.By = store.SpendByPayee
	return params
}

// addPayees registers payees named names, each with id "payee-"+name.
func addPayees(rows *store.Rows, names ...string) {
	for _, name := range names {
		rows.Payees = append(rows.Payees, store.Payee{ID: "payee-" + name, SourceID: int64(len(rows.Payees) + 1), Name: name})
	}
}

func payeeSplit(id, payee, currency string, amount int64) splitSpec {
	spec := splitSpec{id: id, category: new(catExpense), currency: currency, amount: amount}
	if payee != "" {
		spec.payee = new("payee-" + payee)
	}
	return spec
}

func Test_spending_by_payee_lists_each_currencys_biggest_payee_first_and_a_refunded_one_last(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addPayees(&rows, "Zed", "Ann", "Refunder")
	addSplit(&rows, payeeSplit("zed-cad", "Zed", "CAD", -500))
	addSplit(&rows, payeeSplit("ann-cad", "Ann", "CAD", -900))
	addSplit(&rows, payeeSplit("none-cad", "", "CAD", -700))
	addSplit(&rows, payeeSplit("refund-cad", "Refunder", "CAD", 200))
	addSplit(&rows, payeeSplit("zed-usd", "Zed", "USD", -100))
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), payeeParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("Ann"), Currency: "CAD", Spent: 900},
		{Key: nil, Currency: "CAD", Spent: 700},
		{Key: new("Zed"), Currency: "CAD", Spent: 500},
		{Key: new("Refunder"), Currency: "CAD", Spent: -200},
		{Key: new("Zed"), Currency: "USD", Spent: 100},
	}, got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1900}, {Currency: "USD", Spent: 100}}, got.Totals)
}

func Test_spending_by_payee_breaks_a_tie_by_name_ignoring_case(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addPayees(&rows, "banana", "Cherry", "apple")
	addSplit(&rows, payeeSplit("banana", "banana", "CAD", -100))
	addSplit(&rows, payeeSplit("cherry", "Cherry", "CAD", -100))
	addSplit(&rows, payeeSplit("apple", "apple", "CAD", -100))
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), payeeParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("apple"), Currency: "CAD", Spent: 100},
		{Key: new("banana"), Currency: "CAD", Spent: 100},
		{Key: new("Cherry"), Currency: "CAD", Spent: 100},
	}, got.Rows)
}

func Test_spending_by_payee_breaks_a_case_only_tie_by_byte_order(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addPayees(&rows, "alpha", "Alpha")
	addSplit(&rows, payeeSplit("lower", "alpha", "CAD", -100))
	addSplit(&rows, payeeSplit("upper", "Alpha", "CAD", -100))
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), payeeParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("Alpha"), Currency: "CAD", Spent: 100},
		{Key: new("alpha"), Currency: "CAD", Spent: 100},
	}, got.Rows)
}

func Test_spending_by_payee_sorts_no_payee_after_a_named_payee_it_ties_with(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addPayees(&rows, "Zed")
	addSplit(&rows, payeeSplit("none", "", "CAD", -100))
	addSplit(&rows, payeeSplit("zed", "Zed", "CAD", -100))
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), payeeParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("Zed"), Currency: "CAD", Spent: 100},
		{Key: nil, Currency: "CAD", Spent: 100},
	}, got.Rows)
}

func Test_spending_by_payee_drops_a_payee_that_nets_to_zero(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addPayees(&rows, "Ann", "Even")
	addSplit(&rows, payeeSplit("ann", "Ann", "CAD", -300))
	addSplit(&rows, payeeSplit("even-out", "Even", "CAD", -50))
	addSplit(&rows, payeeSplit("even-back", "Even", "CAD", 50))
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), payeeParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{{Key: new("Ann"), Currency: "CAD", Spent: 300}}, got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 300}}, got.Totals)
}
