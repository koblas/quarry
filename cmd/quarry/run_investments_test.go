package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sync_imports_securities_and_their_prices(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	barePK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Ticker: "", Currency: "USD"})
	noCurrencyPK := b.Security(v9fixture.SecurityRow{Name: "Plain Co", Ticker: "PLN"})

	day1 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day1, ClosingPrice: "12.5"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day2, ClosingPrice: "0"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day3, ClosingPrice: ""})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: barePK, QuoteDate: &day1, ClosingPrice: "12.3456785"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: noCurrencyPK, QuoteDate: nil, ClosingPrice: "5"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	storePath := filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")
	db, err := duckdb.OpenReadOnly(t.Context(), storePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	secID := func(pk int64) string { return fmt.Sprintf("sec-%d", pk) }

	assert.Equal(t, map[string]string{
		secID(acmePK):       "Acme Corp|ACME|CAD",
		secID(barePK):       "Bare Fund|NULL|USD",
		secID(noCurrencyPK): "Plain Co|PLN|NULL",
	}, stringMap(t, db, `SELECT id, name || '|' || COALESCE(ticker, 'NULL') || '|' || COALESCE(currency, 'NULL') FROM securities`))
	assert.Equal(t, map[string]string{
		secID(acmePK) + " 2026-03-15": "12.500000",
		secID(acmePK) + " 2026-03-16": "0.000000",
		secID(barePK) + " 2026-03-15": "12.345678",
	}, stringMap(t, db, `SELECT security_id || ' ' || CAST(date AS VARCHAR), CAST(price AS VARCHAR) FROM prices`))
}

func Test_run_sync_records_security_and_price_counts_in_import_runs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	barePK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Currency: "USD"})
	day1 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day1, ClosingPrice: "12.5"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &day2, ClosingPrice: "13"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: barePK, QuoteDate: &day1, ClosingPrice: "4"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	storePath := filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")
	db, err := duckdb.OpenReadOnly(t.Context(), storePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{"counts": "2 3"},
		stringMap(t, db, `SELECT 'counts', concat_ws(' ', securities_rows, prices_rows) FROM import_runs`))
}
