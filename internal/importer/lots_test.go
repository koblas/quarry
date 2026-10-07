package importer_test

import (
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
)

const reasonNoLots = "the snapshot has investment transactions but no Quicken lots to check their share counts against"

// quickenShares renders each reference entry as "account/security=millionths", in the order Import returned them.
func quickenShares(fake *fakeStore) []string {
	out := make([]string, len(fake.Rows.QuickenShares))
	for i, s := range fake.Rows.QuickenShares {
		out[i] = fmt.Sprintf("%s/%s=%s", s.AccountID, s.SecurityID, s.Millionths)
	}
	return out
}

func holding(accountPK, securityPK int64, millionths string) string {
	return fmt.Sprintf("acct-%d/sec-%d=%s", accountPK, securityPK, millionths)
}

func Test_import_sums_each_holdings_lot_units_into_one_share_count(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	betaPK := b.Security(v9fixture.SecurityRow{Name: "Beta Corp"})
	gammaPK := b.Security(v9fixture.SecurityRow{Name: "Gamma Corp"})
	retirementPK := b.Account(v9fixture.AccountRow{Name: "Retirement", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	acmePosition := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	betaPosition := b.Position(v9fixture.PositionRow{Account: accountPK, Security: betaPK})
	gammaPosition := b.Position(v9fixture.PositionRow{Account: accountPK, Security: gammaPK})
	retirementPosition := b.Position(v9fixture.PositionRow{Account: retirementPK, Security: acmePK})
	b.Lot(v9fixture.LotRow{Position: retirementPosition, LatestUnits: "1"})
	b.Lot(v9fixture.LotRow{Position: gammaPosition, LatestUnits: "7"})
	b.Lot(v9fixture.LotRow{Position: acmePosition, LatestUnits: "1.5"})
	b.Lot(v9fixture.LotRow{Position: betaPosition, LatestUnits: "10"})
	b.Lot(v9fixture.LotRow{Position: acmePosition, LatestUnits: "2.25"})

	fake, _ := importOK(t, b)

	assert.Equal(t, []string{
		holding(accountPK, acmePK, "3750000"),
		holding(accountPK, betaPK, "10000000"),
		holding(accountPK, gammaPK, "7000000"),
		holding(retirementPK, acmePK, "1000000"),
	}, quickenShares(fake))
}

func Test_import_sums_the_lots_of_two_positions_in_one_holding(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	first := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	second := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	b.Lot(v9fixture.LotRow{Position: first, LatestUnits: "4"})
	b.Lot(v9fixture.LotRow{Position: second, LatestUnits: "5"})

	fake, _ := importOK(t, b)

	assert.Equal(t, []string{holding(accountPK, acmePK, "9000000")}, quickenShares(fake))
}

func Test_import_lists_a_holding_whose_lots_total_zero(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	position := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	b.Lot(v9fixture.LotRow{Position: position, LatestUnits: "0"})

	fake, _ := importOK(t, b)

	assert.Equal(t, []string{holding(accountPK, acmePK, "0")}, quickenShares(fake))
}

func Test_import_lists_no_holding_for_a_position_without_lots(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.QuickenShares)
}

func Test_import_adds_lot_units_beyond_the_range_of_a_64_bit_count(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	position := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	for range 10 {
		b.Lot(v9fixture.LotRow{Position: position, LatestUnits: "999999999999"})
	}

	fake, _ := importOK(t, b)

	assert.Equal(t, []string{holding(accountPK, acmePK, "9999999999990000000")}, quickenShares(fake))
}

func Test_import_snaps_a_lots_float_residue_to_its_millionth(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	position := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	b.Lot(v9fixture.LotRow{Position: position, LatestUnits: "1.0000000001"})

	fake, _ := importOK(t, b)

	assert.Equal(t, []string{holding(accountPK, acmePK, "1000000")}, quickenShares(fake))
}

func Test_import_leaves_out_a_lot_of_another_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	position := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	b.Lot(v9fixture.LotRow{Position: position, LatestUnits: "1"})
	b.Lot(v9fixture.LotRow{Entity: 999, Position: position, LatestUnits: "5"})

	fake, _ := importOK(t, b)

	assert.Equal(t, []string{holding(accountPK, acmePK, "1000000")}, quickenShares(fake))
}

// Each lot has NULL units, which would refuse the import if it counted.
func Test_import_leaves_out_a_lot_that_does_not_count(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(b *v9fixture.Builder, accountPK, acmePK int64)
	}{
		{name: "deleted lot", setup: func(b *v9fixture.Builder, accountPK, acmePK int64) {
			position := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
			b.Lot(v9fixture.LotRow{Position: position, Deleted: true})
		}},
		{name: "lot of a deleted position", setup: func(b *v9fixture.Builder, accountPK, acmePK int64) {
			position := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK, Deleted: true})
			b.Lot(v9fixture.LotRow{Position: position})
		}},
		{name: "lot of a position of another entity", setup: func(b *v9fixture.Builder, accountPK, acmePK int64) {
			position := b.Position(v9fixture.PositionRow{Entity: 999, Account: accountPK, Security: acmePK})
			b.Lot(v9fixture.LotRow{Position: position})
		}},
		{name: "lot of a position in a deleted account", setup: func(b *v9fixture.Builder, _, acmePK int64) {
			deletedPK := b.Account(v9fixture.AccountRow{Name: "Old Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Deleted: true})
			position := b.Position(v9fixture.PositionRow{Account: deletedPK, Security: acmePK})
			b.Lot(v9fixture.LotRow{Position: position})
		}},
		{name: "lot of a deleted security", setup: func(b *v9fixture.Builder, accountPK, _ int64) {
			gonePK := b.Security(v9fixture.SecurityRow{Name: "Gone Inc", Deleted: true})
			position := b.Position(v9fixture.PositionRow{Account: accountPK, Security: gonePK})
			b.Lot(v9fixture.LotRow{Position: position})
		}},
		{name: "lot with no position", setup: func(b *v9fixture.Builder, _, _ int64) {
			b.Lot(v9fixture.LotRow{})
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			acmePK := newAcme(b)
			c.setup(b, accountPK, acmePK)

			fake, _ := importOK(t, b)

			assert.Empty(t, fake.Rows.QuickenShares)
		})
	}
}

