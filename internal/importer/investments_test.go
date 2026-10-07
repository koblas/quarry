package importer_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func actionsOf(fake *fakeStore) []string {
	out := make([]string, len(fake.Rows.InvestmentTransactions))
	for i, txn := range fake.Rows.InvestmentTransactions {
		out[i] = txn.Action
	}
	return out
}

func Test_import_names_each_of_the_thirteen_mapped_action_codes(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	codes := []int64{2, 3, 6, 7, 8, 9, 10, 11, 12, 15, 17, 19, 23}
	for _, code := range codes {
		investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: &code, Amount: "1.00", PostedDate: &investDay, Numerator: "1", Denominator: "1"})
	}

	fake, result := importOK(t, b)

	assert.Equal(t, []string{
		"add_shares", "buy", "margin_interest", "misc_expense", "capital_gain_long", "capital_gain_short",
		"dividend", "interest", "misc_income", "reinvest_dividend", "remove_shares", "sell", "split",
	}, actionsOf(fake))
	assert.Equal(t, 13, result.Counts.InvestmentTransactions)
}

func Test_import_refuses_an_unmapped_action_code(t *testing.T) {
	t.Parallel()
	for _, code := range []int64{5, 14, 21} {
		t.Run(fmt.Sprintf("code %d", code), func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: &code, Amount: "1.00", PostedDate: &investDay})

			reason, fake := importRefused(t, b)

			assert.Equal(t, fmt.Sprintf(`an investment transaction on 2026-03-01 in "Brokerage" has action code %d, which quarry does not map yet`, code), reason)
			assert.Zero(t, fake.replaceCalls)
		})
	}
}

func Test_import_refuses_an_investment_transaction_with_no_action_code(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Amount: "1.00", PostedDate: &investDay})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `an investment transaction on 2026-03-01 in "Brokerage" has no action code`, reason)
}

func Test_import_refuses_an_investment_transaction_with_neither_a_posted_nor_an_entered_date(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		units, amount string
	}{
		{name: "readable_values", amount: "1.00"},
		{name: "before_any_other_fault", units: "n/a", amount: "n/a"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Units: c.units, Amount: c.amount})

			reason, _ := importRefused(t, b)

			assert.Equal(t, fmt.Sprintf(`an investment transaction in "Brokerage" (source id %d) has no date`, pk), reason)
		})
	}
}

func Test_import_ignores_an_unreadable_action_code_in_a_deleted_account(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		code *int64
	}{
		{name: "NULL code", code: nil},
		{name: "unmapped code", code: new(int64(14))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			liveAccountPK := newBrokerage(b)
			deletedAccountPK := b.Account(v9fixture.AccountRow{Name: "Old Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Deleted: true})
			livePK := investmentWithEntry(b, v9fixture.TransactionRow{Account: liveAccountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay})
			b.InvestmentTransaction(v9fixture.TransactionRow{Account: deletedAccountPK, Type: c.code, Amount: "1.00", PostedDate: &investDay})

			fake, _ := importOK(t, b)

			require.Len(t, fake.Rows.InvestmentTransactions, 1)
			assert.Equal(t, livePK, fake.Rows.InvestmentTransactions[0].SourceID)
		})
	}
}

func Test_import_dates_an_investment_transaction_by_its_posted_day_else_its_entered_day(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		posted  *time.Time
		entered *time.Time
		want    time.Time
	}{
		{name: "posted and entered", posted: &investDay, entered: &investLater, want: investDay},
		{name: "entered only", posted: nil, entered: &investLater, want: investLater},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: c.posted, EnteredDate: c.entered})

			fake, _ := importOK(t, b)

			require.Len(t, fake.Rows.InvestmentTransactions, 1)
			assert.Equal(t, c.want, fake.Rows.InvestmentTransactions[0].Date)
		})
	}
}

func Test_import_leaves_out_an_investment_row_it_does_not_import(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  func(accountPK int64) v9fixture.TransactionRow
	}{
		{name: "deleted_row", row: func(accountPK int64) v9fixture.TransactionRow {
			return v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "2.00", PostedDate: &investDay, Deleted: true}
		}},
		{name: "row_of_another_entity", row: func(accountPK int64) v9fixture.TransactionRow {
			return v9fixture.TransactionRow{Entity: 999, Account: accountPK, Type: buyCode, Amount: "2.00", PostedDate: &investDay}
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			keptPK := investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay})
			b.InvestmentTransaction(c.row(accountPK))

			fake, _ := importOK(t, b)

			require.Len(t, fake.Rows.InvestmentTransactions, 1)
			assert.Equal(t, keptPK, fake.Rows.InvestmentTransactions[0].SourceID)
		})
	}
}

