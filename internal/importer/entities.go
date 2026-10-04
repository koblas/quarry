package importer

import (
	"context"
	"fmt"
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
)

// optionalEntities lists every optional entity name resolveEntities looks up.
var optionalEntities = []string{investmentEntity, securityEntity, securityQuoteEntity, positionEntity, lotEntity}

// resolveEntities returns each entity's Z_ENT by name from Z_PRIMARYKEY, or
// an *UnmappableError naming every missing required entity.
func resolveEntities(ctx context.Context, src Source) (map[string]int64, error) {
	names := slices.Concat(requiredEntities, optionalEntities)
	found := make(map[string]int64, len(names))
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
		found[name] = ent
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("resolve entities: %w", err)
	}

	var missing []string
	for _, name := range requiredEntities {
		if _, ok := found[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, &UnmappableError{Reason: fmt.Sprintf(
			"the snapshot has no %s entity, which quarry needs to read Quicken's records", joinOr(missing))}
	}
	return found, nil
}

// joinOr joins names with ", " between all but the last pair and " or "
// before the last: "A", "A or B", "A, B or C".
func joinOr(names []string) string {
	switch len(names) {
	case 0:
		// unreachable: resolveEntities only calls joinOr inside its own
		// len(missing) > 0 check.
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
	}
}
