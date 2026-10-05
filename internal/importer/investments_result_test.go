package importer_test

import (
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_import_returns_the_securities_and_investment_transactions_it_wrote(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	acmePK := newAcme(b)
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: acmePK})
	investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: buyCode, Amount: "1.00", PostedDate: &investDay, Position: positionPK, Units: "0"})

	fake, result := importInvestments(t, b)

	require.Len(t, fake.Rows.Securities, 1)
	require.Len(t, fake.Rows.InvestmentTransactions, 1)
	assert.Equal(t, store.Investments{Securities: fake.Rows.Securities, Transactions: fake.Rows.InvestmentTransactions}, result.Investments)
}