func Test_import_resolves_the_security_of_an_investment_transaction_through_its_position(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Position: positionPK, Units: "0"})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Equal(t, new(fmt.Sprintf("sec-%d", acmePK)), fake.Rows.InvestmentTransactions[0].SecurityID)
}

func Test_import_gives_a_zero_share_row_no_security_when_its_position_cannot_be_resolved(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(b *v9fixture.Builder, accountPK, acmePK int64) int64
	}{
		{name: "position of another entity", setup: func(b *v9fixture.Builder, accountPK, acmePK int64) int64 {
			return b.Position(v9fixture.PositionRow{Entity: 999, Account: accountPK, Security: acmePK})
		}},
		{name: "deleted position", setup: func(b *v9fixture.Builder, accountPK, acmePK int64) int64 {
			return b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK, Deleted: true})
		}},
		{name: "position in a deleted account", setup: func(b *v9fixture.Builder, _, acmePK int64) int64 {
			deletedPK := b.Account(v9fixture.AccountRow{Name: "Old Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Deleted: true})
			return b.Position(v9fixture.PositionRow{Account: deletedPK, Security: acmePK})
		}},
		{name: "position with no security", setup: func(b *v9fixture.Builder, accountPK, _ int64) int64 {
			return b.Position(v9fixture.PositionRow{Account: accountPK})
		}},
		{name: "position with no account", setup: func(b *v9fixture.Builder, _, acmePK int64) int64 {
			return b.Position(v9fixture.PositionRow{Security: acmePK})
		}},
		{name: "deleted security", setup: func(b *v9fixture.Builder, accountPK, _ int64) int64 {
			goneSecurityPK := b.Security(v9fixture.SecurityRow{Name: "Gone Inc", Ticker: "GONE", Currency: "CAD", Deleted: true})
			return b.Position(v9fixture.PositionRow{Account: accountPK, Security: goneSecurityPK})
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			acmePK := newAcme(b)
			positionPK := c.setup(b, accountPK, acmePK)
			investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Position: positionPK, Units: "0"})

			fake, _ := importOK(t, b)

			require.Len(t, fake.Rows.InvestmentTransactions, 1)
			assert.Nil(t, fake.Rows.InvestmentTransactions[0].SecurityID)
		})
	}
}

// An account at Z_PK 0 exists, so a NULL account read as 0 would be checked against it.
func Test_import_skips_an_investment_transaction_with_no_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	livePK := investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay})
	b.InvestmentTransaction(v9fixture.TransactionRow{Type: buyCode, Amount: "2.00", PostedDate: &investDay})
	dataPath := snapshotPath(t, b)
	execOn(t, dataPath, "INSERT INTO ZACCOUNT (Z_PK, ZNAME, ZTYPENAME, ZCURRENCY, ZACTIVE) VALUES (0, 'Zero', 'BROKERAGENORMAL', 'CAD', 1)")

	fake, _ := importOKFrom(t, dataPath)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Equal(t, livePK, fake.Rows.InvestmentTransactions[0].SourceID)
}

// A position at Z_PK 0 exists, so a NULL position read as 0 would resolve to it.
func Test_import_gives_a_row_with_no_position_no_security_though_a_position_has_pk_zero(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Units: "0"})
	dataPath := snapshotPath(t, b)
	execOn(t, dataPath, "UPDATE ZPOSITION SET Z_PK = 0 WHERE Z_PK = ?", positionPK)

	fake, _ := importOKFrom(t, dataPath)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Nil(t, fake.Rows.InvestmentTransactions[0].SecurityID)
}

func Test_import_leaves_out_a_position_row_of_another_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	otherPositionPK := b.Position(v9fixture.PositionRow{Entity: 999, Account: accountPK, Security: acmePK})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Position: otherPositionPK, Units: "0"})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Nil(t, fake.Rows.InvestmentTransactions[0].SecurityID)
}

func Test_import_imports_investment_transactions_without_a_security_when_the_snapshot_has_no_position_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("Position")
	accountPK := newBrokerage(b)
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Nil(t, fake.Rows.InvestmentTransactions[0].SecurityID)
}

func Test_import_imports_no_investment_transactions_when_the_snapshot_has_no_investment_transaction_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("InvestmentTransaction")
	accountPK := newBrokerage(b)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Amount: "5.00", PostedDate: &investDay})

	fake, result := importOK(t, b)

	assert.Empty(t, fake.Rows.InvestmentTransactions)
	assert.Zero(t, result.Counts.InvestmentTransactions)
}

