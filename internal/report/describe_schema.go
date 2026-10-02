package report

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/store"
)

// describeSchemaCommand names describe_schema in its refusals.
const describeSchemaCommand = "describe_schema"

// Schema is the store's description in listing order, with the accounts and categories cut to the
// list cap; AccountsTotal and CategoriesTotal count them before the cut.
type Schema struct {
	store.Schema

	AccountsTotal, CategoriesTotal int
}

// DescribeSchema describes the store from one read. Relations come tables first, then by name;
// accounts in quarry accounts order (name ignoring case, then name, then id); categories by full
// path. Accounts and categories are sorted, then cut to the first maxListed each; a maxListed of 0
// or less keeps every one. It refuses with a RefusalError when interrupted or when the store
// cannot be opened; other store errors are returned unchanged.
func (s *Server) DescribeSchema(ctx context.Context, maxListed int) (Schema, error) {
	read, err := s.store.Schema(ctx)
	if err != nil {
		return Schema{}, s.readRefusal(ctx, describeSchemaCommand, err)
	}
	schema := Schema{Schema: read, AccountsTotal: len(read.Accounts), CategoriesTotal: len(read.Categories)}
	slices.SortFunc(schema.Relations, compareRelations)
	slices.SortFunc(schema.Accounts, compareAccounts)
	slices.SortFunc(schema.Categories, func(a, b store.Category) int { return strings.Compare(a.FullPath, b.FullPath) })
	if maxListed > 0 {
		schema.Accounts = schema.Accounts[:min(len(schema.Accounts), maxListed)]
		schema.Categories = schema.Categories[:min(len(schema.Categories), maxListed)]
	}
	return schema, nil
}

// compareRelations orders tables before views, then by name.
func compareRelations(a, b store.Relation) int {
	if byKind := cmp.Compare(kindTier(a.Kind), kindTier(b.Kind)); byKind != 0 {
		return byKind
	}
	return strings.Compare(a.Name, b.Name)
}

// kindTier is 0 for a table and 1 for anything else.
func kindTier(kind string) int {
	if kind == store.RelationTable {
		return 0
	}
	return 1
}

// compareAccounts is quarry accounts order: name ignoring case, then name, then id, the last two by bytes.
func compareAccounts(a, b store.Account) int {
	return cmp.Or(
		strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		strings.Compare(a.Name, b.Name),
		strings.Compare(a.ID, b.ID),
	)
}
