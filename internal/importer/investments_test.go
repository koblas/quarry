package importer_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	investDay    = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	investLater  = time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	buyCode      = new(int64(3))
	dividendCode = new(int64(10))
)

// newBrokerage adds the account every investment test imports into.
func newBrokerage(b *v9fixture.Builder) int64 {
	return b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
}

func importInvestments(t *testing.T, b *v9fixture.Builder) (*fakeStore, store.Result) {
	t.Helper()
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	return fake, result
}

func importInvestmentsRefused(t *testing.T, b *v9fixture.Builder) (string, *fakeStore) {
	t.Helper()
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	return importReason(t, err), fake
}

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

	fake, result := importInvestments(t, b)

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

			reason, fake := importInvestmentsRefused(t, b)

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

	reason, _ := importInvestmentsRefused(t, b)

	assert.Equal(t, `an investment transaction on 2026-03-01 in "Brokerage" has no action code`, reason)
}

func Test_import_refuses_an_investment_transaction_with_neither_a_posted_nor_an_entered_date(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00"})

	reason, _ := importInvestmentsRefused(t, b)

	assert.Equal(t, fmt.Sprintf(`an investment transaction in "Brokerage" (source id %d) has no date`, pk), reason)
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

			fake, _ := importInvestments(t, b)

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

			fake, _ := importInvestments(t, b)

			require.Len(t, fake.Rows.InvestmentTransactions, 1)
			assert.Equal(t, c.want, fake.Rows.InvestmentTransactions[0].Date)
		})
	}
}

func Test_import_leaves_a_deleted_investment_transaction_out(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	keptPK := investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay})
	b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "2.00", PostedDate: &investDay, Deleted: true})

	fake, _ := importInvestments(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Equal(t, keptPK, fake.Rows.InvestmentTransactions[0].SourceID)
}

func Test_import_leaves_out_an_investment_row_of_another_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	keptPK := investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay})
	b.InvestmentTransaction(v9fixture.TransactionRow{Entity: 999, Account: accountPK, Type: buyCode, Amount: "2.00", PostedDate: &investDay})

	fake, _ := importInvestments(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Equal(t, keptPK, fake.Rows.InvestmentTransactions[0].SourceID)
}

func Test_import_resolves_the_security_of_an_investment_transaction_through_its_position(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Position: positionPK, Units: "0"})

	fake, _ := importInvestments(t, b)

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

			fake, _ := importInvestments(t, b)

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
	bundle := b.WriteBundle(t, t.TempDir())
	execOn(t, bundle.DataPath, "INSERT INTO ZACCOUNT (Z_PK, ZNAME, ZTYPENAME, ZCURRENCY, ZACTIVE) VALUES (0, 'Zero', 'BROKERAGENORMAL', 'CAD', 1)")
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
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
	bundle := b.WriteBundle(t, t.TempDir())
	execOn(t, bundle.DataPath, "UPDATE ZPOSITION SET Z_PK = 0 WHERE Z_PK = ?", positionPK)
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
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

	fake, _ := importInvestments(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Nil(t, fake.Rows.InvestmentTransactions[0].SecurityID)
}

func Test_import_imports_investment_transactions_without_a_security_when_the_snapshot_has_no_position_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("Position")
	accountPK := newBrokerage(b)
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})

	fake, _ := importInvestments(t, b)

	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Nil(t, fake.Rows.InvestmentTransactions[0].SecurityID)
}

func Test_import_imports_no_investment_transactions_when_the_snapshot_has_no_investment_transaction_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("InvestmentTransaction")
	accountPK := newBrokerage(b)
	cashPK := b.Transaction(v9fixture.TransactionRow{Account: accountPK, Amount: "5.00", PostedDate: &investDay})
	b.Entry(v9fixture.EntryRow{Parent: cashPK, Amount: "5.00"})

	fake, result := importInvestments(t, b)

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

			fake, _ := importInvestments(t, b)

			require.Len(t, fake.Rows.InvestmentTransactions, 1)
			assert.Equal(t, c.want, fake.Rows.InvestmentTransactions[0].Commission)
		})
	}
}

func Test_import_sets_the_split_columns_only_on_a_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Numerator: "1", Denominator: "12"})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: new(int64(23)), Amount: "0", PostedDate: &investDay, Numerator: "1", Denominator: "12"})

	fake, _ := importInvestments(t, b)

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

	fake, _ := importInvestments(t, b)

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

	fake, _ := importInvestments(t, b)

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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			b.InvestmentTransaction(v9fixture.TransactionRow{
				Account: accountPK, Type: buyCode, PostedDate: &investDay, Units: c.units, Amount: c.amount, Commission: c.commission,
			})

			reason, fake := importInvestmentsRefused(t, b)

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
	bundle := b.WriteBundle(t, t.TempDir())
	setColumnBlob(t, bundle.DataPath, "ZTRANSACTION", "ZUNITS", pk, []byte{0x01, 0x02})

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `an investment transaction on 2026-03-01 in "Brokerage" has a share count that is not a number`, importReason(t, err))
}

