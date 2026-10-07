// White-box: renderAccounts and accountStatus are unexported layout rules
// whose column widths and status forms are best driven directly.
package cli

import (
	"math/big"
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func balanceRow(name, accountType, currency string, cents int64, closed, active bool) store.AccountBalance {
	return store.AccountBalance{
		Name: name, Type: accountType, Currency: currency, Closed: closed, Active: active,
		Balance: big.NewInt(cents), Cash: big.NewInt(cents),
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
			name: "the specification's example, a long balance widening Balance",
			accounts: []store.AccountBalance{
				balanceRow("Chequing", "chequing", "CAD", 1234567, false, true),
				balanceRow("RRSP", "retirement", "CAD", 0, false, true),
				balanceRow("US Chequing", "chequing", "USD", 831000, false, true),
				balanceRow("Visa Infinite", "credit_card", "CAD", -120417, true, true),
			},
			want: "" +
				"Account        Type         Currency    Balance  Status\n" +
				"Chequing       chequing     CAD       12,345.67\n" +
				"RRSP           retirement   CAD            0.00  unclassified\n" +
				"US Chequing    chequing     USD        8,310.00\n" +
				"Visa Infinite  credit_card  CAD       -1,204.17  closed\n",
		},
		{
			name:     "header only when there are no accounts",
			accounts: nil,
			want:     "Account  Type  Currency  Balance  Status\n",
		},
		{
			name:     "Balance header right-aligned over a wider amount",
			accounts: []store.AccountBalance{balanceRow("Chequing", "chequing", "CAD", 1234567, false, true)},
			want: "" +
				"Account   Type      Currency    Balance  Status\n" +
				"Chequing  chequing  CAD       12,345.67\n",
		},
		{
			name: "an investment account's Balance cell is cash plus holdings value, widening the column",
			accounts: []store.AccountBalance{
				{Name: "Brokerage", Type: "brokerage", Currency: "CAD", Active: true, Cash: big.NewInt(1000000), HoldingsValue: big.NewInt(20500000), Balance: big.NewInt(21500000)},
				balanceRow("Chequing", "chequing", "CAD", 1234567, false, true),
			},
			want: "" +
				"Account    Type       Currency     Balance  Status\n" +
				"Brokerage  brokerage  CAD       215,000.00  unclassified\n" +
				"Chequing   chequing   CAD        12,345.67\n",
		},
		{
			name: "a balance past 64 bits renders in exact cents, grouped",
			accounts: []store.AccountBalance{
				{Name: "Brokerage", Type: "brokerage", Currency: "CAD", Active: true, Cash: big.NewInt(0), HoldingsValue: cents("99999999999999999800000000"), Balance: cents("99999999999999999800000000")},
			},
			want: "" +
				"Account    Type       Currency                             Balance  Status\n" +
				"Brokerage  brokerage  CAD       999,999,999,999,999,998,000,000.00  unclassified\n",
		},
		{
			name: "a non-ASCII name padded by rune count",
			accounts: []store.AccountBalance{
				balanceRow("Chequing", "chequing", "CAD", 0, false, true),
				balanceRow("Épargnes", "savings", "CAD", 0, false, false),
			},
			want: "" +
				"Account   Type      Currency  Balance  Status\n" +
				"Chequing  chequing  CAD          0.00\n" +
				"Épargnes  savings   CAD          0.00  inactive\n",
		},
		{
			name: "linked tracking after not in reports in the Status cell",
			accounts: []store.AccountBalance{
				withNotInReports(withLinkedTracking(balanceRow("Netskope 401(k)", "retirement", "USD", 0, true, true))),
				withLinkedTracking(balanceRow("Brokerage", "brokerage", "USD", 0, false, true)),
			},
			want: "" +
				"Account          Type        Currency  Balance  Status\n" +
				"Netskope 401(k)  retirement  USD          0.00  closed, not in reports, linked tracking, unclassified\n" +
				"Brokerage        brokerage   USD          0.00  linked tracking, unclassified\n",
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

func withCells(a store.AccountBalance, cad, usd *big.Int) store.AccountBalance {
	a.BalanceCAD, a.BalanceUSD = cad, usd
	return a
}

func Test_renderAccounts_adds_the_reporting_currency_column_after_Balance(t *testing.T) {
	chequing := withCells(balanceRow("Chequing", "chequing", "CAD", 1234567, false, true), big.NewInt(1234567), big.NewInt(987654))
	usChequing := withCells(balanceRow("US Chequing", "chequing", "USD", 831000, false, true), big.NewInt(1038750), big.NewInt(831000))
	brokerage := balanceRow("Brokerage", "brokerage", "USD", 0, false, true)
	cases := []struct {
		name    string
		listing report.AccountListing
		want    string
	}{
		{
			name:    "CAD: each cell converted, the CAD cell identity, an unconverted cell reading no rate",
			listing: listingIn(money.CAD, chequing, usChequing, brokerage),
			want: "" +
				"Account      Type       Currency    Balance     In CAD  Status\n" +
				"Chequing     chequing   CAD       12,345.67  12,345.67\n" +
				"US Chequing  chequing   USD        8,310.00  10,387.50\n" +
				"Brokerage    brokerage  USD            0.00    no rate  unclassified\n",
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
				withCells(balanceRow("US Chequing", "chequing", "USD", 831000, false, true), nil, big.NewInt(831000))),
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
			listing: listingIn(money.CAD, withCells(balanceRow("Old US", "chequing", "USD", 100, true, true), big.NewInt(125), big.NewInt(100))),
			want: "" +
				"Account  Type      Currency  Balance  In CAD  Status\n" +
				"Old US   chequing  USD          1.00    1.25  closed\n",
		},
		{
			name:    "CAD: a no rate cell is padded to the column so a closed Status starts two spaces after it",
			listing: listingIn(money.CAD, chequing, balanceRow("Brokerage", "brokerage", "USD", 0, true, true)),
			want: "" +
				"Account    Type       Currency    Balance     In CAD  Status\n" +
				"Chequing   chequing   CAD       12,345.67  12,345.67\n" +
				"Brokerage  brokerage  USD            0.00    no rate  closed, unclassified\n",
		},
		{
			name:    "CAD: a no rate cell is padded to the column so a not in reports Status starts two spaces after it",
			listing: listingIn(money.CAD, chequing, withNotInReports(brokerage)),
			want: "" +
				"Account    Type       Currency    Balance     In CAD  Status\n" +
				"Chequing   chequing   CAD       12,345.67  12,345.67\n" +
				"Brokerage  brokerage  USD            0.00    no rate  not in reports, unclassified\n",
		},
		{
			name:    "USD: a no rate cell is padded to the narrower column so Status starts two spaces after it",
			listing: listingIn(money.USD, chequing, withNotInReports(brokerage)),
			want: "" +
				"Account    Type       Currency    Balance    In USD  Status\n" +
				"Chequing   chequing   CAD       12,345.67  9,876.54\n" +
				"Brokerage  brokerage  USD            0.00   no rate  not in reports, unclassified\n",
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

func Test_renderAccounts_says_no_rate_for_every_cell_a_missing_rate_leaves_unconverted(t *testing.T) {
	zeroBrokerage := balanceRow("Brokerage", "brokerage", "USD", 0, false, true)
	missingRate := withCells(balanceRow("US Chequing", "chequing", "USD", 831000, false, true), nil, big.NewInt(831000))

	got := renderAccounts(listingIn(money.CAD, zeroBrokerage, missingRate))

	assert.Equal(t, ""+
		"Account      Type       Currency   Balance   In CAD  Status\n"+
		"Brokerage    brokerage  USD           0.00  no rate  unclassified\n"+
		"US Chequing  chequing   USD       8,310.00  no rate\n", got)
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
			assert.Equal(t, c.want, accountStatus(c.account, report.Classification{}))
		})
	}
}

func Test_accountStatus_ends_with_registered_or_unclassified(t *testing.T) {
	classification := report.Classification{Registered: []string{"acct-1"}, NonRegistered: []string{"acct-2"}}
	cases := []struct {
		name    string
		account store.Account
		want    string
	}{
		{name: "a listed registered account", account: store.Account{ID: "acct-1", Type: "brokerage", Active: true}, want: "registered"},
		{name: "a listed registered non-investment account", account: store.Account{ID: "acct-1", Type: "chequing", Active: true}, want: "registered"},
		{name: "a listed non-registered account gets nothing", account: store.Account{ID: "acct-2", Type: "brokerage", Active: true}, want: ""},
		{name: "an unlisted investment account is unclassified", account: store.Account{ID: "acct-3", Type: "retirement", Active: true}, want: "unclassified"},
		{name: "an unlisted non-investment account gets nothing", account: store.Account{ID: "acct-3", Type: "chequing", Active: true}, want: ""},
		{
			name:    "registered follows every other state",
			account: store.Account{ID: "acct-1", Type: "brokerage", Closed: true, NotInReports: true, LinkedTracking: true},
			want:    "closed, not in reports, linked tracking, registered",
		},
		{
			name:    "unclassified follows every other state",
			account: store.Account{ID: "acct-3", Type: "brokerage", NotInReports: true, LinkedTracking: true},
			want:    "inactive, not in reports, linked tracking, unclassified",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, accountStatus(c.account, classification))
		})
	}
}

func Test_renderAccounts_escapes_an_account_name_and_pads_after_it(t *testing.T) {
	got := renderAccounts(report.AccountListing{Accounts: []store.AccountBalance{
		balanceRow("\tAccount Not Synced", "chequing", "CAD", 100, false, true),
		balanceRow("Chequing", "chequing", "CAD", 100, false, true),
	}})

	assert.Equal(t, ""+
		"Account               Type      Currency  Balance  Status\n"+
		"\\tAccount Not Synced  chequing  CAD          1.00\n"+
		"Chequing              chequing  CAD          1.00\n", got)
}
