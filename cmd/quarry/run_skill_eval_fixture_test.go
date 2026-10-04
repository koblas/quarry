// Package main: main cannot be imported, so these tests live beside the unexported run and
// the test helpers the cmd/quarry tests share.
package main

import (
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// skillEvalStore builds the store the skill's recipes and use cases are evaluated on under home.
// Every split is dated on or before 2026-09-29, and the one rate is dated 2026-03-01.
func skillEvalStore(t *testing.T, home string) {
	t.Helper()
	replaceStoreWithRates(t, home, skillEvalRows(), usdRate(day(2026, time.March, 1), 1_300_000))
}

// skillEvalRows is the rows of skillEvalStore.
func skillEvalRows() store.Rows {
	accounts := []store.Account{
		chequingAccount("acct-cad", 1),
		usdChequingAccount("acct-usd", 2),
		{ID: "acct-savings", SourceID: 3, Name: "Savings", Type: "savings", Currency: "CAD", Active: true},
	}
	var txns []chargeTxn
	add := func(more ...chargeTxn) { txns = append(txns, more...) }

	add(monthlySeries("Netflix", 2026, time.February, slices.Concat(slices.Repeat([]int64{999}, 4), slices.Repeat([]int64{1199}, 4))...)...)
	add(hardwareHistory()...)
	add(bigHardware())
	add(inUSD(monthlySeries("Spotify", 2025, time.November, slices.Repeat([]int64{1099}, 7)...))...)
	add(salary("salary-2026-03", day(2026, time.March, 1), 500000), salary("salary-2026-04", day(2026, time.April, 1), 500000))
	add(skillTransferPair()...)
	add(
		skillCharge("food", "Corner Deli", "cat-food", day(2026, time.May, 5), -1100),
		skillCharge("organic", "Organic Co-op", "cat-organic", day(2026, time.May, 6), -2200),
		skillCharge("foodies", "Foodie Hub", "cat-foodies", day(2026, time.May, 7), -4400),
	)
	add(
		skillCharge("grocery-2021", "", "cat-groceries", day(2021, time.December, 31), -1050),
		skillCharge("grocery-2022", "", "cat-groceries", day(2022, time.March, 15), -2250),
		skillCharge("grocery-2023", "", "cat-groceries", day(2023, time.March, 15), -3375),
		skillCharge("grocery-2024", "", "cat-groceries", day(2024, time.March, 15), -4125),
		skillCharge("grocery-2025", "", "cat-groceries", day(2025, time.March, 15), -5550),
	)
	for i, date := range []time.Time{day(2024, time.June, 9), day(2024, time.June, 10), day(2024, time.June, 20), day(2024, time.June, 21)} {
		spent, interest := []int64{100, 200, 400, 800}[i], []int64{300, 250, 350, 600}[i]
		add(
			skillCharge("bound-spend-"+date.Format(time.DateOnly), "", "cat-groceries", date, -spent),
			skillOn(skillCharge("bound-income-"+date.Format(time.DateOnly), "", "cat-interest", date, interest), "acct-savings", "CAD"),
		)
	}
	add(
		skillCharge("gas-1", "Gas Bar", "cat-fuel", day(2026, time.June, 1), -3700),
		skillCharge("gas-2", "Gas Bar", "cat-fuel", day(2026, time.June, 3), -3700),
		skillCharge("uncategorized-out", "", "", day(2026, time.July, 1), -6400),
		skillCharge("uncategorized-in", "", "", day(2026, time.July, 2), 12800),
		skillOn(skillCharge("us-client-1", "US Client", "cat-salary", day(2026, time.February, 15), 100000), "acct-usd", "USD"),
		skillOn(skillCharge("us-client-2", "US Client", "cat-salary", day(2026, time.April, 15), 10000), "acct-usd", "USD"),
	)

	rows := chargeRows(accounts, txns...)
	rows.Categories = append(rows.Categories,
		store.Category{ID: "cat-food", SourceID: 3, Name: "Food", FullPath: "Food", Kind: "expense"},
		store.Category{ID: "cat-organic", SourceID: 4, Name: "Organic", FullPath: "Food:Groceries:Organic", Kind: "expense"},
		store.Category{ID: "cat-foodies", SourceID: 5, Name: "Foodies", FullPath: "Foodies", Kind: "expense"},
		store.Category{ID: "cat-salary", SourceID: 6, Name: "Salary", FullPath: "Income:Salary", Kind: "income"},
		store.Category{ID: "cat-interest", SourceID: 7, Name: "Interest", FullPath: "Income:Interest", Kind: "income"},
	)
	rows.ReferencedCategoryIDs = append(rows.ReferencedCategoryIDs, "cat-food", "cat-organic", "cat-foodies", "cat-salary", "cat-interest")
	slices.Sort(rows.ReferencedCategoryIDs)
	rows.Transfers = []store.Transfer{{ID: "xfer-sweep", FromSplitID: "split-sweep-out-0", ToSplitID: new("split-sweep-in-0")}}
	return rows
}

// skillCharge is a one-split transaction on acct-cad in CAD; "" payee or category means none.
func skillCharge(id, payee, category string, date time.Time, cents int64) chargeTxn {
	return chargeTxn{
		id: id, account: "acct-cad", payee: payee, currency: "CAD", day: date,
		splits: []chargeSplit{{category: category, cents: cents}},
	}
}

// skillOn is charge moved to account, which holds currency.
func skillOn(charge chargeTxn, account, currency string) chargeTxn {
	charge.account, charge.currency = account, currency
	return charge
}

// skillTransferPair is the "Savings Sweep" of 500.00 from acct-cad to acct-savings, both legs uncategorized.
func skillTransferPair() []chargeTxn {
	out := skillCharge("sweep-out", "Savings Sweep", "", day(2026, time.April, 15), -50000)
	in := skillCharge("sweep-in", "Savings Sweep", "", day(2026, time.April, 15), 50000)
	return []chargeTxn{out, skillOn(in, "acct-savings", "CAD")}
}