func Test_import_refuses_an_undated_investment_transaction_before_any_other_fault(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Units: "n/a", Amount: "n/a"})

	reason, _ := importInvestmentsRefused(t, b)

	assert.Equal(t, fmt.Sprintf(`an investment transaction in "Brokerage" (source id %d) has no date`, pk), reason)
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

	fake, _ := importInvestments(t, b)

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

			reason, fake := importInvestmentsRefused(t, b)

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

			fake, _ := importInvestments(t, b)

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

			reason, fake := importInvestmentsRefused(t, b)

			assert.Equal(t, `a stock split on 2026-03-01 in "Brokerage" of "Acme Corp" has a ratio quarry cannot read `+c.want, reason)
			assert.Zero(t, fake.replaceCalls)
		})
	}
}

func Test_import_shows_a_REAL_stored_zero_split_side_as_a_plain_zero(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: newAcme(b)})
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{
		Account: accountPK, Type: new(int64(23)), PostedDate: &investDay, Amount: "0", Position: positionPK, Numerator: "1", Denominator: "1",
	})
	bundle := b.WriteBundle(t, t.TempDir())
	execOn(t, bundle.DataPath, "UPDATE ZTRANSACTION SET ZNUMERATOR = 1.5, ZDENOMINATOR = 0.0 WHERE Z_PK = ?", pk)

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a stock split on 2026-03-01 in "Brokerage" of "Acme Corp" has a ratio quarry cannot read (1.5:0)`, importReason(t, err))
}

func Test_import_shows_a_blob_split_side_as_blob(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: newAcme(b)})
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{
		Account: accountPK, Type: new(int64(23)), PostedDate: &investDay, Amount: "0", Position: positionPK, Numerator: "1", Denominator: "12",
	})
	bundle := b.WriteBundle(t, t.TempDir())
	execOn(t, bundle.DataPath, "UPDATE ZTRANSACTION SET ZNUMERATOR = X'00FF41' WHERE Z_PK = ?", pk)

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a stock split on 2026-03-01 in "Brokerage" of "Acme Corp" has a ratio quarry cannot read (blob:12)`, importReason(t, err))
}

func Test_import_shows_a_blob_split_denominator_as_blob(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: newAcme(b)})
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{
		Account: accountPK, Type: new(int64(23)), PostedDate: &investDay, Amount: "0", Position: positionPK, Numerator: "1", Denominator: "12",
	})
	bundle := b.WriteBundle(t, t.TempDir())
	execOn(t, bundle.DataPath, "UPDATE ZTRANSACTION SET ZDENOMINATOR = X'00FF41' WHERE Z_PK = ?", pk)

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a stock split on 2026-03-01 in "Brokerage" of "Acme Corp" has a ratio quarry cannot read (1:blob)`, importReason(t, err))
}

func Test_import_names_no_security_in_the_refusal_of_a_split_whose_security_is_not_imported(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	goneSecurityPK := b.Security(v9fixture.SecurityRow{Name: "Gone Inc", Ticker: "GONE", Currency: "CAD", Deleted: true})
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: goneSecurityPK})
	b.InvestmentTransaction(v9fixture.TransactionRow{
		Account: accountPK, Type: new(int64(23)), PostedDate: &investDay, Amount: "0", Position: positionPK, Numerator: "1", Denominator: "0",
	})

	reason, _ := importInvestmentsRefused(t, b)

	assert.Equal(t, `a stock split on 2026-03-01 in "Brokerage" has a ratio quarry cannot read (1:0)`, reason)
}

func Test_import_names_no_security_in_the_refusal_of_a_split_with_no_security(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	b.InvestmentTransaction(v9fixture.TransactionRow{
		Account: accountPK, Type: new(int64(23)), PostedDate: &investDay, Amount: "0", Numerator: "1", Denominator: "0",
	})

	reason, _ := importInvestmentsRefused(t, b)

	assert.Equal(t, `a stock split on 2026-03-01 in "Brokerage" has a ratio quarry cannot read (1:0)`, reason)
}

func Test_import_ignores_the_ratio_of_a_row_that_is_not_a_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, PostedDate: &investDay, Amount: "1.00", Numerator: "0", Denominator: "n/a"})

	fake, _ := importInvestments(t, b)

	assert.Len(t, fake.Rows.InvestmentTransactions, 1)
}
