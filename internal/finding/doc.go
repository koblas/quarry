// Package finding owns what quarry means by a finding: the ten finding
// types and their display order, the id grammar users write into
// findings.ignore, the status rule, the suggested fix for each type, and the
// counts a sync or status reports.
//
// It has no dependencies beyond the standard library, so the store, the
// report layer and the commands all read one definition instead of each
// re-deriving it.
package finding
