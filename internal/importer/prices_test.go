package importer_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
)

// priceLine is one price as "<security id> <date> <millionths>".
func priceLine(securityPK int64, day time.Time, millionths int64) string {
	return fmt.Sprintf("sec-%d %s %d", securityPK, day.Format(time.DateOnly), millionths)
}

func priceLines(fake *fakeStore) []string {
	out := make([]string, len(fake.Rows.Prices))
	for i, p := range fake.Rows.Prices {
		out[i] = fmt.Sprintf("%s %s %d", p.SecurityID, p.Date.Format(time.DateOnly), p.Price)
	}
	return out
}

func Test_import_keeps_the_highest_source_id_quote_of_a_day_even_when_its_price_is_lower(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acmePK := newAcme(b)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "20"})
	keptPK := b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "10"})
	newChequing(b)

	fake, result := importOK(t, b)

	assert.Equal(t, []string{priceLine(acmePK, priceDay1, 10_000_000)}, priceLines(fake))
	assert.Equal(t, keptPK, fake.Rows.Prices[0].SourceID)
	assert.Equal(t, 1, result.Counts.Prices)
}

func Test_import_treats_two_quotes_at_different_times_of_one_utc_day_as_a_duplicate(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acmePK := newAcme(b)
	morning := priceDay1.Add(3 * time.Hour)
	evening := priceDay1.Add(20 * time.Hour)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &morning, ClosingPrice: "20"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &evening, ClosingPrice: "10"})
	newChequing(b)

	fake, _ := importOK(t, b)

	assert.Equal(t, []string{priceLine(acmePK, priceDay1, 10_000_000)}, priceLines(fake))
}

func Test_import_keeps_one_price_per_security_for_the_same_day(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acmePK := newAcme(b)
	barePK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Currency: "USD"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "20"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: barePK, QuoteDate: &priceDay1, ClosingPrice: "10"})
	newChequing(b)

	fake, _ := importOK(t, b)

	assert.Equal(t, []string{priceLine(acmePK, priceDay1, 20_000_000), priceLine(barePK, priceDay1, 10_000_000)}, priceLines(fake))
}

func Test_import_orders_prices_by_security_then_day(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acmePK := newAcme(b)
	barePK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Currency: "USD"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: barePK, QuoteDate: &priceDay1, ClosingPrice: "3"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay2, ClosingPrice: "2"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "1"})
	newChequing(b)

	fake, _ := importOK(t, b)

	assert.Equal(t, []string{
		priceLine(acmePK, priceDay1, 1_000_000), priceLine(acmePK, priceDay2, 2_000_000), priceLine(barePK, priceDay1, 3_000_000),
	}, priceLines(fake))
}

func Test_import_keeps_a_valid_quote_over_another_quote_that_does_not_count(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		other func(acmePK int64) v9fixture.SecurityQuoteRow
	}{
		{name: "when_a_higher_source_id_quote_has_no_price", other: func(acmePK int64) v9fixture.SecurityQuoteRow {
			return v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: ""}
		}},
		{name: "when_a_higher_source_id_quote_is_deleted", other: func(acmePK int64) v9fixture.SecurityQuoteRow {
			return v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "10", Deleted: true}
		}},
		{name: "when_the_other_is_a_quote_row_of_another_entity", other: func(acmePK int64) v9fixture.SecurityQuoteRow {
			return v9fixture.SecurityQuoteRow{Entity: 999, Security: acmePK, QuoteDate: &priceDay2, ClosingPrice: "10"}
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acmePK := newAcme(b)
			b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "20"})
			b.SecurityQuote(c.other(acmePK))
			newChequing(b)

			fake, _ := importOK(t, b)

			assert.Equal(t, []string{priceLine(acmePK, priceDay1, 20_000_000)}, priceLines(fake))
		})
	}
}

func Test_import_leaves_out_a_quote_it_cannot_place(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		quote func(b *v9fixture.Builder)
	}{
		{name: "with_no_date", quote: func(b *v9fixture.Builder) {
			b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: newAcme(b), QuoteDate: nil, ClosingPrice: "20"})
		}},
		{name: "with_no_security", quote: func(b *v9fixture.Builder) {
			newAcme(b)
			b.SecurityQuote(v9fixture.SecurityQuoteRow{QuoteDate: &priceDay1, ClosingPrice: "20"})
		}},
		{name: "of_a_deleted_security_even_when_unreadable", quote: func(b *v9fixture.Builder) {
			gonePK := b.Security(v9fixture.SecurityRow{Name: "Gone Inc", Ticker: "GONE", Currency: "CAD", Deleted: true})
			b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: gonePK, QuoteDate: &priceDay1, ClosingPrice: "not a price"})
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			c.quote(b)
			newChequing(b)

			fake, _ := importOK(t, b)

			assert.Empty(t, fake.Rows.Prices)
		})
	}
}

func Test_import_keeps_a_zero_price(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acmePK := newAcme(b)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "0"})
	newChequing(b)

	fake, _ := importOK(t, b)

	assert.Equal(t, []string{priceLine(acmePK, priceDay1, 0)}, priceLines(fake))
}

func Test_import_imports_securities_without_prices_when_the_snapshot_has_no_quote_entity(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("SecurityQuote")
	acmePK := newAcme(b)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "20"})
	newChequing(b)

	fake, result := importOK(t, b)

	assert.Len(t, fake.Rows.Securities, 1)
	assert.Empty(t, fake.Rows.Prices)
	assert.Zero(t, result.Counts.Prices)
}

func Test_import_refuses_a_price_quarry_cannot_read(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		price string
		want  string
	}{
		{name: "not_a_number", price: "n/a", want: `a price of "Acme Corp" on 2026-03-15 is not a number`},
		{name: "too_large_for_quarry", price: "1000000000000", want: `a price of "Acme Corp" on 2026-03-15 is 1000000000000, which is too large for quarry's prices`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acmePK := newAcme(b)
			b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: c.price})
			newChequing(b)

			reason, fake := importRefused(t, b)

			assert.Equal(t, c.want, reason)
			assert.Zero(t, fake.replaceCalls)
		})
	}
}

func Test_import_refuses_an_unreadable_price_that_a_later_quote_of_the_day_supersedes(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acmePK := newAcme(b)
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "n/a"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "10"})
	newChequing(b)

	reason, fake := importRefused(t, b)

	assert.Equal(t, `a price of "Acme Corp" on 2026-03-15 is not a number`, reason)
	assert.Zero(t, fake.replaceCalls)
}
