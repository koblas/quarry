// White-box: renderAccounts and accountStatus are unexported layout rules
// whose column widths and status forms are best driven directly.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func balanceRow(name, accountType, currency string, cents *int64, closed, active bool) store.AccountBalance {
	return store.AccountBalance{
		Name: name, Type: accountType, Currency: currency, Closed: closed, Active: active,
		Balance: cents,
	}
}

func withNotInReports(a store.AccountBalance) store.AccountBalance {
	a.NotInReports = true
	return a
}

func withLinkedTracking(a store.AccountBalance) store.AccountBalance {
	a.LinkedTracking = true
	return a
}

func Test_renderAccounts(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.AccountBalance
		want     string
	}{
		{
			name: "the specification's example, not imported widening Balance",
			accounts: []store.AccountBalance{
				balanceRow("Chequing", "chequing", "CAD", new(int64(1234567)), false, true),
				balanceRow("RRSP", "retirement", "CAD", nil, false, true),
				balanceRow("US Chequing", "chequing", "USD", new(int64(831000)), false, true),
				balanceRow("Visa Infinite", "credit_card", "CAD", new(int64(-120417)), true, true),
			},
			want: "" +
				"Account        Type         Currency       Balance  Status\n" +
				"Chequing       chequing     CAD          12,345.67\n" +
				"RRSP           retirement   CAD       not imported\n" +
				"US Chequing    chequing     USD           8,310.00\n" +
				"Visa Infinite  credit_card  CAD          -1,204.17  closed\n",
		},
		{
			name:     "header only when there are no accounts",
			accounts: nil,
			want:     "Account  Type  Currency  Balance  Status\n",
		},
		{
			name:     "Balance header right-aligned over a wider amount",
			accounts: []store.AccountBalance{balanceRow("Chequing", "chequing", "CAD", new(int64(1234567)), false, true)},
			want: "" +
				"Account   Type      Currency    Balance  Status\n" +
				"Chequing  chequing  CAD       12,345.67\n",
		},
		{
			name: "a non-ASCII name padded by rune count",
			accounts: []store.AccountBalance{
				balanceRow("Chequing", "chequing", "CAD", new(int64(0)), false, true),
				balanceRow("Épargnes", "savings", "CAD", new(int64(0)), false, false),
			},
			want: "" +
				"Account   Type      Currency  Balance  Status\n" +
				"Chequing  chequing  CAD          0.00\n" +
				"Épargnes  savings   CAD          0.00  inactive\n",
		},
		{
			name: "linked tracking after not in reports in the Status cell",
			accounts: []store.AccountBalance{
				withNotInReports(withLinkedTracking(balanceRow("Netskope 401(k)", "retirement", "USD", nil, true, true))),
				withLinkedTracking(balanceRow("Brokerage", "brokerage", "USD", nil, false, true)),
			},
			want: "" +
				"Account          Type        Currency       Balance  Status\n" +
				"Netskope 401(k)  retirement  USD       not imported  closed, not in reports, linked tracking\n" +
				"Brokerage        brokerage   USD       not imported  linked tracking\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, renderAccounts(report.AccountListing{Accounts: c.accounts}))
		})
	}
}

func listingIn(currency money.Currency, accounts ...store.AccountBalance) report.AccountListing {
	return report.AccountListing{Accounts: accounts, Currency: currency}
}

func withCells(a store.AccountBalance, cad, usd *int64) store.AccountBalance {
	a.BalanceCAD, a.BalanceUSD = cad, usd
	return a
}