func Test_import_stores_commission_as_ten_thousandths_and_null_for_none_or_zero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		commission string
		want       *int64
	}{
		{name: "NULL", commission: "", want: nil},
		{name: "stored zero", commission: "0", want: nil},
		{name: "residue snapping to 0.0000", commission: "0.000000001", want: nil},
		{name: "1.50", commission: "1.50", want: new(int64(15_000))},
		{name: "8.4998", commission: "8.4998", want: new(int64(84_998))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Commission: c.commission})

			fake, _ := importOK(t, b)

			require.Len(t, fake.Rows.InvestmentTransactions, 1)
			assert.Equal(t, c.want, fake.Rows.InvestmentTransactions[0].Commission)
		})
	}
}

func Test_import_stores_cost_basis_in_cents_and_null_for_none_or_zero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		commission string
		costBasis  string
		want       *int64
	}{
		{name: "NULL", costBasis: "", want: nil},
		{name: "stored zero", costBasis: "0", want: nil},
		{name: "residue snapping to 0.00", costBasis: "0.000000001", want: nil},
		{name: "1000.50", costBasis: "1000.50", want: new(int64(100_050))},
		{name: "stored as an integer", costBasis: "1000", want: new(int64(100_000))},
		{name: "negative as recorded", costBasis: "-5.00", want: new(int64(-500))},
		{name: "set with a NULL commission", commission: "", costBasis: "20.25", want: new(int64(2_025))},
		{name: "set with a commission", commission: "1.50", costBasis: "20.25", want: new(int64(2_025))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			investmentWithEntry(b, v9fixture.TransactionRow{
				Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Commission: c.commission, CostBasis: c.costBasis,
			})

			fake, _ := importOK(t, b)

			require.Len(t, fake.Rows.InvestmentTransactions, 1)
			assert.Equal(t, c.want, fake.Rows.InvestmentTransactions[0].CostBasis)
		})
	}
}

func Test_import_stores_a_commission_beside_a_cost_basis_of_none(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Commission: "1.50"})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Equal(t, new(int64(15_000)), fake.Rows.InvestmentTransactions[0].Commission)
	assert.Nil(t, fake.Rows.InvestmentTransactions[0].CostBasis)
}

func Test_import_sets_the_split_columns_only_on_a_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Numerator: "1", Denominator: "12"})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: new(int64(23)), Amount: "0", PostedDate: &investDay, Numerator: "1", Denominator: "12"})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 2)
	buy, split := fake.Rows.InvestmentTransactions[0], fake.Rows.InvestmentTransactions[1]
	assert.Nil(t, buy.SplitNewShares)
	assert.Nil(t, buy.SplitOldShares)
	assert.Equal(t, new(int64(1_000_000)), split.SplitNewShares)
	assert.Equal(t, new(int64(12_000_000)), split.SplitOldShares)
}

func Test_import_reads_shares_and_amount_in_quickens_sign_with_the_accounts_currency(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	usdPK := b.Account(v9fixture.AccountRow{Name: "US Brokerage", Type: "BROKERAGENORMAL", Currency: "USD", Active: true})
	acmePK := newAcme(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: usdPK, Security: acmePK})
	pk := investmentWithEntry(b, v9fixture.TransactionRow{
		Account: usdPK, Type: new(int64(19)), Amount: "400.25", PostedDate: &investDay, Position: positionPK, Units: "-4.5",
	})

	fake, _ := importOK(t, b)

	assert.Equal(t, []store.InvestmentTransaction{{
		ID: fmt.Sprintf("itxn-%d", pk), SourceID: pk, AccountID: fmt.Sprintf("acct-%d", usdPK),
		SecurityID: new(fmt.Sprintf("sec-%d", acmePK)), Date: investDay, Action: "sell",
		Shares: new(int64(-4_500_000)), Amount: 40025, Currency: "USD",
	}}, fake.Rows.InvestmentTransactions)
}

func Test_import_stores_an_investment_transactions_note_as_its_memo_and_an_empty_note_as_null(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Note: "quarterly"})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "2.00", PostedDate: &investDay})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 2)
	assert.Equal(t, new("quarterly"), fake.Rows.InvestmentTransactions[0].Memo)
	assert.Nil(t, fake.Rows.InvestmentTransactions[1].Memo)
}

