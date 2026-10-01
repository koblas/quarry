package duckstore_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
