package finding_test

import (
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/stretchr/testify/assert"
)

func Test_types_lists_the_ten_types_in_display_order(t *testing.T) {
	assert.Equal(t, []finding.Type{
		"duplicate", "one-sided-transfer", "unlinked-transfer", "uncategorized",
		"mixed-categories", "payee-variants", "similar-categories", "unused-category", "unclassified-account",
		"shares-without-cost",
	}, finding.Types())
}

func Test_read_time_is_true_only_for_the_unclassified_account_and_shares_without_cost_types(t *testing.T) {
	for _, typ := range finding.Types() {
		assert.Equal(t, typ == finding.UnclassifiedAccount || typ == finding.SharesWithoutCost, typ.ReadTime(), typ)
	}
	assert.False(t, finding.Type("future-kind").ReadTime())
}

func Test_types_returns_a_fresh_slice_each_call(t *testing.T) {
	first := finding.Types()
	first[0] = "changed"

	assert.Equal(t, finding.Duplicate, finding.Types()[0])
}

func Test_known_is_true_for_each_listed_type_and_false_for_any_other(t *testing.T) {
	for _, typ := range finding.Types() {
		assert.True(t, typ.Known(), typ)
	}
	assert.False(t, finding.Type("future-kind").Known())
	assert.False(t, finding.Type("").Known())
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
		{name: "a number sorts before a non-number arriving second", a: "txn-b", b: "txn-9", want: "duplicate:txn-9+txn-b"},
		{name: "a number sorts before a non-number arriving first", a: "txn-9", b: "txn-b", want: "duplicate:txn-9+txn-b"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, finding.PairID(finding.Duplicate, c.a, c.b))
		})
	}
}

func Test_PayeeKey_reduces_a_payee_name_to_the_key_its_variants_share(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"a trailing store number is cut", "TIM HORTONS #1234", "tim-hortons"},
		{"case is ignored", "Tim Hortons", "tim-hortons"},
		{"hyphens split like spaces", "TIM-HORTONS", "tim-hortons"},
		{"cut at the first star", "AMZN Mktp CA*1A2B3", "amzn-mktp-ca"},
		{"cut at whichever of star and hash comes first", "A#B*C", "a"},
		{"cut at a star with letters after it", "SQ *Coffee Shop", "sq"},
		{"cut at a hash with letters after it", "Tim #Hortons", "tim"},
		{"a diacritic is kept", "Café", "café"},
		{"no diacritic folding", "Cafe", "cafe"},
		{"a combining mark is a separator", "Cafe\u0301 Nord", "cafe-nord"},
		{"punctuation splits a word", "Tim's Hortons.", "tim-s-hortons"},
		{"runs of spaces collapse", "  Tim   Hortons  ", "tim-hortons"},
		{"a numeric token is dropped, not the whole name", "Store 24", "store"},
		{"a hyphen splits before the digit drop", "7-Eleven #123", "eleven"},
		{"a token holding a digit is dropped whole", "7Eleven", ""},
		{"only a store number", "#1234", ""},
		{"only digits", "12345", ""},
		{"only a star", "*", ""},
		{"only a hash", "#", ""},
		{"empty", "", ""},
		{"all punctuation", "..., -!", ""},
		{"non-Latin letters are kept", "Москва", "москва"},
		{"Arabic-Indic digits are digits", "Store ٢٤", "store"},
		{"a token holding an Arabic-Indic digit is dropped whole", "a٢b", ""},
		{"a vulgar fraction is a separator, not a digit", "Half½Price", "half-price"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, finding.PayeeKey(c.in))
		})
	}
}