func Test_import_refuses_an_investment_value_quarry_cannot_read(t *testing.T) {
	t.Parallel()
	const prefix = `an investment transaction on 2026-03-01 in "Brokerage" `
	cases := []struct {
		name       string
		units      string
		amount     string
		commission string
		costBasis  string
		want       string
	}{
		{name: "shares beyond 6 decimals", units: "1.23456789", amount: "1.00", want: prefix + "has 1.23456789 shares, which has more than 6 decimal places"},
		{name: "shares too large", units: "1000000000000", amount: "1.00", want: prefix + "has 1000000000000 shares, which is too large for quarry's share counts"},
		{name: "shares not a number", units: "n/a", amount: "1.00", want: prefix + "has a share count that is not a number"},
		{name: "amount beyond 2 decimals", amount: "1.234", want: prefix + "has an amount of 1.234, which has more than 2 decimal places"},
		{name: "amount too large", amount: "10000000000000000", want: prefix + "has an amount of 10000000000000000, which is too large for quarry's amounts"},
		{name: "amount not a number", amount: "n/a", want: prefix + "has an amount that is not a number"},
		{name: "amount NULL", amount: "", want: prefix + "has no amount"},
		{name: "commission beyond 4 decimals", amount: "1.00", commission: "1.23456", want: prefix + "has a commission of 1.23456, which has more than 4 decimal places"},
		{name: "commission too large", amount: "1.00", commission: "10000000000000000", want: prefix + "has a commission of 10000000000000000, which is too large for quarry's amounts"},
		{name: "commission not a number", amount: "1.00", commission: "n/a", want: prefix + "has a commission that is not a number"},
		{name: "cost basis beyond 2 decimals", amount: "1.00", costBasis: "1.234", want: prefix + "has a cost basis of 1.234, which has more than 2 decimal places"},
		{name: "cost basis too large", amount: "1.00", costBasis: "10000000000000000", want: prefix + "has a cost basis of 10000000000000000, which is too large for quarry's amounts"},
		{name: "cost basis not a number", amount: "1.00", costBasis: "n/a", want: prefix + "has a cost basis that is not a number"},
		{name: "commission and cost basis both unreadable", amount: "1.00", commission: "n/a", costBasis: "n/a", want: prefix + "has a commission that is not a number"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			b.InvestmentTransaction(v9fixture.TransactionRow{
				Account: accountPK, Type: buyCode, PostedDate: &investDay, Units: c.units, Amount: c.amount, Commission: c.commission, CostBasis: c.costBasis,
			})

			reason, fake := importRefused(t, b)

			assert.Equal(t, c.want, reason)
			assert.Zero(t, fake.replaceCalls)
		})
	}
}

func Test_import_refuses_a_blob_share_count(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: buyCode, PostedDate: &investDay, Amount: "1.00", Units: "1"})
	dataPath := snapshotPath(t, b)
	setColumnBlob(t, dataPath, "ZTRANSACTION", "ZUNITS", pk, []byte{0x01, 0x02})

	reason, _ := importRefusedFrom(t, dataPath)

	assert.Equal(t, `an investment transaction on 2026-03-01 in "Brokerage" has a share count that is not a number`, reason)
}

func Test_import_snaps_float_residue_in_shares_to_the_nearest_millionth(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	investmentWithEntry(b, v9fixture.TransactionRow{
		Account: accountPK, Type: buyCode, PostedDate: &investDay, Amount: "1.00", Position: positionPK, Units: "0.30000000000000004",
	})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Equal(t, new(int64(300_000)), fake.Rows.InvestmentTransactions[0].Shares)
}

func Test_import_fails_on_shares_without_a_security(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		units string
		setup func(b *v9fixture.Builder, accountPK int64) int64
	}{
		{name: "no position", units: "2", setup: func(*v9fixture.Builder, int64) int64 { return 0 }},
		{name: "negative shares and no position", units: "-4", setup: func(*v9fixture.Builder, int64) int64 { return 0 }},
		{name: "position of a deleted security", units: "2", setup: func(b *v9fixture.Builder, accountPK int64) int64 {
			goneSecurityPK := b.Security(v9fixture.SecurityRow{Name: "Gone Inc", Ticker: "GONE", Currency: "CAD", Deleted: true})
			return b.Position(v9fixture.PositionRow{Account: accountPK, Security: goneSecurityPK})
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			b.InvestmentTransaction(v9fixture.TransactionRow{
				Account: accountPK, Type: buyCode, PostedDate: &investDay, Amount: "1.00", Position: c.setup(b, accountPK), Units: c.units,
			})

			reason, fake := importRefused(t, b)

			assert.Equal(t, `an investment transaction on 2026-03-01 in "Brokerage" has shares but no security`, reason)
			assert.Zero(t, fake.replaceCalls)
		})
	}
}

