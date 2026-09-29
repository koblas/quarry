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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, renderAccounts(store.AccountList{Accounts: c.accounts}))
		})
	}
}

func Test_accountStatus(t *testing.T) {
	cases := []struct {
		name           string
		closed, active bool
		want           string
	}{
		{name: "open and active is blank", closed: false, active: true, want: ""},
		{name: "open and not active is inactive", closed: false, active: false, want: "inactive"},
		{name: "closed and active is closed", closed: true, active: true, want: "closed"},
		{name: "closed and not active is closed, not inactive", closed: true, active: false, want: "closed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, accountStatus(c.closed, c.active))
		})
	}
}
