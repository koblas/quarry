package duckstore_test

import (
	"strconv"
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chargesOf(t *testing.T, rows store.Rows) store.Charges {
	t.Helper()
	got, err := newStoreWith(t, rows).Charges(t.Context(), store.ChargeParams{Through: chargesThrough})
	require.NoError(t, err)
	return got
}

func Test_charges_sums_a_split_transaction_into_one_charge(t *testing.T) {
	t.Parallel()
	rows := chargeRowsFor()
	addCharge(&rows, chargeSpec{id: "split", sourceID: 1, splits: []splitPart{{new(catExpense), -300}, {new(catFuel), -200}}})

	got := chargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(500), got.Rows[0].Amount)
	assert.Equal(t, 2, got.Rows[0].ExpenseSplits)
	assert.Equal(t, "txn-split", got.Rows[0].TransactionID)
}

func Test_charges_drops_refunds_and_net_zero_transactions(t *testing.T) {
	t.Parallel()
	rows := chargeRowsFor()
	addCharge(&rows, chargeSpec{id: "refund", sourceID: 1, splits: []splitPart{{new(catExpense), 900}}})
	addCharge(&rows, chargeSpec{id: "net-zero", sourceID: 2, splits: []splitPart{{new(catExpense), -500}, {new(catFuel), 500}}})
	addCharge(&rows, chargeSpec{id: "net-negative", sourceID: 3, splits: []splitPart{{new(catExpense), -500}, {new(catFuel), 700}}})
	addCharge(&rows, chargeSpec{id: "penny", sourceID: 4, splits: []splitPart{{new(catExpense), -1}}})
	addCharge(&rows, oneSplit("normal", 5, 1200))

	got := chargesOf(t, rows)

	assert.Equal(t, []int64{1, 1200}, amountsOf(got))
}

func Test_charges_ignores_rows_dated_after_through(t *testing.T) {
	t.Parallel()
	rows := chargeRowsFor()
	onThrough, afterThrough := oneSplit("on-through", 1, 100), oneSplit("after-through", 2, 200)
	onThrough.date, afterThrough.date = chargesThrough, chargesThrough.AddDate(0, 0, 1)
	addCharge(&rows, onThrough)
	addCharge(&rows, afterThrough)

	got := chargesOf(t, rows)

	assert.Equal(t, []int64{100}, amountsOf(got))
}

func Test_charges_leaves_out_what_spending_leaves_out(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		spec      chargeSpec
		transfers []store.Transfer
	}{
		{
			name: "a transfer leg", spec: oneSplit("left-out", 2, 500),
			transfers: []store.Transfer{{ID: "xfer", FromSplitID: "left-out-a", ToSplitID: new("peer-leg")}},
		},
		{name: "an account left out of reports", spec: chargeSpec{id: "left-out", sourceID: 2, account: acctNotReports, splits: []splitPart{{new(catExpense), -500}}}},
		{name: "a linked-tracking account", spec: chargeSpec{id: "left-out", sourceID: 2, account: acctLinked, splits: []splitPart{{new(catExpense), -500}}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := chargeRowsFor()
			addCharge(&rows, oneSplit("kept", 1, 100))
			addCharge(&rows, c.spec)
			rows.Transfers = append(rows.Transfers, c.transfers...)

			got := chargesOf(t, rows)

			assert.Equal(t, []int64{100}, amountsOf(got))
		})
	}
}

func Test_charges_sets_the_category_only_when_every_expense_row_shares_one(t *testing.T) {
	t.Parallel()
	groceries := &store.ChargeCategory{ID: catExpense, Path: "Groceries"}
	cases := []struct {
		name   string
		splits []splitPart
		want   *store.ChargeCategory
		parts  int
	}{
		{name: "one split in one category", splits: []splitPart{{new(catExpense), -100}}, want: groceries, parts: 1},
		{name: "two splits in the same category", splits: []splitPart{{new(catExpense), -100}, {new(catExpense), -200}}, want: groceries, parts: 2},
		{name: "no category on any split", splits: []splitPart{{nil, -100}, {nil, -200}}, want: nil, parts: 2},
		{name: "two different categories", splits: []splitPart{{new(catExpense), -100}, {new(catFuel), -200}}, want: nil, parts: 2},
		{name: "a NULL category and one category", splits: []splitPart{{nil, -100}, {new(catExpense), -200}}, want: nil, parts: 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := chargeRowsFor()
			addCharge(&rows, chargeSpec{id: "only", sourceID: 1, splits: c.splits})

			got := chargesOf(t, rows)

			require.Len(t, got.Rows, 1)
			assert.Equal(t, c.want, got.Rows[0].Category)
			assert.Equal(t, c.parts, got.Rows[0].ExpenseSplits)
		})
	}
}