func Test_CategoryKey_reduces_a_category_path_to_the_key_its_look_alikes_share(t *testing.T) {
	cases := []struct {
		name     string
		kind     string
		fullPath string
		want     string
	}{
		{"a plural is cut", "expense", "Groceries", "grocery"},
		{"the singular is the key", "expense", "Grocery", "grocery"},
		{"case is ignored", "expense", "GROCERY", "grocery"},
		{"an income key carries the income prefix", "income", "Grocery", "income:grocery"},
		{"levels join with a slash", "expense", "Auto:Fuel", "auto/fuel"},
		{"punctuation splits a word", "expense", "Auto & Fuel", "auto-fuel"},
		{"ies becomes y", "expense", "Utilities", "utility"},
		{"ss is kept", "expense", "Business", "business"},
		{"a token of three runes is untouched", "expense", "Gas", "gas"},
		{"a four-rune token ending in ss is kept", "expense", "Gass", "gass"},
		{"a four-rune token loses its s", "expense", "Cats", "cat"},
		{"length counts runes, not bytes", "expense", "Ées", "ées"},
		{"a diacritic is kept when the s is cut", "expense", "Cafés", "café"},
		{"digits are kept", "expense", "Auto 2024", "auto-2024"},
		{"the literal rule cuts a plural-looking singular", "expense", "Taxes", "taxe"},
		{"no diacritic folding", "expense", "Café", "café"},
		{"a combining mark is a separator", "expense", "Cafe\u0301 Nord", "cafe-nord"},
		{"non-Latin letters are kept", "expense", "Москва", "москва"},
		{"Arabic-Indic digits are kept as digits", "expense", "Store ٢٤", "store-٢٤"},
		{"a vulgar fraction is a separator, not a digit", "expense", "Half½Price", "half-price"},
		{"an empty level stays empty", "expense", "Auto:&:Fuel", "auto//fuel"},
		{"empty", "expense", "", ""},
		{"punctuation only", "expense", "&&", ""},
		{"separators only", "expense", "::", ""},
		{"a kind other than income and expense has no key", "system", "Groceries", ""},
		{"an empty kind has no key", "", "Groceries", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, finding.CategoryKey(c.kind, c.fullPath))
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
		{finding.UnclassifiedAccount, finding.Fix{
			Sentence: "Add this account's id to accounts.registered if it is an RRSP, RRIF, TFSA, RESP, FHSA or other registered plan, else to accounts.non-registered, in " +
				"~/Library/Application Support/quarry/config.toml; quarry acb leaves registered accounts out",
			Heading:     "Unclassified investment accounts",
			GroupClause: "list each account's id (acct-…) in accounts.registered or accounts.non-registered in ~/Library/Application Support/quarry/config.toml; see quarry findings --help",
		}},
		{finding.SharesWithoutCost, finding.Fix{
			Sentence: "Open this Add Shares transaction in Quicken and enter the shares' cost basis, then run quarry sync; " +
				"until then quarry acb counts them at no cost, so its gains on this security are too high",
			Heading:     "Shares added with no cost",
			GroupClause: "enter what each one cost on its Add Shares transaction in Quicken, then run quarry sync; until then quarry acb counts those shares at no cost",
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

func Test_classify_counts_a_fixed_finding_that_is_ignored_as_fixed_and_newly_fixed(t *testing.T) {
	states := []finding.State{{ID: "uncategorized:payee-2", Fixed: true, NewlyFixed: true}}

	got := finding.Classify(states, []string{"uncategorized:payee-2"})

	assert.Equal(t, []finding.Status{finding.StatusFixed}, got.Statuses)
	assert.Equal(t, finding.Counts{Fixed: 1, NewlyFixed: 1}, got.Counts)
}

func Test_classify_does_not_count_an_ignored_new_finding_as_new(t *testing.T) {
	states := []finding.State{
		{ID: "duplicate:txn-1+txn-2", New: true},
		{ID: "duplicate:txn-3+txn-4", New: true},
		{ID: "one-sided-transfer:xfer-3"},
	}

	got := finding.Classify(states, []string{"duplicate:txn-3+txn-4"})

	assert.Equal(t, []finding.Status{finding.StatusOpen, finding.StatusIgnored, finding.StatusOpen}, got.Statuses)
	assert.Equal(t, finding.Counts{Open: 2, Ignored: 1, New: 1}, got.Counts)
}

func Test_classify_lists_each_unmatched_element_in_file_order(t *testing.T) {
	states := []finding.State{{ID: "duplicate:txn-1+txn-2"}}
	ignore := []string{"uncategorized:payee-9", "duplicate:txn-1+txn-2", "", "uncategorized:payee-9", "duplicate:txn-1+txn-2"}

	got := finding.Classify(states, ignore)

	assert.Equal(t, []string{"uncategorized:payee-9", "", "uncategorized:payee-9"}, got.Unmatched)
}

func Test_classify_with_no_ignore_list_counts_as_before_and_lists_nothing_unmatched(t *testing.T) {
	states := []finding.State{
		{ID: "duplicate:txn-1+txn-2", New: true},
		{ID: "duplicate:txn-3+txn-4"},
		{ID: "uncategorized:payee-2", Fixed: true, NewlyFixed: true},
		{ID: "uncategorized:payee-3", Fixed: true},
	}

	got := finding.Classify(states, nil)

	assert.Equal(t, finding.Counts{Open: 2, Fixed: 2, New: 1, NewlyFixed: 1}, got.Counts)
	assert.Nil(t, got.Unmatched)
}
