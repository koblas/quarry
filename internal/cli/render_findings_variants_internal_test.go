// White-box: payeeVariantRows and findingsHeader are unexported layout rules whose
// padding and singular/plural forms are best driven directly.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

const variantsFix = "rename each group to one payee in Quicken and add a renaming rule"

// variantRow is one payee of a payee-variants finding and its transaction count.
type variantRow struct {
	name string
	n    int
}

// variantFinding is an open payee-variants finding with one item per row.
func variantFinding(id string, rows ...variantRow) report.ListedFinding {
	items := make([]store.FindingItem, len(rows))
	for i, r := range rows {
		items[i] = store.FindingItem{Payee: r.name, Transactions: r.n}
	}
	return openFinding(store.Finding{ID: id, Type: finding.PayeeVariants, Items: items})
}

func variantsGroup(findings ...report.ListedFinding) report.FindingsGroup {
	return report.FindingsGroup{Type: finding.PayeeVariants, Findings: findings}
}

func Test_renderFindings_the_specification_example_for_payee_variants(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{variantsGroup(variantFinding("payee-variants:tim-hortons",
			variantRow{"TIM HORTONS #1234", 212}, variantRow{"Tim Hortons", 40}))},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, openView, true)

	assert.Equal(t, "Payee variants (1 group): "+variantsFix+"\n"+
		"  payee-variants:tim-hortons  2 payees, 252 transactions\n"+
		"    TIM HORTONS #1234  212 transactions\n"+
		"    Tim Hortons         40 transactions\n"+
		"\n1 open finding\n"+ignoreHint, got)
}

func Test_findingsHeader_counts_payee_variants_in_groups(t *testing.T) {
	cases := []struct {
		name  string
		group report.FindingsGroup
		want  string
	}{
		{"one group", variantsGroup(variantFinding("payee-variants:a")), "Payee variants (1 group): " + variantsFix},
		{"two groups", variantsGroup(variantFinding("payee-variants:a"), variantFinding("payee-variants:b")), "Payee variants (2 groups): " + variantsFix},
		{"no open one listed", variantsGroup(withStatus(variantFinding("payee-variants:a"), finding.StatusIgnored)), "Payee variants (1 group)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, findingsHeader(c.group))
		})
	}
}

func Test_payeeVariantRows_pads_ids_across_findings_and_names_and_counts_within_each(t *testing.T) {
	findings := []report.ListedFinding{
		variantFinding("payee-variants:shell", variantRow{"Shell #1", 20}, variantRow{"SHELL", 5}),
		variantFinding("payee-variants:café-nord", variantRow{"Café Nord", 2}, variantRow{"Cafe Nord", 100}),
	}

	got := payeeVariantRows(findings, openView)

	assert.Equal(t, []string{
		"  payee-variants:shell      2 payees, 25 transactions",
		"    Shell #1  20 transactions",
		"    SHELL      5 transactions",
		"  payee-variants:café-nord  2 payees, 102 transactions",
		"    Café Nord    2 transactions",
		"    Cafe Nord  100 transactions",
	}, got)
}

func Test_payeeVariantRows_singularises_a_one_transaction_row_and_groups_the_thousands(t *testing.T) {
	findings := []report.ListedFinding{variantFinding("payee-variants:costco", variantRow{"Costco", 1200}, variantRow{"COSTCO", 1})}

	got := payeeVariantRows(findings, openView)

	assert.Equal(t, []string{
		"  payee-variants:costco  2 payees, 1,201 transactions",
		"    Costco  1,200 transactions",
		"    COSTCO       1 transaction",
	}, got)
}

func Test_payeeVariantRows_ends_the_id_line_of_an_ignored_finding_with_a_marker_under_the_all_view(t *testing.T) {
	findings := []report.ListedFinding{withStatus(variantFinding("payee-variants:shell", variantRow{"Shell", 2}, variantRow{"SHELL", 1}), finding.StatusIgnored)}

	got := payeeVariantRows(findings, allView)

	assert.Equal(t, []string{
		"  payee-variants:shell  2 payees, 3 transactions  ignored",
		"    Shell  2 transactions",
		"    SHELL   1 transaction",
	}, got)
}