func Test_import_gives_a_row_with_zero_or_NULL_shares_and_no_position_no_security(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		units string
		want  *int64
	}{
		{name: "zero shares", units: "0", want: new(int64(0))},
		{name: "NULL shares", units: "", want: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, PostedDate: &investDay, Amount: "1.00", Units: c.units})

			fake, _ := importOK(t, b)

			require.Len(t, fake.Rows.InvestmentTransactions, 1)
			assert.Nil(t, fake.Rows.InvestmentTransactions[0].SecurityID)
			assert.Equal(t, c.want, fake.Rows.InvestmentTransactions[0].Shares)
		})
	}
}

func Test_import_fails_on_a_split_with_an_unreadable_ratio(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		numerator   string
		denominator string
		want        string
	}{
		{name: "NULL numerator", numerator: "", denominator: "12", want: "(none:12)"},
		{name: "NULL denominator", numerator: "1", denominator: "", want: "(1:none)"},
		{name: "zero numerator", numerator: "0", denominator: "12", want: "(0:12)"},
		{name: "zero denominator", numerator: "1", denominator: "0", want: "(1:0)"},
		{name: "unreadable numerator", numerator: "n/a", denominator: "12", want: "(n/a:12)"},
		{name: "unreadable denominator", numerator: "1", denominator: "n/a", want: "(1:n/a)"},
		{name: "numerator beyond 6 decimals", numerator: "1.23456789", denominator: "12", want: "(1.23456789:12)"},
		{name: "numerator too large", numerator: "1000000000000", denominator: "12", want: "(1000000000000:12)"},
		{name: "denominator too large", numerator: "1", denominator: "1000000000000", want: "(1:1000000000000)"},
		{name: "denominator beyond 6 decimals", numerator: "1", denominator: "1.23456789", want: "(1:1.23456789)"},
		{name: "negative numerator", numerator: "-1", denominator: "12", want: "(-1:12)"},
		{name: "negative denominator", numerator: "1", denominator: "-12", want: "(1:-12)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: newAcme(b)})
			b.InvestmentTransaction(v9fixture.TransactionRow{
				Account: accountPK, Type: new(int64(23)), PostedDate: &investDay, Amount: "0", Position: positionPK,
				Numerator: c.numerator, Denominator: c.denominator,
			})

			reason, fake := importRefused(t, b)

			assert.Equal(t, `a stock split on 2026-03-01 in "Brokerage" of "Acme Corp" has a ratio quarry cannot read `+c.want, reason)
			assert.Zero(t, fake.replaceCalls)
		})
	}
}

func Test_import_shows_a_REAL_or_blob_split_side_as_stored_in_its_refusal(t *testing.T) {
	t.Parallel()
	const prefix = `a stock split on 2026-03-01 in "Brokerage" of "Acme Corp" has a ratio quarry cannot read `
	cases := []struct {
		name   string
		update string
		want   string
	}{
		{name: "a_REAL_stored_zero_as_a_plain_zero", update: "UPDATE ZTRANSACTION SET ZNUMERATOR = 1.5, ZDENOMINATOR = 0.0 WHERE Z_PK = ?", want: prefix + "(1.5:0)"},
		{name: "a_blob_numerator_as_blob", update: "UPDATE ZTRANSACTION SET ZNUMERATOR = X'00FF41' WHERE Z_PK = ?", want: prefix + "(blob:12)"},
		{name: "a_blob_denominator_as_blob", update: "UPDATE ZTRANSACTION SET ZDENOMINATOR = X'00FF41' WHERE Z_PK = ?", want: prefix + "(1:blob)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: newAcme(b)})
			pk := b.InvestmentTransaction(v9fixture.TransactionRow{
				Account: accountPK, Type: new(int64(23)), PostedDate: &investDay, Amount: "0", Position: positionPK, Numerator: "1", Denominator: "12",
			})
			dataPath := snapshotPath(t, b)
			execOn(t, dataPath, c.update, pk)

			reason, _ := importRefusedFrom(t, dataPath)

			assert.Equal(t, c.want, reason)
		})
	}
}

func Test_import_names_no_security_in_the_refusal_of_a_split_without_an_imported_security(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		position func(b *v9fixture.Builder, accountPK int64) int64
	}{
		{name: "whose_security_is_not_imported", position: func(b *v9fixture.Builder, accountPK int64) int64 {
			goneSecurityPK := b.Security(v9fixture.SecurityRow{Name: "Gone Inc", Ticker: "GONE", Currency: "CAD", Deleted: true})
			return b.Position(v9fixture.PositionRow{Account: accountPK, Security: goneSecurityPK})
		}},
		{name: "with_no_security", position: func(*v9fixture.Builder, int64) int64 { return 0 }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			b.InvestmentTransaction(v9fixture.TransactionRow{
				Account: accountPK, Type: new(int64(23)), PostedDate: &investDay, Amount: "0", Position: c.position(b, accountPK), Numerator: "1", Denominator: "0",
			})

			reason, _ := importRefused(t, b)

			assert.Equal(t, `a stock split on 2026-03-01 in "Brokerage" has a ratio quarry cannot read (1:0)`, reason)
		})
	}
}

