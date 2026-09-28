package importer

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// requiredEntities are the v9 Z_PRIMARYKEY entity names Import must resolve
// before reading any row, rather than hard-coding their Z_ENT numbers.
var requiredEntities = []string{"CashFlowTransaction", "CategoryTag", "UserTag"}

// resolveEntities reads Z_PRIMARYKEY and returns each of requiredEntities'
// Z_ENT number by name. It returns an *UnmappableError (S4 reason 7) naming
// every entity the snapshot's Z_PRIMARYKEY lacks, sorted and joined with
// "or".
func resolveEntities(ctx context.Context, src Source) (map[string]int64, error) {
	found := make(map[string]int64, len(requiredEntities))
	args := make([]any, len(requiredEntities))
	for i, name := range requiredEntities {
		args[i] = name
	}
	query := "SELECT Z_ENT, Z_NAME FROM Z_PRIMARYKEY WHERE Z_NAME IN (?, ?, ?)"
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
