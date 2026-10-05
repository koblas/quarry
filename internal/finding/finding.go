package finding

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Type names one kind of finding; its string form is the id prefix and the
// --type value.
type Type string

// The nine finding types.
const (
	Duplicate         Type = "duplicate"
	OneSidedTransfer  Type = "one-sided-transfer"
	UnlinkedTransfer  Type = "unlinked-transfer"
	Uncategorized     Type = "uncategorized"
	MixedCategories   Type = "mixed-categories"
	PayeeVariants     Type = "payee-variants"
	SimilarCategories Type = "similar-categories"
	UnusedCategory    Type = "unused-category"

	UnclassifiedAccount Type = "unclassified-account"
)

// ReadTime reports whether findings of t are computed from the config at read time: never stored, never new or fixed.
func (t Type) ReadTime() bool {
	return t == UnclassifiedAccount
}

// Types returns every finding type in display order, as a fresh slice.
func Types() []Type {
	return []Type{
		Duplicate, OneSidedTransfer, UnlinkedTransfer, Uncategorized,
		MixedCategories, PayeeVariants, SimilarCategories, UnusedCategory, UnclassifiedAccount,
	}
}

// Known reports whether t is one of the types Types lists; a store written by a newer quarry may hold others.
func (t Type) Known() bool {
	return slices.Contains(Types(), t)
}

// NoPayee is the entity of the uncategorized finding for splits whose
// transaction has no payee.
const NoPayee = "no-payee"

// MatchDays is how many days apart two transactions may be and still match as a pair finding
// (duplicate, unlinked-transfer), inclusive.
const MatchDays = 3

// MixedMin is the fewest categorized transactions a payee needs before it can be a mixed-categories finding.
const MixedMin = 3

// PayeeKey returns the payee-variants entity for a payee name, or "" when nothing is left to compare.
// Ids are a contract (users write them into config.toml), so the rule never changes silently: cut at the first
// "*" or "#", lower-case, split on every rune that is no letter or digit, drop each token holding a digit, join with "-".
// No diacritic folding: "Café" and "Cafe" are different keys.
func PayeeKey(name string) string {
	if i := strings.IndexAny(name, "*#"); i >= 0 {
		name = name[:i]
	}
	tokens := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	kept := tokens[:0]
	for _, token := range tokens {
		if !strings.ContainsFunc(token, unicode.IsDigit) {
			kept = append(kept, token)
		}
	}
	return strings.Join(kept, "-")
}

// Category kinds that similar-categories compares; any other kind has no key.
const (
	kindIncome  = "income"
	kindExpense = "expense"
)

// incomeKeyPrefix keeps an income group's entity apart from an expense group with the same key; a key never holds ":".
const incomeKeyPrefix = kindIncome + idSeparator

// singularMinRunes is the shortest token whose plural ending is cut.
const singularMinRunes = 4

