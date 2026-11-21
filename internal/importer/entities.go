package importer

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

// requiredEntities are the v9 Z_PRIMARYKEY entity names Import must resolve
// before reading any row, rather than hard-coding their Z_ENT numbers.
var requiredEntities = []string{"CashFlowTransaction", "CategoryTag", "UserTag"}

// The optional entities: resolved with requiredEntities, but absent, each has no map entry
// and the data it names is empty rather than a refusal.
const (
	investmentEntity    = "InvestmentTransaction"
	securityEntity      = "Security"
	securityQuoteEntity = "SecurityQuote"
	positionEntity      = "Position"
	lotEntity           = "Lot"
	// splitEntryEntity is the entity behind the split-tags join table; its absence leaves that table's name as written.
	splitEntryEntity = "CashFlowTransactionEntry"
)

// optionalEntities lists every optional entity name resolveEntities looks up.
var optionalEntities = []string{investmentEntity, securityEntity, securityQuoteEntity, positionEntity, lotEntity, splitEntryEntity}

// keepName is the EntityResolver that reads every name as written.
func keepName(name string, _ map[string][]int64) (string, bool) { return name, true }

// resolvedEntities is the entities resolveEntities found: ents holds each found name's single Z_ENT, listed
// every Z_ENT per name, for the EntityResolver.
type resolvedEntities struct {
	ents   map[string]int64
	listed map[string][]int64
}

// resolveEntities returns each looked-up entity's Z_ENT by name from Z_PRIMARYKEY, or an *UnmappableError naming
// every missing required entity, or else every looked-up entity listed more than once.
func resolveEntities(ctx context.Context, src Source) (resolvedEntities, error) {
	names := slices.Concat(requiredEntities, optionalEntities)
	listed := make(map[string][]int64, len(names))
	args := make([]any, len(names))
	for i, name := range names {
		args[i] = name
	}
	query := "SELECT Z_ENT, Z_NAME FROM Z_PRIMARYKEY WHERE Z_NAME IN (?" + strings.Repeat(", ?", len(names)-1) + ")"
	err := src.QueryRows(ctx, query, args, func(scan func(dest ...any) error) error {
		var ent int64
		var name string
		if err := scan(&ent, &name); err != nil {
			return err
		}
		listed[name] = append(listed[name], ent)
		return nil
	})
	if err != nil {
		return resolvedEntities{}, fmt.Errorf("resolve entities: %w", err)
	}

	var missing []string
	for _, name := range requiredEntities {
		if len(listed[name]) == 0 {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return resolvedEntities{}, &UnmappableError{Reason: fmt.Sprintf(
			"the snapshot has no %s entity, which quarry needs to read Quicken's records", joinNames(missing, "or"))}
	}

	ents := make(map[string]int64, len(listed))
	var repeated []string
	for _, name := range slices.Sorted(maps.Keys(listed)) {
		if len(listed[name]) > 1 {
			repeated = append(repeated, name)
			continue
		}
		ents[name] = listed[name][0]
	}
	if len(repeated) > 0 {
		return resolvedEntities{}, &UnmappableError{Reason: repeatedReason(repeated)}
	}
	return resolvedEntities{ents: ents, listed: listed}, nil
}

// repeatedReason is the refusal for the sorted names of entities Z_PRIMARYKEY lists more than once.
func repeatedReason(names []string) string {
	if len(names) == 1 {
		return fmt.Sprintf("the snapshot lists the %s entity more than once, so quarry cannot tell which records are its", names[0])
	}
	return fmt.Sprintf("the snapshot lists the %s entities more than once, so quarry cannot tell which records are theirs",
		joinNames(names, "and"))
}

// joinNames joins names with ", " between all but the last pair and " <conjunction> " before the last:
// "A", "A or B", "A, B or C".
func joinNames(names []string, conjunction string) string {
	switch len(names) {
	case 0:
		// unreachable: both callers check len(names) > 0 first.
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " " + conjunction + " " + names[len(names)-1]
	}
}
