package v9

import (
	"regexp"
	"strconv"
)

// referenceEntities maps each entity number the reference's numbered names use to the Z_NAME Core Data
// records for it in Z_PRIMARYKEY.
var referenceEntities = map[int64]string{
	1:  "Account",
	9:  "Budget",
	15: "CashFlowTransactionEntry",
	30: "DownloadSession",
	55: "QuickFillRuleSplitEntry",
	76: "UserTag",
	78: "Transaction",
	79: "CashFlowTransaction",
}

// numberedNames match the two forms Core Data derives a name from an entity number in: Z_<n><NAME> (a table, or
// a column with an optional disambiguator suffix) and Z<n>_<NAME>. The number is group 2.
var numberedNames = []*regexp.Regexp{
	regexp.MustCompile(`^(Z_)(\d+)([A-Z].*)$`),
	regexp.MustCompile(`^(Z)(\d+)(_.+)$`),
}

// ResolveName renames a reference table or column name to the numbering of a file whose Z_PRIMARYKEY maps each
// Z_NAME to its Z_ENT numbers in entities. ok is true when name carries no entity number or its number resolved;
// otherwise name comes back as written, with ok false (its entity is absent from entities, listed more than once,
// or not one the reference numbers).
func ResolveName(name string, entities map[string][]int64) (string, bool) {
	for _, form := range numberedNames {
		parts := form.FindStringSubmatch(name)
		if parts == nil {
			continue
		}
		prefix, digits, suffix := parts[1], parts[2], parts[3]

		referenceNumber, err := strconv.ParseInt(digits, 10, 64)
		if err != nil {
			return name, false
		}
		entity, known := referenceEntities[referenceNumber]
		if !known {
			return name, false
		}
		numbers := entities[entity]
		if len(numbers) != 1 {
			return name, false
		}
		return prefix + strconv.FormatInt(numbers[0], 10) + suffix, true
	}
	return name, true
}
