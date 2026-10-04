package cli_test

import (
	"bytes"
	"regexp"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	holdingsAccountHelp = "list only the account with this name or id; repeat for more"
	zetaID              = "acct-zeta"
	alphaID             = "acct-alpha"
	retiredID           = "acct-retired"
)

// investmentAccounts has Zeta (brokerage), Alpha (retirement) and Retired (a closed brokerage).
func investmentAccounts() fakeReportStore {
	return accountsStore(
		store.Account{ID: zetaID, Name: "Zeta", Type: store.AccountTypeBrokerage},
		store.Account{ID: alphaID, Name: "Alpha", Type: store.AccountTypeRetirement},
		store.Account{ID: retiredID, Name: "Retired", Type: store.AccountTypeBrokerage, Closed: true},
	)
}

func Test_holdings_help_shows_the_account_flag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Regexp(t, `(?m)--account name +`+regexp.QuoteMeta(holdingsAccountHelp)+`$`, stdout.String())
}

func Test_holdings_reads_the_accounts_named_by_id_and_by_name(t *testing.T) {
	var got store.HoldingsParams
	fake := investmentAccounts()
	fake.gotHoldings = &got
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fake, &stdout, &stderr, "--account", "alpha", "--account", zetaID)

	require.NoError(t, err)
	assert.Equal(t, store.HoldingsParams{
		AsOf:       time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
		AccountIDs: []string{alphaID, zetaID},
	}, got)
}

func Test_holdings_captions_the_named_accounts_in_the_order_given(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, investmentAccounts(), &stdout, &stderr, "--account", "Zeta", "--account", "alpha")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Holdings on 2026-09-29 in Zeta, Alpha, amounts in CAD; cash not included\n")
}

func Test_holdings_captions_a_native_listing_with_the_named_accounts_and_no_currency(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, investmentAccounts(), &stdout, &stderr, "--account", "Zeta", "--currency", "native")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Holdings on 2026-09-29 in Zeta; cash not included\n")
}

func Test_holdings_captions_an_account_named_twice_once(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, investmentAccounts(), &stdout, &stderr, "--account", "Zeta", "--account", zetaID)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Holdings on 2026-09-29 in Zeta, amounts in CAD; cash not included\n")
}

func Test_holdings_lists_a_named_closed_account_with_its_closed_mark_in_its_row_not_its_caption(t *testing.T) {
	fake := investmentAccounts()
	row := brokerageHolding()
	row.AccountID, row.Account, row.AccountClosed = retiredID, "Retired", true
	fake.holdings = store.Holdings{Holdings: []store.Holding{row}}
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fake, &stdout, &stderr, "--account", "Retired")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Holdings on 2026-09-29 in Retired, amounts in CAD; cash not included\n")
	assert.Contains(t, stdout.String(), "Retired (closed)")
}
