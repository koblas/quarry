package document

import "github.com/koblas/quarry/internal/report"

// Schema is the describe_schema tool's structured result: the conventions, then the store's
// relations, the accounts and categories listed, the transaction dates, and the warnings.
type Schema struct {
	Conventions string           `json:"conventions"`
	Relations   []SchemaRelation `json:"relations"`
	Accounts    []SchemaAccount  `json:"accounts"`
	Categories  []SchemaCategory `json:"categories"`
	Dates       SchemaDates      `json:"dates"`
	Warnings    []string         `json:"warnings"`
}

// SchemaRelation is one table or view with its columns in declared order.
type SchemaRelation struct {
	Name    string         `json:"name"`
	Kind    string         `json:"kind"`
	Columns []SchemaColumn `json:"columns"`
}

// SchemaColumn is one column's name and DuckDB type name.
type SchemaColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// SchemaAccount is one account the schema lists.
type SchemaAccount struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Currency string `json:"currency"`
	Closed   bool   `json:"closed"`
}

// SchemaCategory is one category the schema lists.
type SchemaCategory struct {
	ID       string `json:"id"`
	FullPath string `json:"full_path"`
	Kind     string `json:"kind"`
	Hidden   bool   `json:"hidden"`
}

// SchemaDates is the first and last transaction day; both are null when the store has no transactions.
type SchemaDates struct {
	First *string `json:"first"`
	Last  *string `json:"last"`
}

// NewSchema converts schema into the describe_schema document with report.SQLConventions as its
// conventions; every list is an empty array, never null, and warnings are as given.
func NewSchema(schema report.Schema, warnings []string) Schema {
	relations := make([]SchemaRelation, len(schema.Relations))
	for i, r := range schema.Relations {
		columns := make([]SchemaColumn, len(r.Columns))
		for j, c := range r.Columns {
			columns[j] = SchemaColumn{Name: c.Name, Type: c.Type}
		}
		relations[i] = SchemaRelation{Name: r.Name, Kind: r.Kind, Columns: columns}
	}
	accounts := make([]SchemaAccount, len(schema.Accounts))
	for i, a := range schema.Accounts {
		accounts[i] = SchemaAccount{ID: a.ID, Name: a.Name, Type: a.Type, Currency: a.Currency, Closed: a.Closed}
	}
	categories := make([]SchemaCategory, len(schema.Categories))
	for i, c := range schema.Categories {
		categories[i] = SchemaCategory{ID: c.ID, FullPath: c.FullPath, Kind: c.Kind, Hidden: c.Hidden}
	}
	return Schema{
		Conventions: report.SQLConventions,
		Relations:   relations,
		Accounts:    accounts,
		Categories:  categories,
		Dates:       SchemaDates{First: nullDate(schema.Transactions.First), Last: nullDate(schema.Transactions.Last)},
		Warnings:    append([]string{}, warnings...),
	}
}