func Test_charges_names_the_payee(t *testing.T) {
	t.Parallel()
	rows := chargeRowsFor()
	spec := oneSplit("gym", 1, 1200)
	spec.payee = new(payeeGym)
	addCharge(&rows, spec)

	got := chargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Equal(t, new(payeeGym), got.Rows[0].PayeeID)
	assert.Equal(t, new(nameGym), got.Rows[0].Payee)
}

func Test_charges_keeps_a_charge_with_no_payee(t *testing.T) {
	t.Parallel()
	rows := chargeRowsFor()
	addCharge(&rows, oneSplit("anonymous", 1, 1200))

	got := chargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Nil(t, got.Rows[0].PayeeID)
	assert.Nil(t, got.Rows[0].Payee)
	assert.Equal(t, int64(1200), got.Rows[0].Amount)
}

func Test_charges_fills_the_account_and_currency(t *testing.T) {
	t.Parallel()
	rows := chargeRowsFor()
	rows.Accounts = append(rows.Accounts, store.Account{ID: "acct-old", SourceID: 9, Name: "Old Visa", Type: "credit_card", Currency: "EUR", Closed: true})
	spec := oneSplit("old", 1, 1200)
	spec.account, spec.currency, spec.date = "acct-old", "USD", day(2026, 4, 5)
	addCharge(&rows, spec)

	got := chargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Equal(t, store.Account{ID: "acct-old", Name: "Old Visa", Currency: "EUR", Closed: true}, got.Rows[0].Account)
	assert.Equal(t, "USD", got.Rows[0].Currency)
	assert.Equal(t, day(2026, 4, 5), got.Rows[0].Date)
}

func Test_charges_orders_by_date_then_numeric_source_id(t *testing.T) {
	t.Parallel()
	ten, nine := oneSplit("ten", 10, 100), oneSplit("nine", 9, 100)
	late, early := oneSplit("late", 1, 100), oneSplit("early", 2, 100)
	late.date, early.date = day(2026, 5, 1), day(2026, 4, 1)
	cases := []struct {
		name  string
		specs []chargeSpec
		want  []int64
	}{
		{name: "the higher source id inserted first", specs: []chargeSpec{ten, nine}, want: []int64{9, 10}},
		{name: "the lower source id inserted first", specs: []chargeSpec{nine, ten}, want: []int64{9, 10}},
		{name: "an earlier date outranks a lower source id", specs: []chargeSpec{late, early}, want: []int64{2, 1}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := chargeRowsFor()
			for _, spec := range c.specs {
				addCharge(&rows, spec)
			}

			got := chargesOf(t, rows)

			ids := make([]int64, len(got.Rows))
			for i, charge := range got.Rows {
				ids[i] = charge.SourceID
			}
			assert.Equal(t, c.want, ids)
		})
	}
}

func Test_charges_gives_the_span_of_every_transaction_in_the_store(t *testing.T) {
	t.Parallel()
	rows := chargeRowsFor()
	early, late := oneSplit("early", 1, 100), oneSplit("late", 2, 100)
	early.date, late.date = day(2003, 1, 4), day(2026, 8, 1)
	future := chargeSpec{id: "future", sourceID: 3, account: acctNotReports, date: day(2027, 2, 2), splits: []splitPart{{new(catExpense), 900}}}
	addCharge(&rows, early)
	addCharge(&rows, late)
	addCharge(&rows, future)

	got := chargesOf(t, rows)

	assert.Equal(t, store.TransactionRange{First: day(2003, 1, 4), Last: day(2027, 2, 2)}, got.Transactions)
	assert.Len(t, got.Rows, 2)
}

func Test_charges_gives_no_rows_and_a_zero_span_for_a_store_without_transactions(t *testing.T) {
	t.Parallel()

	got := chargesOf(t, spendRows())

	assert.Empty(t, got.Rows)
	assert.Zero(t, got.Transactions)
}

func Test_charges_returns_the_transaction_range_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT min"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, queryFault: fault}))

	_, err := st.Charges(t.Context(), store.ChargeParams{Through: chargesThrough})

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_charges_returns_a_transaction_range_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, scanFault: errScanFailed}))

	_, err := st.Charges(t.Context(), store.ChargeParams{Through: chargesThrough})

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}

// namedChargesOf reads chargesOf's charges with the span scoped to ids.
func namedChargesOf(t *testing.T, rows store.Rows, ids ...string) store.Charges {
	t.Helper()
	got, err := newStoreWith(t, rows).Charges(t.Context(), store.ChargeParams{Through: chargesThrough, AccountIDs: ids})
	require.NoError(t, err)
	return got
}

