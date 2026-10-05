// White-box: unclassifiedRows and accountItem are unexported layout rules driven directly.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// unclassifiedFinding is an open unclassified-account finding for the account the arguments describe.
func unclassifiedFinding(id, name, accountType, currency string, closed bool) report.ListedFinding {
	return openFinding(store.Finding{
		ID:   finding.ID(finding.UnclassifiedAccount, id),
		Type: finding.UnclassifiedAccount,
		Items: []store.FindingItem{{
			AccountID: id, Account: name, AccountType: accountType, Currency: currency, Closed: closed,
		}},
	})
}

func Test_unclassifiedRows_pads_the_ids_and_names_to_the_widest(t *testing.T) {
	findings := []report.ListedFinding{
		unclassifiedFinding("acct-9", "Old RRSP", "retirement", "CAD", false),
		unclassifiedFinding("acct-12", "Questrade TFSA", "brokerage", "USD", false),
	}

	got := unclassifiedRows(findings, openView)

	assert.Equal(t, []string{
		"  unclassified-account:acct-9   Old RRSP        retirement, CAD",
		"  unclassified-account:acct-12  Questrade TFSA  brokerage, USD",
	}, got)
}

func Test_unclassifiedRows_ends_a_closed_account_with_closed(t *testing.T) {
	findings := []report.ListedFinding{
		unclassifiedFinding("acct-31", "Old RRSP", "retirement", "CAD", true),
		unclassifiedFinding("acct-12", "Questrade TFSA", "brokerage", "CAD", false),
	}

	got := unclassifiedRows(findings, openView)

	assert.Equal(t, []string{
		"  unclassified-account:acct-31  Old RRSP        retirement, CAD, closed",
		"  unclassified-account:acct-12  Questrade TFSA  brokerage, CAD",
	}, got)
}

func Test_unclassifiedRows_escapes_the_account_name_before_padding_it(t *testing.T) {
	findings := []report.ListedFinding{
		unclassifiedFinding("acct-1", "A\tB", "brokerage", "CAD", false),
		unclassifiedFinding("acct-2", "Cash", "brokerage", "CAD", false),
	}

	got := unclassifiedRows(findings, openView)

	assert.Equal(t, []string{
		`  unclassified-account:acct-1  A\tB  brokerage, CAD`,
		"  unclassified-account:acct-2  Cash  brokerage, CAD",
	}, got)
}

func Test_unclassifiedRows_ends_an_ignored_finding_with_a_marker_under_the_all_view_only(t *testing.T) {
	findings := []report.ListedFinding{
		withStatus(unclassifiedFinding("acct-12", "Questrade TFSA", "brokerage", "CAD", true), finding.StatusIgnored),
		unclassifiedFinding("acct-31", "Old RRSP", "retirement", "CAD", false),
	}

	assert.Equal(t, []string{
		"  unclassified-account:acct-12  Questrade TFSA  brokerage, CAD, closed  ignored",
		"  unclassified-account:acct-31  Old RRSP        retirement, CAD",
	}, unclassifiedRows(findings, allView))
	assert.Equal(t, "  unclassified-account:acct-12  Questrade TFSA  brokerage, CAD, closed", unclassifiedRows(findings, ignoredView)[0])
}

func Test_renderFindings_lists_unclassified_accounts_under_the_ruled_heading(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.UnclassifiedAccount, Findings: []report.ListedFinding{
			unclassifiedFinding("acct-31", "Old RRSP", "retirement", "CAD", true),
			unclassifiedFinding("acct-12", "Questrade TFSA", "brokerage", "CAD", false),
		}}},
		Counts: finding.Counts{Open: 2},
	}

	got := renderFindings(listing, openView, true)

	assert.Equal(t, "Unclassified investment accounts (2): "+
		"list each account's id (acct-…) in accounts.registered or accounts.non-registered in ~/Library/Application Support/quarry/config.toml; see quarry findings --help\n"+
		"  unclassified-account:acct-31  Old RRSP        retirement, CAD, closed\n"+
		"  unclassified-account:acct-12  Questrade TFSA  brokerage, CAD\n"+
		"\n2 open findings\n"+ignoreHint, got)
}
