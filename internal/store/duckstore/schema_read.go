package duckstore

import (
	"context"
	"database/sql"

	"github.com/koblas/quarry/internal/store"
)

// userRelations selects the user-visible relations of the current database's main schema, never DuckDB's own.
const userRelations = `database_name = current_database() AND schema_name = 'main' AND NOT internal`

// schemaRelationsQuery reads each table and view with its columns, tables first, columns in declared order.
const schemaRelationsQuery = `
SELECT r.name, r.kind, c.column_name, c.data_type
FROM (
	SELECT table_name AS name, '` + store.RelationTable + `' AS kind, 0 AS tier FROM duckdb_tables() WHERE ` + userRelations + `
	UNION ALL
	SELECT view_name, '` + store.RelationView + `', 1 FROM duckdb_views() WHERE ` + userRelations + `
) r
JOIN duckdb_columns() c ON c.database_name = current_database() AND c.schema_name = 'main' AND c.table_name = r.name
ORDER BY r.tier, r.name, c.column_index`

// schemaAccountsQuery and schemaCategoriesQuery read the rows a schema description lists; the report orders and cuts them.
const (
	schemaAccountsQuery   = `SELECT id, name, type, currency, closed FROM accounts`
	schemaCategoriesQuery = `SELECT id, name, full_path, kind, hidden FROM categories`
	schemaDatesQuery      = `SELECT min(date), max(date) FROM transactions`
)

// Schema reads the store's tables and views with their columns, every account and category, and
// the first and last transaction dates, on one connection. Relations come from DuckDB's catalog
// at call time, tables first, each by name; accounts and categories are in no order. It refuses a
// store it cannot open or read with *store.OpenError.
func (s *Store) Schema(ctx context.Context) (store.Schema, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.Schema{}, err
	}
	defer func() { _ = db.Close() }()

	var schema store.Schema
	err = db.QueryRows(ctx, schemaRelationsQuery, nil, func(scan func(dest ...any) error) error {
		var name, kind string
		var column store.Column
		if err := scan(&name, &kind, &column.Name, &column.Type); err != nil {
			return err
		}
		last := len(schema.Relations) - 1
		if last < 0 || schema.Relations[last].Name != name {
			schema.Relations = append(schema.Relations, store.Relation{Name: name, Kind: kind})
			last++
		}
		schema.Relations[last].Columns = append(schema.Relations[last].Columns, column)
		return nil
	})
	if err != nil {
		return store.Schema{}, openFault(s.Path(), err)
	}

	err = db.QueryRows(ctx, schemaAccountsQuery, nil, func(scan func(dest ...any) error) error {
		var a store.Account
		if err := scan(&a.ID, &a.Name, &a.Type, &a.Currency, &a.Closed); err != nil {
			return err
		}
		schema.Accounts = append(schema.Accounts, a)
		return nil
	})
	if err != nil {
		return store.Schema{}, openFault(s.Path(), err)
	}

	err = db.QueryRows(ctx, schemaCategoriesQuery, nil, func(scan func(dest ...any) error) error {
		var c store.Category
		if err := scan(&c.ID, &c.Name, &c.FullPath, &c.Kind, &c.Hidden); err != nil {
			return err
		}
		schema.Categories = append(schema.Categories, c)
		return nil
	})
	if err != nil {
		return store.Schema{}, openFault(s.Path(), err)
	}

	var first, last sql.NullTime
	err = db.QueryRows(ctx, schemaDatesQuery, nil, func(scan func(dest ...any) error) error {
		return scan(&first, &last)
	})
	if err != nil {
		return store.Schema{}, openFault(s.Path(), err)
	}
	schema.Transactions = store.TransactionRange{First: first.Time, Last: last.Time}
	return schema, nil
}
