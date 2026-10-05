// White-box: sharesWithoutCostRows and sharesCount are unexported layout rules driven directly.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// sharesFinding is an open shares-without-cost finding for the add the arguments describe, shares in millionths.
func sharesFinding(txn string, date time.Time, account, security string, shares int64) report.ListedFinding {
	return openFinding(store.Finding{
		ID:   finding.ID(finding.SharesWithoutCost, txn),
		Type: finding.SharesWithoutCost,
		Items: []store.FindingItem{{
			Date: date, Account: account, Security: security, Shares: shares,
		}},
	})
}

func Test_sharesWithoutCostRows_pads_the_ids_accounts_and_securities_to_the_widest(t *testing.T) {
	findings := []report.ListedFinding{
		sharesFinding("itxn-9", findingDay(2016, time.March, 1), "Margin", "XEQT", 100_000_000),
		sharesFinding("itxn-123", findingDay(2015, time.December, 31), "Questrade Margin", "Vanguard FTSE", 2_500_000),
	}

	got := sharesWithoutCostRows(findings, openView)

	assert.Equal(t, []string{
		"  shares-without-cost:itxn-9    2016-03-01  Margin            XEQT           100 shares",
		"  shares-without-cost:itxn-123  2015-12-31  Questrade Margin  Vanguard FTSE  2.5 shares",
	}, got)
}

func Test_sharesWithoutCostRows_escapes_the_account_and_security_before_padding_them(t *testing.T) {
	findings := []report.ListedFinding{
		sharesFinding("itxn-1", findingDay(2016, time.March, 1), "A\tB", "X\nY", 3_000_000),
		sharesFinding("itxn-2", findingDay(2016, time.March, 1), "Cash", "ZZZZZ", 3_000_000),
	}

	got := sharesWithoutCostRows(findings, openView)

	assert.Equal(t, []string{
		`  shares-without-cost:itxn-1  2016-03-01  A\tB  X\nY   3 shares`,
		"  shares-without-cost:itxn-2  2016-03-01  Cash  ZZZZZ  3 shares",
	}, got)
}

func Test_sharesWithoutCostRows_ends_an_ignored_finding_with_a_marker_under_the_all_view_only(t *testing.T) {
	findings := []report.ListedFinding{
		withStatus(sharesFinding("itxn-1", findingDay(2016, time.March, 1), "Margin", "XEQT", 3_000_000), finding.StatusIgnored),
		sharesFinding("itxn-2", findingDay(2016, time.March, 1), "Margin", "XEQT", 3_000_000),
	}

	assert.Equal(t, []string{
		"  shares-without-cost:itxn-1  2016-03-01  Margin  XEQT  3 shares  ignored",
		"  shares-without-cost:itxn-2  2016-03-01  Margin  XEQT  3 shares",
	}, sharesWithoutCostRows(findings, allView))
	assert.Equal(t, "  shares-without-cost:itxn-1  2016-03-01  Margin  XEQT  3 shares", sharesWithoutCostRows(findings, ignoredView)[0])
}

func Test_sharesCount_says_share_only_for_exactly_one(t *testing.T) {
	cases := []struct {
		name      string
		millionth int64
		want      string
	}{
		{name: "exactly one", millionth: 1_000_000, want: "1 share"},
		{name: "half", millionth: 500_000, want: "0.5 shares"},
		{name: "one and a bit", millionth: 1_000_001, want: "1.000001 shares"},
		{name: "just under one", millionth: 999_999, want: "0.999999 shares"},
		{name: "many, grouped", millionth: 1_200_000_000, want: "1,200 shares"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, sharesCount(c.millionth))
		})
	}
}

func Test_renderFindings_lists_shares_added_with_no_cost_under_the_ruled_heading(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{
			Type:     finding.SharesWithoutCost,
			Findings: []report.ListedFinding{sharesFinding("itxn-123", findingDay(2016, time.March, 1), "Questrade Margin", "XEQT", 100_000_000)},
		}},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, findingsView{status: finding.StatusOpen}, false)

	assert.Equal(t, "Shares added with no cost (1): enter what each one cost on its Add Shares transaction in Quicken, then run quarry sync; until then quarry acb counts those shares at no cost\n"+
		"  shares-without-cost:itxn-123  2016-03-01  Questrade Margin  XEQT  100 shares\n\n"+
		"1 open finding\n", got)
}