func Test_import_ignores_the_ratio_of_a_row_that_is_not_a_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, PostedDate: &investDay, Amount: "1.00", Numerator: "0", Denominator: "n/a"})

	fake, _ := importOK(t, b)

	assert.Len(t, fake.Rows.InvestmentTransactions, 1)
}

// cashRowOf returns the transactions row of the investment transaction at source pk, by its id.
func cashRowOf(tb testing.TB, fake *fakeStore, pk int64) store.Transaction {
	tb.Helper()
	id := fmt.Sprintf("txn-%d", pk)
	for _, txn := range fake.Rows.Transactions {
		if txn.ID == id {
			return txn
		}
	}
	require.Failf(tb, "no cash row", "transactions has no %s, only %v", id, transactionIDs(fake))
	return store.Transaction{}
}

func Test_import_gives_an_investment_transaction_with_an_amount_a_cash_row_carrying_its_fields(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	pk := investmentWithEntry(b, v9fixture.TransactionRow{
		Account: accountPK, Type: dividendCode, Amount: "-12.00", PostedDate: &investDay, EnteredDate: &investLater,
		Note: "quarterly", Status: new(int64(1)), ExcludeFromReports: new(int64(1)),
	})

	fake, _ := importOK(t, b)

	assert.Equal(t, store.Transaction{
		ID: fmt.Sprintf("txn-%d", pk), SourceID: pk, AccountID: fmt.Sprintf("acct-%d", accountPK),
		Date: investDay, PostedDate: &investDay, Amount: -1200, Currency: "CAD", Status: "cleared",
		ExcludedFromReports: true, Memo: new("quarterly"), InvestmentTransactionID: new(fmt.Sprintf("itxn-%d", pk)),
	}, cashRowOf(t, fake, pk))
}

func Test_import_dates_an_investment_cash_row_by_the_investments_posted_day_else_its_entered_day(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		posted     *time.Time
		wantDate   time.Time
		wantPosted *time.Time
	}{
		{name: "posted and entered", posted: &investDay, wantDate: investDay, wantPosted: &investDay},
		{name: "entered only", posted: nil, wantDate: investLater, wantPosted: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			pk := investmentWithEntry(b, v9fixture.TransactionRow{
				Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: c.posted, EnteredDate: &investLater,
			})

			fake, _ := importOK(t, b)

			cash := cashRowOf(t, fake, pk)
			assert.Equal(t, c.wantDate, cash.Date)
			assert.Equal(t, c.wantPosted, cash.PostedDate)
		})
	}
}

func Test_import_reads_an_investment_cash_rows_status_from_the_reconcile_status(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status *int64
		want   string
	}{
		{name: "NULL is uncleared", status: nil, want: "uncleared"},
		{name: "0 is uncleared", status: new(int64(0)), want: "uncleared"},
		{name: "1 is cleared", status: new(int64(1)), want: "cleared"},
		{name: "2 is reconciled", status: new(int64(2)), want: "reconciled"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			pk := investmentWithEntry(b, v9fixture.TransactionRow{
				Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay, Status: c.status,
			})

			fake, _ := importOK(t, b)

			assert.Equal(t, c.want, cashRowOf(t, fake, pk).Status)
		})
	}
}

func Test_import_refuses_an_investment_transaction_with_an_amount_and_an_unmapped_reconcile_status(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	investmentWithEntry(b, v9fixture.TransactionRow{
		Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay, Status: new(int64(3)),
	})

	reason, fake := importRefused(t, b)

	assert.Equal(t, `a transaction on 2026-03-01 in "Brokerage" has reconcile status 3, which quarry does not map yet`, reason)
	assert.Zero(t, fake.replaceCalls)
}

func Test_import_ignores_the_reconcile_status_of_an_investment_transaction_with_amount_zero(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	investmentWithEntry(b, v9fixture.TransactionRow{
		Account: newBrokerage(b), Type: dividendCode, Amount: "0", PostedDate: &investDay, Status: new(int64(3)),
	})

	fake, _ := importOK(t, b)

	assert.Len(t, fake.Rows.InvestmentTransactions, 1)
}