// CategoryKey returns the similar-categories entity for a category of the given kind and full path, or "" when the
// kind is no income or expense or no level holds a letter or digit. Ids are a contract (users write them into
// config.toml), so the rule never changes silently. Income keys start with "income:". No diacritic folding:
// "Café" and "Cafe" are different keys.
func CategoryKey(kind, fullPath string) string {
	prefix := ""
	switch kind {
	case kindIncome:
		prefix = incomeKeyPrefix
	case kindExpense:
	default:
		return ""
	}
	// Per level: lower-case, split on every rune that is no letter or digit, singularise each token, join with "-";
	// levels join with "/".
	levels := strings.Split(fullPath, ":")
	named := false
	for i, level := range levels {
		tokens := strings.FieldsFunc(strings.ToLower(level), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		named = named || len(tokens) > 0
		for j, token := range tokens {
			tokens[j] = singular(token)
		}
		levels[i] = strings.Join(tokens, "-")
	}
	if !named {
		return ""
	}
	return prefix + strings.Join(levels, "/")
}

// singular cuts a plural ending off a token of at least singularMinRunes runes: "ies" becomes "y", a final "s"
// after anything but another "s" goes.
func singular(token string) string {
	if utf8.RuneCountInString(token) < singularMinRunes {
		return token
	}
	if stem, ok := strings.CutSuffix(token, "ies"); ok {
		return stem + "y"
	}
	if strings.HasSuffix(token, "ss") {
		return token
	}
	return strings.TrimSuffix(token, "s")
}

// idSeparator splits a finding id into its type and entity.
const idSeparator = ":"

// pairJoiner joins the two transaction ids of a pair finding's entity.
const pairJoiner = "+"

// ID returns the finding id "<type>:<entity>". Ids are a contract: users write
// them into config.toml, so the grammar never changes silently.
func ID(t Type, entity string) string {
	return string(t) + idSeparator + entity
}

// PairID returns the id of a finding about two entities named like "txn-9":
// the one with the lower numeric suffix comes first ("txn-9" before
// "txn-10"), whatever order a and b arrive in. Ids without a numeric suffix
// compare as strings.
func PairID(t Type, a, b string) string {
	if pairLess(b, a) {
		a, b = b, a
	}
	return ID(t, a+pairJoiner+b)
}

// pairLess reports whether entity a sorts before b by numeric suffix, falling
// back to string order when either suffix is not a number or both are equal.
func pairLess(a, b string) bool {
	na, okA := numericSuffix(a)
	nb, okB := numericSuffix(b)
	if okA && okB && na != nb {
		return na < nb
	}
	return a < b
}

// numericSuffix parses the digits after the last "-" of an entity id.
func numericSuffix(entity string) (int64, bool) {
	i := strings.LastIndex(entity, "-")
	n, err := strconv.ParseInt(entity[i+1:], 10, 64)
	return n, err == nil
}

// Status is where a finding stands: open, ignored or fixed. It is derived,
// never stored.
type Status string

// The three statuses.
const (
	StatusOpen    Status = "open"
	StatusIgnored Status = "ignored"
	StatusFixed   Status = "fixed"
)

// StatusOf derives a finding's status: fixed wins over ignored, so a finding
// that is gone reads fixed even when its id is still listed in
// findings.ignore.
func StatusOf(fixed, ignored bool) Status {
	switch {
	case fixed:
		return StatusFixed
	case ignored:
		return StatusIgnored
	default:
		return StatusOpen
	}
}

// Counts are the finding tallies a sync or status reports. New counts open
// findings first found by the latest build; NewlyFixed counts findings fixed
// by it.
type Counts struct {
	Open       int
	Ignored    int
	Fixed      int
	New        int
	NewlyFixed int
}

// State is what a tally needs to know about one finding: its id, whether it is fixed, and whether
// the latest build found it (New) or fixed it (NewlyFixed).
type State struct {
	ID         string
	Fixed      bool
	New        bool
	NewlyFixed bool
}

// Classified is the outcome of Classify: Statuses[i] is the status of states[i]; Unmatched holds
// every ignore element that names none of the states, in ignore order, duplicates kept.
type Classified struct {
	Statuses  []Status
	Counts    Counts
	Unmatched []string
}

// Classify is the one place an id in ignore is matched to a finding. It derives each state's status
// with StatusOf, tallies them (an ignored finding is never New; a fixed one counts as fixed even if
// listed) and reports the ignore elements that match no state. A nil ignore list ignores nothing.
func Classify(states []State, ignore []string) Classified {
	listed := make(map[string]bool, len(ignore))
	for _, id := range ignore {
		listed[id] = true
	}

	out := Classified{Statuses: make([]Status, len(states))}
	known := make(map[string]bool, len(states))
	for i, s := range states {
		known[s.ID] = true
		out.Statuses[i] = StatusOf(s.Fixed, listed[s.ID])
		switch out.Statuses[i] {
		case StatusFixed:
			out.Counts.Fixed++
			if s.NewlyFixed {
				out.Counts.NewlyFixed++
			}
		case StatusIgnored:
			out.Counts.Ignored++
		case StatusOpen:
			out.Counts.Open++
			if s.New {
				out.Counts.New++
			}
		}
	}
	for _, id := range ignore {
		if !known[id] {
			out.Unmatched = append(out.Unmatched, id)
		}
	}
	return out
}

// Fix is the suggested repair for one finding type: Sentence is the JSON
// `fix`; Heading and GroupClause make a text group header "<Heading> (<n>):
// <GroupClause>", where the renderer owns the counts in parentheses.
type Fix struct {
	Sentence    string
	Heading     string
	GroupClause string
}

var fixes = map[Type]Fix{
	Duplicate: {
		Sentence:    "Delete the extra one in Quicken, or ignore the pair if both are real",
		Heading:     "Possible duplicates",
		GroupClause: "delete the extra one in Quicken, or ignore the pair if both are real",
	},
	OneSidedTransfer: {
		Sentence:    "Re-enter it as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file",
		Heading:     "One-sided transfers",
		GroupClause: "re-enter each as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file",
	},
	UnlinkedTransfer: {
		Sentence:    "Make the pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts",
		Heading:     "Unlinked transfers",
		GroupClause: "make each pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts",
	},
	Uncategorized: {
		Sentence:    "Give this payee's splits a category in Quicken",
		Heading:     "Uncategorized",
		GroupClause: "give each payee's splits a category in Quicken",
	},
	MixedCategories: {
		Sentence:    "Pick one category for this payee's transactions in Quicken, or ignore it if the mix is intended",
		Heading:     "Payees in mixed categories",
		GroupClause: "pick one category per payee in Quicken, or ignore a payee whose mix is intended",
	},
	PayeeVariants: {
		Sentence:    "Rename these payees to one in Quicken and add a renaming rule, or ignore the group if they are different merchants",
		Heading:     "Payee variants",
		GroupClause: "rename each group to one payee in Quicken and add a renaming rule",
	},
	SimilarCategories: {
		Sentence:    "Merge these categories into one in Quicken, or ignore the group if they mean different things",
		Heading:     "Similar categories",
		GroupClause: "merge each group into one category in Quicken",
	},
	UnusedCategory: {
		Sentence:    "No transaction uses it; check that no scheduled transaction or budget does, then delete it in Quicken, or ignore it to keep it",
		Heading:     "Unused categories",
		GroupClause: "no transaction uses them; check that no scheduled transaction or budget does, then delete them in Quicken",
	},
	UnclassifiedAccount: {
		Sentence: "Add this account's id to accounts.registered if it is an RRSP, RRIF, TFSA, RESP, FHSA or other registered plan, else to accounts.non-registered, in " +
			"~/Library/Application Support/quarry/config.toml; quarry acb leaves registered accounts out",
		Heading:     "Unclassified investment accounts",
		GroupClause: "list each account's id (acct-…) in accounts.registered or accounts.non-registered in ~/Library/Application Support/quarry/config.toml; see quarry findings --help",
	},
}

// Fix returns the suggested repair for t; the zero Fix for an unknown type.
func (t Type) Fix() Fix {
	return fixes[t]
}