// A position at Z_PK 0 exists, so a NULL position read as 0 would resolve to it.
func Test_import_leaves_out_a_lot_with_no_position_though_a_position_has_pk_zero(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	b.Lot(v9fixture.LotRow{LatestUnits: "3"})
	dataPath := snapshotPath(t, b)
	execOn(t, dataPath, "UPDATE ZPOSITION SET Z_PK = 0 WHERE Z_PK = ?", positionPK)

	fake, _ := importOKFrom(t, dataPath)

	assert.Empty(t, fake.Rows.QuickenShares)
}

func Test_import_refuses_a_counting_lot_with_an_unreadable_share_count(t *testing.T) {
	t.Parallel()
	const prefix = `a lot of "Acme Corp" in "Brokerage" `
	cases := []struct {
		name  string
		units string
		want  string
	}{
		{name: "NULL", units: "", want: prefix + "has no share count"},
		{name: "text", units: "n/a", want: prefix + "has a share count that is not a number"},
		{name: "beyond 6 decimals", units: "1.23456789", want: prefix + "has 1.23456789 shares, which has more than 6 decimal places"},
		{name: "too large", units: "1000000000000", want: prefix + "has 1000000000000 shares, which is too large for quarry's share counts"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			acmePK := newAcme(b)
			position := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
			b.Lot(v9fixture.LotRow{Position: position, LatestUnits: c.units})

			reason, fake := importRefused(t, b)

			assert.Equal(t, c.want, reason)
			assert.Zero(t, fake.replaceCalls)
		})
	}
}

func Test_import_refuses_a_counting_lot_whose_share_count_is_a_blob(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	position := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	b.Lot(v9fixture.LotRow{Position: position, LatestUnits: "1"})
	dataPath := snapshotPath(t, b)
	execOn(t, dataPath, "UPDATE ZLOT SET ZLATESTUNITS = X'00'")

	reason, _ := importRefusedFrom(t, dataPath)

	assert.Equal(t, `a lot of "Acme Corp" in "Brokerage" has a share count that is not a number`, reason)
}

func Test_import_refuses_an_unreadable_lot_count_without_a_second_refusal_for_a_nameless_security(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	namelessPK := b.Security(v9fixture.SecurityRow{Ticker: "NONE"})
	position := b.Position(v9fixture.PositionRow{Account: accountPK, Security: namelessPK})
	b.Lot(v9fixture.LotRow{Position: position})

	reason, _ := importRefused(t, b)

	assert.Equal(t, fmt.Sprintf("a security (source id %d) has no name", namelessPK), reason)
}

func Test_import_refuses_investment_transactions_when_the_snapshot_has_no_lot_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("Lot")
	accountPK := newBrokerage(b)
	b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})

	reason, fake := importRefused(t, b)

	assert.Equal(t, reasonNoLots, reason)
	assert.Zero(t, fake.replaceCalls)
}

// oneGoodAndOneBadInvestmentRow imports a dividend plus a buy with the given units and amount.
func oneGoodAndOneBadInvestmentRow(b *v9fixture.Builder, units, amount string) {
	accountPK := newBrokerage(b)
	b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
	b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Units: units, Amount: amount, PostedDate: &investDay})
}

func Test_import_reports_the_missing_lot_entity_ahead_of_an_unreadable_investment_row(t *testing.T) {
	t.Parallel()
	const prefix = `an investment transaction on 2026-03-01 in "Brokerage" `
	cases := []struct {
		name          string
		units, amount string
		wantRow       string
	}{
		{name: "shares not a number", units: "n/a", amount: "1.00", wantRow: prefix + "has a share count that is not a number"},
		{name: "amount NULL", units: "1", amount: "", wantRow: prefix + "has no amount"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			withoutLot := v9fixture.NewBuilder().WithoutEntity("Lot")
			oneGoodAndOneBadInvestmentRow(withoutLot, c.units, c.amount)
			withLot := v9fixture.NewBuilder()
			oneGoodAndOneBadInvestmentRow(withLot, c.units, c.amount)

			noLotReason, _ := importRefused(t, withoutLot)
			controlReason, _ := importRefused(t, withLot)

			assert.Equal(t, reasonNoLots, noLotReason)
			assert.Equal(t, c.wantRow, controlReason)
		})
	}
}

func Test_import_reports_the_unreadable_row_when_no_lot_entity_and_no_investment_row_maps(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("Lot")
	accountPK := newBrokerage(b)
	b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Units: "1", PostedDate: &investDay})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `an investment transaction on 2026-03-01 in "Brokerage" has no amount`, reason)
}

func Test_import_accepts_a_snapshot_with_no_lot_entity_and_no_imported_investment_transactions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(b *v9fixture.Builder)
	}{
		{name: "no investment rows", setup: func(*v9fixture.Builder) {}},
		{name: "investment row in a deleted account", setup: func(b *v9fixture.Builder) {
			deletedPK := b.Account(v9fixture.AccountRow{Name: "Old Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Deleted: true})
			b.InvestmentTransaction(v9fixture.TransactionRow{Account: deletedPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder().WithoutEntity("Lot")
			newBrokerage(b)
			c.setup(b)

			fake, _ := importOK(t, b)

			assert.Empty(t, fake.Rows.QuickenShares)
		})
	}
}