func Test_renderAccounts_adds_the_reporting_currency_column_after_Balance(t *testing.T) {
	chequing := withCells(balanceRow("Chequing", "chequing", "CAD", new(int64(1234567)), false, true), new(int64(1234567)), new(int64(987654)))
	usChequing := withCells(balanceRow("US Chequing", "chequing", "USD", new(int64(831000)), false, true), new(int64(1038750)), new(int64(831000)))
	brokerage := balanceRow("Brokerage", "brokerage", "USD", nil, false, true)
	cases := []struct {
		name    string
		listing report.AccountListing
		want    string
	}{
		{
			name:    "CAD: each cell converted, the CAD cell identity, a not imported cell blank",
			listing: listingIn(money.CAD, chequing, usChequing, brokerage),
			want: "" +
				"Account      Type       Currency       Balance     In CAD  Status\n" +
				"Chequing     chequing   CAD          12,345.67  12,345.67\n" +
				"US Chequing  chequing   USD           8,310.00  10,387.50\n" +
				"Brokerage    brokerage  USD       not imported\n",
		},
		{
			name:    "USD: the header names the reporting currency",
			listing: listingIn(money.USD, chequing, usChequing),
			want: "" +
				"Account      Type      Currency    Balance    In USD  Status\n" +
				"Chequing     chequing  CAD       12,345.67  9,876.54\n" +
				"US Chequing  chequing  USD        8,310.00  8,310.00\n",
		},
		{
			name: "an imported balance no rate converts reads no rate, and its width sets the column",
			listing: listingIn(money.CAD,
				withCells(chequing, chequing.BalanceCAD, nil),
				withCells(balanceRow("US Chequing", "chequing", "USD", new(int64(831000)), false, true), nil, new(int64(831000)))),
			want: "" +
				"Account      Type      Currency    Balance     In CAD  Status\n" +
				"Chequing     chequing  CAD       12,345.67  12,345.67\n" +
				"US Chequing  chequing  USD        8,310.00    no rate\n",
		},
		{
			name:    "the Status cell follows the column after the usual gap",
			listing: listingIn(money.CAD, withNotInReports(usChequing)),
			want: "" +
				"Account      Type      Currency   Balance     In CAD  Status\n" +
				"US Chequing  chequing  USD       8,310.00  10,387.50  not in reports\n",
		},
		{
			name:    "a closed account converts like any other",
			listing: listingIn(money.CAD, withCells(balanceRow("Old US", "chequing", "USD", new(int64(100)), true, true), new(int64(125)), new(int64(100)))),
			want: "" +
				"Account  Type      Currency  Balance  In CAD  Status\n" +
				"Old US   chequing  USD          1.00    1.25  closed\n",
		},
		{
			name:    "no accounts keeps the column in the header",
			listing: listingIn(money.USD),
			want:    "Account  Type  Currency  Balance  In USD  Status\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, renderAccounts(c.listing))
		})
	}
}

func Test_renderAccounts_leaves_a_not_imported_cell_blank_and_says_no_rate_for_a_missing_rate(t *testing.T) {
	notImported := balanceRow("Brokerage", "brokerage", "USD", nil, false, true)
	missingRate := withCells(balanceRow("US Chequing", "chequing", "USD", new(int64(831000)), false, true), nil, new(int64(831000)))

	got := renderAccounts(listingIn(money.CAD, notImported, missingRate))

	assert.Equal(t, ""+
		"Account      Type       Currency       Balance   In CAD  Status\n"+
		"Brokerage    brokerage  USD       not imported\n"+
		"US Chequing  chequing   USD           8,310.00  no rate\n", got)
}

func Test_accountStatus(t *testing.T) {
	cases := []struct {
		name    string
		account store.Account
		want    string
	}{
		{name: "open and active is blank", account: store.Account{Active: true}, want: ""},
		{name: "open and not active is inactive", account: store.Account{}, want: "inactive"},
		{name: "closed and active is closed", account: store.Account{Closed: true, Active: true}, want: "closed"},
		{name: "closed and not active is closed, not inactive", account: store.Account{Closed: true}, want: "closed"},
		{name: "open, active, not in reports", account: store.Account{Active: true, NotInReports: true}, want: "not in reports"},
		{name: "inactive and not in reports", account: store.Account{NotInReports: true}, want: "inactive, not in reports"},
		{name: "closed and not in reports", account: store.Account{Closed: true, Active: true, NotInReports: true}, want: "closed, not in reports"},
		{name: "open, active, linked tracking", account: store.Account{Active: true, LinkedTracking: true}, want: "linked tracking"},
		{name: "inactive and linked tracking", account: store.Account{LinkedTracking: true}, want: "inactive, linked tracking"},
		{name: "closed and linked tracking", account: store.Account{Closed: true, Active: true, LinkedTracking: true}, want: "closed, linked tracking"},
		{name: "not in reports and linked tracking", account: store.Account{Active: true, NotInReports: true, LinkedTracking: true}, want: "not in reports, linked tracking"},
		{name: "closed, not in reports and linked tracking", account: store.Account{Closed: true, Active: true, NotInReports: true, LinkedTracking: true}, want: "closed, not in reports, linked tracking"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, accountStatus(c.account))
		})
	}
}