// chargeRowsWithAccounts is chargeRowsFor plus the second in-report account.
func chargeRowsWithAccounts() store.Rows {
	rows := chargeRowsFor()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctSecond, SourceID: 3, Name: "Savings", Type: "chequing", Currency: "CAD", Active: true})
	return rows
}

func transactionIDsOf(charges store.Charges) []string {
	ids := make([]string, len(charges.Rows))
	for i, c := range charges.Rows {
		ids[i] = c.TransactionID
	}
	return ids
}

func Test_charges_spans_only_the_named_reported_accounts_and_keeps_every_row(t *testing.T) {
	t.Parallel()
	rows := chargeRowsWithAccounts()
	linked := chargeSpec{id: "linked", sourceID: 1, account: acctLinked, date: day(1999, 1, 1), splits: []splitPart{{new(catExpense), -100}}}
	left := chargeSpec{id: "left-out", sourceID: 2, account: acctNotReports, date: day(2001, 5, 5), splits: []splitPart{{new(catExpense), -100}}}
	named := oneSplit("named", 3, 100)
	named.date = day(2019, 3, 2)
	other := oneSplit("other", 4, 100)
	other.account, other.date = acctSecond, day(2025, 8, 8)
	for _, spec := range []chargeSpec{linked, left, named, other} {
		addCharge(&rows, spec)
	}

	got := namedChargesOf(t, rows, acctNotReports, acctLinked, acctInReports)

	assert.Equal(t, store.TransactionRange{First: day(2019, 3, 2), Last: day(2019, 3, 2)}, got.Transactions)
	assert.Equal(t, []string{"txn-named", "txn-other"}, transactionIDsOf(got))
}

func Test_charges_spans_every_account_when_none_is_named(t *testing.T) {
	t.Parallel()
	rows := chargeRowsWithAccounts()
	named, other := oneSplit("named", 1, 100), oneSplit("other", 2, 100)
	named.date = day(2019, 3, 2)
	other.account, other.date = acctSecond, day(2025, 8, 8)
	addCharge(&rows, named)
	addCharge(&rows, other)

	got := namedChargesOf(t, rows)

	assert.Equal(t, store.TransactionRange{First: day(2019, 3, 2), Last: day(2025, 8, 8)}, got.Transactions)
}

func Test_charges_gives_a_zero_span_when_the_named_accounts_have_no_transactions(t *testing.T) {
	t.Parallel()
	rows := chargeRowsWithAccounts()
	addCharge(&rows, oneSplit("unnamed", 1, 100))

	got := namedChargesOf(t, rows, acctSecond)

	assert.Zero(t, got.Transactions)
	assert.Equal(t, []string{"txn-unnamed"}, transactionIDsOf(got))
}

// usdCharge is a one-split charge of cents on the USD account on date.
func usdCharge(id string, cents int64, date int) chargeSpec {
	return chargeSpec{
		id: id, sourceID: 1, account: acctUSD, currency: "USD", date: march(date),
		splits: []splitPart{{category: new(catExpense), cents: -cents}},
	}
}

// ratedChargesOf reads the charges of rows on a store holding fridayAndMonday's rates.
func ratedChargesOf(t *testing.T, rows store.Rows) store.Charges {
	t.Helper()
	got, err := newStoreWithRates(t, rows, fridayAndMonday()...).Charges(t.Context(), store.ChargeParams{Through: chargesThrough})
	require.NoError(t, err)
	return got
}

func Test_charges_converts_a_usd_charge_at_the_rate_of_its_date(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addCharge(&rows, usdCharge("usd", 1000, 16))

	got := ratedChargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(1300), *got.Rows[0].AmountCAD)
	assert.Equal(t, int64(1000), *got.Rows[0].AmountUSD)
	assert.Equal(t, money.Rate(mondayRate), got.Rows[0].USDCAD)
}

func Test_charges_converts_a_cad_charge_into_usd_at_the_rate_of_its_date(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addCharge(&rows, chargeSpec{id: "cad", sourceID: 1, date: march(13), splits: []splitPart{{category: new(catExpense), cents: -1250}}})

	got := ratedChargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(1250), *got.Rows[0].AmountCAD)
	assert.Equal(t, int64(1000), *got.Rows[0].AmountUSD)
	assert.Equal(t, money.Rate(fridayRate), got.Rows[0].USDCAD)
}

func Test_charges_sums_the_converted_splits_of_a_charge_not_its_converted_total(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	spec := usdCharge("split", 0, 13)
	spec.splits = []splitPart{{new(catExpense), -1}, {new(catExpense), -1}}
	addCharge(&rows, spec)

	got := ratedChargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(2), got.Rows[0].Amount)
	assert.Equal(t, int64(2), *got.Rows[0].AmountCAD)
}

