package finding_test

import (
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/stretchr/testify/assert"
)

func Test_types_lists_the_eight_types_in_display_order(t *testing.T) {
	assert.Equal(t, []finding.Type{
		"duplicate", "one-sided-transfer", "unlinked-transfer", "uncategorized",
		"mixed-categories", "payee-variants", "similar-categories", "unused-category",
	}, finding.Types())
}

func Test_types_returns_a_fresh_slice_each_call(t *testing.T) {
	first := finding.Types()
	first[0] = "changed"

	assert.Equal(t, finding.Duplicate, finding.Types()[0])
}

func Test_id_joins_type_and_entity_with_a_colon(t *testing.T) {
	assert.Equal(t, "uncategorized:payee-88", finding.ID(finding.Uncategorized, "payee-88"))
}

func Test_id_of_splits_without_a_payee_uses_the_no_payee_entity(t *testing.T) {
	assert.Equal(t, "uncategorized:no-payee", finding.ID(finding.Uncategorized, finding.NoPayee))
}

func Test_pair_id_puts_the_lower_numeric_source_id_first(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want string
	}{
		{name: "already in order", a: "txn-4410", b: "txn-4412", want: "duplicate:txn-4410+txn-4412"},
		{name: "arrives reversed", a: "txn-4412", b: "txn-4410", want: "duplicate:txn-4410+txn-4412"},
		{name: "numeric, not string, order", a: "txn-10", b: "txn-9", want: "duplicate:txn-9+txn-10"},
		{name: "ids without a number compare as strings", a: "txn-b", b: "txn-a", want: "duplicate:txn-a+txn-b"},
		{name: "equal numbers fall back to string order", a: "txn-7", b: "acct-7", want: "duplicate:acct-7+txn-7"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, finding.PairID(finding.Duplicate, c.a, c.b))
		})
	}
}

func Test_status_of_derives_one_status_from_fixed_and_ignored(t *testing.T) {
	cases := []struct {
		name    string
		fixed   bool
		ignored bool
		want    finding.Status
	}{
		{name: "neither is open", want: finding.StatusOpen},
		{name: "ignored only", ignored: true, want: finding.StatusIgnored},
		{name: "fixed only", fixed: true, want: finding.StatusFixed},
		{name: "fixed wins over ignored", fixed: true, ignored: true, want: finding.StatusFixed},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, finding.StatusOf(c.fixed, c.ignored))
		})
	}
}

func Test_fix_pins_the_ruled_copy_for_every_type(t *testing.T) {
	cases := []struct {
		typ  finding.Type
		want finding.Fix
	}{
		{finding.Duplicate, finding.Fix{
			Sentence:    "Delete the extra one in Quicken, or ignore the pair if both are real",
			Heading:     "Possible duplicates",
			GroupClause: "delete the extra one in Quicken, or ignore the pair if both are real",
		}},
		{finding.OneSidedTransfer, finding.Fix{
			Sentence:    "Re-enter it as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file",
			Heading:     "One-sided transfers",
			GroupClause: "re-enter each as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file",
		}},
		{finding.UnlinkedTransfer, finding.Fix{
			Sentence:    "Make the pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts",
			Heading:     "Unlinked transfers",
			GroupClause: "make each pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts",
		}},
		{finding.Uncategorized, finding.Fix{
			Sentence:    "Give this payee's splits a category in Quicken",
			Heading:     "Uncategorized",
			GroupClause: "give each payee's splits a category in Quicken",
		}},
		{finding.MixedCategories, finding.Fix{
			Sentence:    "Pick one category for this payee's transactions in Quicken, or ignore it if the mix is intended",
			Heading:     "Payees in mixed categories",
			GroupClause: "pick one category per payee in Quicken, or ignore a payee whose mix is intended",
		}},
		{finding.PayeeVariants, finding.Fix{
			Sentence:    "Rename these payees to one in Quicken and add a renaming rule, or ignore the group if they are different merchants",
			Heading:     "Payee variants",
			GroupClause: "rename each group to one payee in Quicken and add a renaming rule",
		}},
		{finding.SimilarCategories, finding.Fix{
			Sentence:    "Merge these categories into one in Quicken, or ignore the group if they mean different things",
			Heading:     "Similar categories",
			GroupClause: "merge each group into one category in Quicken",
		}},
		{finding.UnusedCategory, finding.Fix{
			Sentence:    "No transaction uses it; check that no scheduled transaction or budget does, then delete it in Quicken, or ignore it to keep it",
			Heading:     "Unused categories",
			GroupClause: "no transaction uses them; check that no scheduled transaction or budget does, then delete them in Quicken",
		}},
	}

	for _, c := range cases {
		t.Run(string(c.typ), func(t *testing.T) {
			assert.Equal(t, c.want, c.typ.Fix())
		})
	}
}

func Test_fix_of_an_unknown_type_is_empty(t *testing.T) {
	assert.Equal(t, finding.Fix{}, finding.Type("made-up").Fix())
}
