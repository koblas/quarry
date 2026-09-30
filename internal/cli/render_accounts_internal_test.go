// White-box: renderAccounts and accountStatus are unexported layout rules
// whose column widths and status forms are best driven directly.
package cli

import (
	"testing"

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
			assert.Equal(t, c.want, renderAccounts(store.AccountList{Accounts: c.accounts}))
		})
	}
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
