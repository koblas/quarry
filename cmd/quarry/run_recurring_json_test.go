package main

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recurringIDName is an {id, name} pair of the recurring document.
type recurringIDName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// recurringPriceChangeJSON is one entry of a series' "price_changes".
type recurringPriceChangeJSON struct {
	Date      string  `json:"date"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	ChangePct float64 `json:"change_pct"`
}

// recurringSeriesJSON is one entry of the document's "series".
type recurringSeriesJSON struct {
	Payee        string                     `json:"payee"`
	PayeeKey     *string                    `json:"payee_key"`
	Payees       []recurringIDName          `json:"payees"`
	Currency     string                     `json:"currency"`
	Cadence      string                     `json:"cadence"`
	Amount       string                     `json:"amount"`
	FirstAmount  string                     `json:"first_amount"`
	PerYear      *string                    `json:"per_year"`
	FirstCharge  string                     `json:"first_charge"`
	LastCharge   string                     `json:"last_charge"`
	ChargeCount  int                        `json:"charge_count"`
	State        string                     `json:"state"`
	New          bool                       `json:"new"`
	Accounts     []recurringIDName          `json:"accounts"`
	PriceChanges []recurringPriceChangeJSON `json:"price_changes"`
}

// recurringTotalJSON is one entry of the document's "totals".
type recurringTotalJSON struct {
	Currency string `json:"currency"`
	PerYear  string `json:"per_year"`
}

// recurringJSONDoc is quarry recurring --json's stdout.
type recurringJSONDoc struct {
	Since         string                `json:"since"`
	Until         string                `json:"until"`
	AccountFilter []recurringIDName     `json:"account_filter"`
	Series        []recurringSeriesJSON `json:"series"`
	Totals        []recurringTotalJSON  `json:"totals"`
	Warnings      []string              `json:"warnings"`
}

// decodeRecurringJSON reads stdout as the recurring document, refusing keys the document does not rule.
func decodeRecurringJSON(t *testing.T, stdout string) recurringJSONDoc {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	decoder.DisallowUnknownFields()
	var doc recurringJSONDoc

	require.NoError(t, decoder.Decode(&doc), stdout)
	return doc
}

// monthlySeries is one grocery charge of payee on the 12th of each month from (year, month) on, one per amount in cents.
func monthlySeries(payee string, year int, month time.Month, cents ...int64) []chargeTxn {
	charges := make([]chargeTxn, len(cents))
	for i, c := range cents {
		charges[i] = groceryCharge(payee, day(year, month+time.Month(i), 12), c)
	}
	return charges
}

// inUSD moves charges to the USD account acct-usd, under ids that do not collide with the CAD originals.
func inUSD(charges []chargeTxn) []chargeTxn {
	moved := slices.Clone(charges)
	for i := range moved {
		moved[i].id = "usd-" + moved[i].id
		moved[i].account = "acct-usd"
		moved[i].currency = "USD"
	}
	return moved
}

func Test_run_recurring_json_returns_the_series_document(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	charges := monthlySeries("Netflix.com", 2026, time.February, slices.Concat(slices.Repeat([]int64{999}, 4), slices.Repeat([]int64{1199}, 4))...)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring", "--since", "2000", "--json"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringJSONDoc{
		Since:         "2000-01-01",
		Until:         "2026-09-29",
		AccountFilter: []recurringIDName{},
		Series: []recurringSeriesJSON{{
			Payee:       "Netflix.com",
			PayeeKey:    new("netflix-com"),
			Payees:      []recurringIDName{{ID: "payee-Netflix.com", Name: "Netflix.com"}},
			Currency:    "CAD",
			Cadence:     "monthly",
			Amount:      "11.99",
			FirstAmount: "9.99",
			PerYear:     new("143.88"),
			FirstCharge: "2026-02-12",
			LastCharge:  "2026-09-12",
			ChargeCount: 8,
			State:       "active",
			New:         true,
			Accounts:    []recurringIDName{{ID: "acct-cad", Name: "Chequing"}},
			PriceChanges: []recurringPriceChangeJSON{
				{Date: "2026-06-12", From: "9.99", To: "11.99", ChangePct: 20.0},
			},
		}},
		Totals:   []recurringTotalJSON{{Currency: "CAD", PerYear: "143.88"}},
		Warnings: []string{},
	}, decodeRecurringJSON(t, stdout.String()))
}

func Test_run_recurring_merges_payees_differing_in_store_numbers_and_splits_currencies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	charges := slices.Concat(
		monthlySeries("NETFLIX.COM 1234", 2026, time.April, 1500, 1500, 1500),
		monthlySeries("Netflix.com", 2026, time.July, 1500, 1500, 1500),
		inUSD(monthlySeries("Netflix.com", 2026, time.June, 1200, 1200, 1200, 1200)),
	)
	accounts := []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}
	replaceStore(t, home, chargeRows(accounts, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring", "--since", "2000", "--json"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	doc := decodeRecurringJSON(t, stdout.String())
	require.Len(t, doc.Series, 2)
	cad, usd := doc.Series[0], doc.Series[1]
	assert.Equal(t, "CAD", cad.Currency)
	assert.Equal(t, "Netflix.com", cad.Payee)
	assert.Equal(t, 6, cad.ChargeCount)
	assert.Equal(t, []recurringIDName{
		{ID: "payee-NETFLIX.COM 1234", Name: "NETFLIX.COM 1234"},
		{ID: "payee-Netflix.com", Name: "Netflix.com"},
	}, cad.Payees)
	assert.Equal(t, "USD", usd.Currency)
	assert.Equal(t, 4, usd.ChargeCount)
	assert.Equal(t, []recurringIDName{{ID: "payee-Netflix.com", Name: "Netflix.com"}}, usd.Payees)
	assert.Equal(t, []recurringTotalJSON{{Currency: "CAD", PerYear: "180.00"}, {Currency: "USD", PerYear: "144.00"}}, doc.Totals)
}

func Test_run_recurring_json_marks_new_only_for_a_series_first_charged_inside_the_window(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	charges := slices.Concat(
		monthlySeries("Netflix.com", 2026, time.February, slices.Repeat([]int64{1199}, 8)...),
		monthlySeries("Spotify", 2026, time.July, slices.Repeat([]int64{1099}, 3)...),
	)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring", "--since", "2026-05-01", "--json"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	isNew := map[string]bool{}
	for _, s := range decodeRecurringJSON(t, stdout.String()).Series {
		isNew[s.Payee] = s.New
	}
	assert.Equal(t, map[string]bool{"Netflix.com": false, "Spotify": true}, isNew)
}