func Test_import_flags_an_investment_cash_row_excluded_from_reports_only_when_the_source_is(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		exclude *int64
		want    bool
	}{
		{name: "NULL", exclude: nil, want: false},
		{name: "0", exclude: new(int64(0)), want: false},
		{name: "1", exclude: new(int64(1)), want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			pk := investmentWithEntry(b, v9fixture.TransactionRow{
				Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay, ExcludeFromReports: c.exclude,
			})

			fake, _ := importOK(t, b)

			assert.Equal(t, c.want, cashRowOf(t, fake, pk).ExcludedFromReports)
		})
	}
}

func Test_import_gives_an_investment_cash_row_the_investments_note_as_memo_and_none_for_an_empty_note(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	notedPK := investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay, Note: "quarterly"})
	barePK := investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "13.00", PostedDate: &investDay})

	fake, _ := importOK(t, b)

	assert.Equal(t, new("quarterly"), cashRowOf(t, fake, notedPK).Memo)
	assert.Nil(t, cashRowOf(t, fake, barePK).Memo)
}

func Test_import_gives_no_cash_row_to_an_investment_transaction_with_amount_zero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		code int64
	}{
		{name: "add shares", code: 2},
		{name: "remove shares", code: 17},
		{name: "split", code: 23},
		{name: "reinvested dividend", code: 15},
		{name: "buy of 0.00", code: 3},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			controlPK := investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
			investmentWithEntry(b, v9fixture.TransactionRow{
				Account: accountPK, Type: &c.code, Amount: "0", PostedDate: &investDay, Units: "0", Numerator: "1", Denominator: "1",
			})

			fake, _ := importOK(t, b)

			assert.Len(t, fake.Rows.InvestmentTransactions, 2)
			assert.Equal(t, []string{fmt.Sprintf("txn-%d", controlPK)}, transactionIDs(fake))
		})
	}
}

func Test_import_gives_an_investment_transactions_entry_a_split_with_its_category_amount_and_memo(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	incomePK := b.Category(v9fixture.TagRow{Name: "Dividends", Type: new(int64(0))})
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "12.00", CategoryTag: incomePK, Note: "ACME Q1"})

	fake, _ := importOK(t, b)

	assert.Equal(t, []store.Split{{
		ID: fmt.Sprintf("split-%d", entryPK), SourceID: entryPK, TransactionID: fmt.Sprintf("txn-%d", pk),
		Amount: 1200, CategoryID: new(fmt.Sprintf("cat-%d", incomePK)), Memo: new("ACME Q1"),
	}}, fake.Rows.Splits)
}

func Test_import_gives_an_entry_less_investment_transaction_one_uncategorized_split_of_its_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay})

	fake, result := importOK(t, b)

	assert.Equal(t, []store.Split{{
		ID: fmt.Sprintf("split-itxn-%d", pk), SourceID: -pk, TransactionID: fmt.Sprintf("txn-%d", pk), Amount: 1200,
	}}, fake.Rows.Splits)
	assert.Zero(t, result.Validation.Splits.Mismatched)
}

func Test_import_gives_an_entry_less_investment_transaction_a_split_that_collides_with_no_entry_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := newChequing(b)
	investmentPK := b.InvestmentTransaction(v9fixture.TransactionRow{Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
	registerPK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &investDay})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: registerPK, Amount: "-5.00"})
	require.Equal(t, investmentPK, entryPK)

	fake, _ := importOK(t, b)

	ids := make(map[string]bool)
	sourceIDs := make(map[int64]bool)
	for _, split := range fake.Rows.Splits {
		ids[split.ID] = true
		sourceIDs[split.SourceID] = true
	}
	assert.Len(t, fake.Rows.Splits, 2)
	assert.Len(t, ids, 2)
	assert.Len(t, sourceIDs, 2)
}

func Test_import_still_fails_validation_for_a_register_transaction_with_no_entry(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	pk := b.Transaction(v9fixture.TransactionRow{
		Account: newChequing(b),
		Amount:  "12.00", PostedDate: &investDay,
	})

	result, fake := importFailingValidation(t, b)

	require.Len(t, result.Validation.Splits.Mismatched, 1)
	mismatch := result.Validation.Splits.Mismatched[0]
	assert.Equal(t, fmt.Sprintf("txn-%d", pk), mismatch.ID)
	assert.Equal(t, int64(1200), mismatch.Amount)
	assert.Zero(t, mismatch.SplitsTotal)
	assert.Zero(t, fake.replaceCalls)
}

