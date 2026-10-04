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

func importSecurities(t *testing.T, b *v9fixture.Builder) (*fakeStore, store.Result) {
	t.Helper()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	return fake, result
}

func Test_import_maps_a_security_as_quicken_recorded_it(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})

	fake, _ := importSecurities(t, b)

	assert.Equal(t, []store.Security{{
		ID: fmt.Sprintf("sec-%d", acmePK), SourceID: acmePK, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD"),
	}}, fake.Rows.Securities)
}

func Test_import_stores_a_missing_ticker_as_null(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		update string
	}{
		{name: "NULL ticker", update: "UPDATE ZSECURITY SET ZTICKER = NULL WHERE Z_PK = ?"},
		{name: "empty ticker", update: "UPDATE ZSECURITY SET ZTICKER = '' WHERE Z_PK = ?"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
			pk := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Ticker: "ACME", Currency: "USD"})
			bundle := b.WriteBundle(t, t.TempDir())
			execOn(t, bundle.DataPath, c.update, pk)
			fake := &fakeStore{}

			_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

			require.NoError(t, err)
			require.Len(t, fake.Rows.Securities, 1)
			assert.Nil(t, fake.Rows.Securities[0].Ticker)
		})
	}
}

func Test_import_stores_a_missing_currency_as_null(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		update string
	}{
		{name: "NULL currency", update: "UPDATE ZSECURITY SET ZCURRENCY = NULL WHERE Z_PK = ?"},
		{name: "empty currency", update: "UPDATE ZSECURITY SET ZCURRENCY = '' WHERE Z_PK = ?"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
			pk := b.Security(v9fixture.SecurityRow{Name: "Plain Co", Ticker: "PLN", Currency: "USD"})
			bundle := b.WriteBundle(t, t.TempDir())
			execOn(t, bundle.DataPath, c.update, pk)
			fake := &fakeStore{}

			_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

			require.NoError(t, err)
			require.Len(t, fake.Rows.Securities, 1)
			assert.Nil(t, fake.Rows.Securities[0].Currency)
		})
	}
}

func Test_import_leaves_a_deleted_security_out(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	keptPK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	b.Security(v9fixture.SecurityRow{Name: "Gone Inc", Ticker: "GONE", Currency: "CAD", Deleted: true})

	fake, result := importSecurities(t, b)

	require.Len(t, fake.Rows.Securities, 1)
	assert.Equal(t, keptPK, fake.Rows.Securities[0].SourceID)
	assert.Equal(t, 1, result.Counts.Securities)
}

func Test_import_leaves_out_a_security_row_of_another_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	keptPK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	b.Security(v9fixture.SecurityRow{Entity: 999, Name: "Other Kind", Ticker: "OTH", Currency: "CAD"})

	fake, _ := importSecurities(t, b)

	require.Len(t, fake.Rows.Securities, 1)
	assert.Equal(t, keptPK, fake.Rows.Securities[0].SourceID)
}

func Test_import_resolves_the_security_entities_by_name_from_z_primarykey(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithEntity("Security", 9101).WithEntity("SecurityQuote", 9102)
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day, ClosingPrice: "12.5"})

	fake, _ := importSecurities(t, b)

	assert.Len(t, fake.Rows.Securities, 1)
	assert.Len(t, fake.Rows.Prices, 1)
}

func Test_import_imports_no_securities_and_no_prices_when_the_snapshot_has_no_security_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("Security")
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day, ClosingPrice: "12.5"})

	fake, result := importSecurities(t, b)

	assert.Empty(t, fake.Rows.Securities)
	assert.Empty(t, fake.Rows.Prices)
	assert.Zero(t, result.Counts.Securities)
	assert.Zero(t, result.Counts.Prices)
}

func Test_import_refuses_a_security_with_no_name(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value string
	}{
		{name: "NULL name", value: "NULL"},
		{name: "empty name", value: "''"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
			b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
			pk := b.Security(v9fixture.SecurityRow{Name: "Placeholder", Ticker: "NONAME", Currency: "CAD"})
			bundle := b.WriteBundle(t, t.TempDir())
			execOn(t, bundle.DataPath, "UPDATE ZSECURITY SET ZNAME = "+c.value+" WHERE Z_PK = ?", pk)
			fake := &fakeStore{}

			_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

			assert.Equal(t, fmt.Sprintf("a security (source id %d) has no name", pk), importReason(t, err))
			assert.Zero(t, fake.replaceCalls)
		})
	}
}

func Test_import_counts_a_second_security_with_no_name_as_one_more(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	firstPK := b.Security(v9fixture.SecurityRow{Ticker: "NONAME1", Currency: "CAD"})
	b.Security(v9fixture.SecurityRow{Ticker: "NONAME2", Currency: "CAD"})

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: b.WriteBundle(t, t.TempDir()).DataPath})

	assert.Equal(t, fmt.Sprintf("a security (source id %d) has no name (and 1 more)", firstPK), importReason(t, err))
}

func Test_import_ignores_a_deleted_security_with_no_name(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Security(v9fixture.SecurityRow{Ticker: "GONE", Currency: "CAD", Deleted: true})

	fake, result := importSecurities(t, b)

	assert.Empty(t, fake.Rows.Securities)
	assert.Zero(t, result.Counts.Securities)
}

func Test_import_reports_a_security_with_no_name_ahead_of_its_unreadable_quotes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		price string
	}{
		{name: "price that is not a number", price: "abc"},
		{name: "price too large", price: "1000000000000"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
			pk := b.Security(v9fixture.SecurityRow{Ticker: "NONAME", Currency: "CAD"})
			day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
			b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: pk, QuoteDate: &day, ClosingPrice: c.price})
			bundle := b.WriteBundle(t, t.TempDir())

			_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

			assert.Equal(t, fmt.Sprintf("a security (source id %d) has no name", pk), importReason(t, err))
		})
	}
}

func Test_import_reports_a_security_with_no_name_once_when_a_transaction_holds_its_shares(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	pk := b.Security(v9fixture.SecurityRow{Ticker: "NONAME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: accountPK, Security: pk})
	b.InvestmentTransaction(v9fixture.TransactionRow{
		Account: accountPK, Type: buyCode, PostedDate: &investDay, Amount: "1.00", Position: positionPK, Units: "2",
	})

	reason, _ := importInvestmentsRefused(t, b)

	assert.Equal(t, fmt.Sprintf("a security (source id %d) has no name", pk), reason)
}
