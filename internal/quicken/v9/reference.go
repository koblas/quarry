package v9

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/platform/sqlschema"
)

// ReferenceDDL is the embedded, verbatim v9 Core Data schema — the one copy of the reference ever corrected.
//
//go:embed reference.sql
var ReferenceDDL string

// ReferenceLabel identifies the commit ReferenceDDL is pinned to.
const ReferenceLabel = "hardkoded/quicken-skills@752107b"

// Reference executes ReferenceDDL against a private in-memory database and
// returns the resulting schema, unscoped. Callers apply their own table
// scope before comparing it against a snapshot's schema.
func Reference(ctx context.Context) (sqlschema.Schema, error) {
	schema, err := executeDDL(ctx, ReferenceDDL)
	if err != nil {
		return nil, fmt.Errorf("build reference schema: %w", err)
	}
	return schema, nil
}

// executeDDL runs ddl against a fresh in-memory database and returns the
// schema it produces.
func executeDDL(ctx context.Context, ddl string) (sqlschema.Schema, error) {
	db, err := sqlite.OpenMemory(ctx)
	if err != nil {
		return nil, fmt.Errorf("open in-memory database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(ctx, ddl); err != nil {
		return nil, fmt.Errorf("execute schema: %w", err)
	}
	return db.Schema(ctx)
}