func Test_import_gives_an_entry_less_investment_transaction_with_amount_zero_no_row_and_no_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.InvestmentTransaction(v9fixture.TransactionRow{Account: newBrokerage(b), Type: dividendCode, Amount: "0", PostedDate: &investDay})

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.Transactions)
	assert.Empty(t, fake.Rows.Splits)
}

func Test_import_gives_an_investment_cash_row_the_currency_of_its_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	usdPK := b.Account(v9fixture.AccountRow{Name: "US Brokerage", Type: "BROKERAGENORMAL", Currency: "USD", Active: true})
	pk := investmentWithEntry(b, v9fixture.TransactionRow{Account: usdPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})

	fake, _ := importOK(t, b)

	assert.Equal(t, "USD", cashRowOf(t, fake, pk).Currency)
}

// investmentTransferLeg adds an investment transaction in account whose one entry carries quickenID and the
// ZTRANSFER text link, returning the entry's Z_PK.
func investmentTransferLeg(b *v9fixture.Builder, account int64, amount string, quickenID int64, link string) int64 {
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: account, Type: dividendCode, Amount: amount, PostedDate: &investDay})
	return b.Entry(v9fixture.EntryRow{Parent: pk, Amount: amount, QuickenID: quickenID, Transfer: link})
}

func Test_import_pairs_an_investment_transfer_entry_with_its_counterpart_leg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		counterpart  v9fixture.AccountRow
		counterLeg   func(b *v9fixture.Builder, account int64, amount string, quickenID int64, link string) int64
		wantTransfer store.TransferCheck
	}{
		{
			name:         "a chequing register entry",
			counterpart:  v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true},
			counterLeg:   transferLeg,
			wantTransfer: store.TransferCheck{Paired: 1},
		},
		{
			name:         "an entry of another brokerage",
			counterpart:  v9fixture.AccountRow{Name: "Second Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true},
			counterLeg:   investmentTransferLeg,
			wantTransfer: store.TransferCheck{Paired: 1},
		},
		{
			name:         "a USD chequing register entry",
			counterpart:  v9fixture.AccountRow{Name: "US Chequing", Type: "CHECKING", Currency: "USD", Active: true},
			counterLeg:   transferLeg,
			wantTransfer: store.TransferCheck{Paired: 1, CrossCurrency: 1},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			brokeragePK := newBrokerage(b)
			counterpartPK := b.Account(c.counterpart)
			investmentLeg := investmentTransferLeg(b, brokeragePK, "50.00", 101, "102")
			counterLeg := c.counterLeg(b, counterpartPK, "-50.00", 102, "101")

			fake, result := importOK(t, b)

			assert.Equal(t, c.wantTransfer, result.Validation.Transfers)
			assert.Empty(t, result.Validation.Splits.Mismatched)
			assert.Equal(t, new(accountIDFor(counterpartPK)), splitByID(fake, splitIDFor(investmentLeg)).TransferAccountID)
			assert.Equal(t, new(accountIDFor(brokeragePK)), splitByID(fake, splitIDFor(counterLeg)).TransferAccountID)
		})
	}
}

func Test_import_keeps_an_investment_transfer_entry_named_for_an_account_as_a_one_sided_transfer(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := newChequing(b)
	leg := investmentTransferLeg(b, newBrokerage(b), "50.00", 101, "Chequing")

	_, result := importOK(t, b)

	require.Len(t, result.Validation.Transfers.OneSided, 1)
	oneSided := result.Validation.Transfers.OneSided[0]
	assert.Equal(t, leg, oneSided.SourceID)
	assert.Equal(t, new(accountIDFor(chequingPK)), oneSided.OtherAccountID)
}

func Test_import_reports_an_investment_transfer_entry_that_differs_from_its_transaction_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
	b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "10.00", QuickenID: 101, Transfer: "Chequing"})

	result, _ := importFailingValidation(t, b)

	require.Len(t, result.Validation.Splits.Mismatched, 1)
	assert.Equal(t, fmt.Sprintf("txn-%d", pk), result.Validation.Splits.Mismatched[0].ID)
	assert.Equal(t, int64(1000), result.Validation.Splits.Mismatched[0].SplitsTotal)
}

func Test_import_splits_an_investment_transaction_across_its_two_entries_with_no_synthetic_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
	first := b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "7.00"})
	second := b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "5.00"})

	fake, result := importOK(t, b)

	assert.Equal(t, []string{splitIDFor(first), splitIDFor(second)}, []string{fake.Rows.Splits[0].ID, fake.Rows.Splits[1].ID})
	assert.Len(t, fake.Rows.Splits, 2)
	assert.Empty(t, result.Validation.Splits.Mismatched)
}
