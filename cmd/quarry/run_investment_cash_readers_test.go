package main

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// monthlyOn12th is the 12th of the month i months after October 2025.
func monthlyOn12th(i int) time.Time {
	return time.Date(2025, time.October+time.Month(i), 12, 0, 0, 0, 0, time.UTC)
}

// marginInterestChargesFixture's Brokerage holds one margin-interest charge per amount, dated by when(i).
func marginInterestChargesFixture(when func(i int) time.Time, amounts ...string) *v9fixture.Builder {
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	marginPK := b.Category(v9fixture.TagRow{Name: "Margin Interest", Type: new(int64(categoryKindExpense))})
	for i, amount := range amounts {
		investmentCash(b, when(i), investmentCodeMarginInterest, marginPK, inAccount(brokeragePK, amount))
	}
	return b
}

func Test_run_anomalies_lists_a_margin_interest_charge_against_its_categorys_earlier_charges(t *testing.T) {
	amounts := []string{"-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-20.00", "-150.00"}
	b := marginInterestChargesFixture(func(i int) time.Time {
		if i < 10 {
			return time.Date(2025, time.January+time.Month(i), 3, 0, 0, 0, 0, time.UTC)
		}
		return time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC)
	}, amounts...)
	syncedHome(t, b)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "1 charge checked",
		[]string{"2026-03-02", "Brokerage (CAD)", "(no payee)", "Margin Interest", "150.00", "20.00", "7.5x", "category, 10 earlier"}),
		stdout.String())
}

func Test_run_anomalies_counts_a_margin_interest_charge_of_100_or_more_with_no_history_as_not_judged(t *testing.T) {
	b := marginInterestChargesFixture(func(i int) time.Time {
		return time.Date(2026, time.March, 1+i, 0, 0, 0, 0, time.UTC)
	}, "-20.00", "-30.00", "-150.00")
	syncedHome(t, b)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		"3 charges checked; 1 had too little history to judge"), stdout.String())
}

// marginInterestAndNetflixFixture is twelve monthly margin-interest charges in Brokerage, with no payee,
// beside twelve monthly Netflix.com charges in Chequing.
func marginInterestAndNetflixFixture() *v9fixture.Builder {
	amounts := make([]string, 12)
	for i := range amounts {
		amounts[i] = "-20.00"
	}
	b := marginInterestChargesFixture(monthlyOn12th, amounts...)
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	netflixPK := b.Payee(v9fixture.PayeeRow{Name: "Netflix.com"})
	subscriptionsPK := b.Category(v9fixture.TagRow{Name: "Subscriptions", Type: new(int64(categoryKindExpense))})
	for i := range 12 {
		day := monthlyOn12th(i)
		pk := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-20.99", PostedDate: &day, Payee: netflixPK})
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "-20.99", CategoryTag: subscriptionsPK})
	}
	return b
}

func Test_run_recurring_finds_no_series_in_investment_rows_that_have_no_payee(t *testing.T) {
	syncedHome(t, marginInterestAndNetflixFixture())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		[]string{"Netflix.com", "CAD", "month", "20.99", "251.88", "2025-10-12", "2026-09-12", "active", ""},
		[]string{"Total", "CAD", "", "", "251.88", "", "", "", ""}),
		stdout.String())
}

func Test_run_spend_by_payee_puts_margin_interest_in_the_no_payee_bucket(t *testing.T) {
	syncedHome(t, marginInterestAndNetflixFixture())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"spend", "--by", "payee"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const row = "%-11s  %-8s  %6s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Payee", "Currency", "Spent")+
		fmt.Sprintf(row, "Netflix.com", "CAD", "188.91")+
		fmt.Sprintf(row, "(no payee)", "CAD", "180.00")+
		fmt.Sprintf(row, "Total", "CAD", "368.91"),
		stdout.String())
}