func Test_charges_cells_match_money_convert_for_single_split_charges(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	var id int64
	for _, currency := range []string{"CAD", "USD"} {
		for _, cents := range []int64{1, 10, 20, 99, 12_345, 99_999_999} {
			for _, day := range []int{13, 16, 17} {
				id++
				spec := chargeSpec{id: "c" + strconv.FormatInt(id, 10), sourceID: id, date: march(day), splits: []splitPart{{category: new(catExpense), cents: -cents}}}
				if currency == "USD" {
					spec.account, spec.currency = acctUSD, "USD"
				}
				addCharge(&rows, spec)
			}
		}
	}
	st := newStoreWithRates(t, rows, ratesOn(13, 1_250_000, "FXUSDCAD"), ratesOn(16, 1_600_001, "FXUSDCAD"), ratesOn(17, 1_249_999, "FXUSDCAD"))

	got, err := st.Charges(t.Context(), store.ChargeParams{Through: chargesThrough})

	require.NoError(t, err)
	require.Len(t, got.Rows, int(id))
	for _, c := range got.Rows {
		own, _ := money.ParseCurrency(c.Currency)
		wantCAD, okCAD := money.Convert(c.Amount, own, money.CAD, c.USDCAD)
		wantUSD, okUSD := money.Convert(c.Amount, own, money.USD, c.USDCAD)
		require.True(t, okCAD && okUSD)
		require.NotNil(t, c.AmountCAD)
		require.NotNil(t, c.AmountUSD)
		assert.Equal(t, []int64{wantCAD, wantUSD}, []int64{*c.AmountCAD, *c.AmountUSD}, "%s %d at %d", c.Currency, c.Amount, c.USDCAD)
	}
}

func Test_charges_cell_of_a_split_charge_differs_from_converting_its_total(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	spec := usdCharge("split", 0, 13)
	spec.splits = []splitPart{{new(catExpense), -1}, {new(catExpense), -1}}
	addCharge(&rows, spec)

	got := ratedChargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	converted, ok := money.Convert(got.Rows[0].Amount, money.USD, money.CAD, got.Rows[0].USDCAD)
	require.True(t, ok)
	assert.Equal(t, int64(3), converted)
	assert.Equal(t, int64(2), *got.Rows[0].AmountCAD)
}

func Test_charges_takes_the_prior_rate_for_a_weekend_charge(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addCharge(&rows, usdCharge("weekend", 1000, 14))

	got := ratedChargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Equal(t, money.Rate(fridayRate), got.Rows[0].USDCAD)
	assert.Equal(t, int64(1250), *got.Rows[0].AmountCAD)
}

func Test_charges_leaves_the_cross_cell_and_rate_empty_for_a_usd_charge_before_the_first_rate(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addCharge(&rows, usdCharge("early", 1000, 10))

	got := ratedChargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Nil(t, got.Rows[0].AmountCAD)
	assert.Equal(t, int64(1000), *got.Rows[0].AmountUSD)
	assert.Zero(t, got.Rows[0].USDCAD)
}

func Test_charges_leaves_the_cross_cell_and_rate_empty_for_a_cad_charge_before_the_first_rate(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addCharge(&rows, chargeSpec{id: "early", sourceID: 1, date: march(10), splits: []splitPart{{category: new(catExpense), cents: -700}}})

	got := ratedChargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(700), *got.Rows[0].AmountCAD)
	assert.Nil(t, got.Rows[0].AmountUSD)
	assert.Zero(t, got.Rows[0].USDCAD)
}

func Test_charges_gives_a_cad_charge_in_a_store_without_rates_as_its_own_cad_cell(t *testing.T) {
	t.Parallel()
	rows := chargeRowsFor()
	addCharge(&rows, oneSplit("cad", 1, 1200))

	got := chargesOf(t, rows)

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(1200), *got.Rows[0].AmountCAD)
	assert.Nil(t, got.Rows[0].AmountUSD)
	assert.Zero(t, got.Rows[0].USDCAD)
}

func Test_charges_gives_the_date_of_the_first_rate(t *testing.T) {
	t.Parallel()

	got := ratedChargesOf(t, fxSpendRows())

	assert.Equal(t, march(13), got.FirstRate)
}

func Test_charges_gives_no_first_rate_date_for_a_store_without_rates(t *testing.T) {
	t.Parallel()

	got := chargesOf(t, chargeRowsFor())

	assert.True(t, got.FirstRate.IsZero())
}

func Test_charges_returns_the_first_rate_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT min(date) FROM fx_rates"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 2, queryFault: fault}))

	_, err := st.Charges(t.Context(), store.ChargeParams{Through: chargesThrough})

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_charges_returns_a_first_rate_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 2, scanFault: errScanFailed}))

	_, err := st.Charges(t.Context(), store.ChargeParams{Through: chargesThrough})

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}
