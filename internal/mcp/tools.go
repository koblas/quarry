package mcp

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/koblas/quarry/internal/finding"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool names, as clients call them.
const (
	toolQuery       = "query"
	toolDescribe    = "describe_schema"
	toolSyncStatus  = "sync_status"
	toolDataQuality = "data_quality"
)

// Row limits: the most any tool returns, and data_quality's default.
const (
	maxRows          = 500
	defaultFindLimit = 50
)

// instructions is sent to every client at initialize.
const instructions = `quarry serves David's Quicken Classic for Mac data from a local, read-only
store. Call sync_status first and tell the user how old the snapshot is
(snapshot.taken_at). Call describe_schema before writing SQL: its
conventions say which views already leave out transfers and how amounts,
signs and currencies work. Every number you report must come from a tool
result; never estimate. quarry cannot change data: fixes are made in
Quicken, then the user runs quarry sync.`

const queryDescription = `Run one read-only SQL query (DuckDB dialect) against quarry's store and
return its columns and rows. Call describe_schema first for the tables,
views and conventions. For spending and income use v_spending and
v_cash_flow: they already leave out transfers between the user's own
accounts. Returns at most ` + "`limit`" + ` rows (default 500, the most allowed);
aggregate in SQL rather than paging through rows. The store cannot be
changed, and other files, databases and extensions are off.
Send one statement; if you send several, only the last one's rows come back.`

const describeSchemaDescription = `Describe quarry's store: every table and view with its columns and types,
the conventions for amounts, signs, transfers and currencies, the
accounts, the category tree, and the first and last transaction dates.
Call this before writing SQL for query.`

const syncStatusDescription = `Report how fresh quarry's data is: the snapshot the store was built from
and when it was taken, the dates its transactions cover, the checks sync
ran (balances reconciled to Quicken, splits, transfers), open findings,
and Bank of Canada rate coverage. quarry cannot refresh the data; if it
is old, ask the user to run quarry sync.`

const dataQualityDescription = `List the data-quality findings quarry's last sync found: problems to fix
in Quicken (duplicates, one-sided or unlinked transfers, uncategorized
splits, payees in mixed categories, payee name variants, similar or
unused categories). Each finding has an id, the suggested fix, and the
transactions, payees or categories it is about. quarry never fixes them:
the user fixes them in Quicken and runs quarry sync, and fixed findings
drop off. To ignore a finding the user adds its id to findings.ignore in
quarry's config file.`

// errNotBuilt is what a tool answers until its handler lands.
var errNotBuilt = errors.New("this tool is not available yet")

type (
	// queryInput is the query tool's arguments.
	queryInput struct {
		SQL   string `json:"sql"`
		Limit int    `json:"limit"`
	}
	// dataQualityInput is the data_quality tool's arguments.
	dataQualityInput struct {
		Status string `json:"status"`
		Type   string `json:"type"`
		Limit  int    `json:"limit"`
	}
	// noInput is the arguments of a tool that takes none.
	noInput struct{}
)

// notBuilt answers every tool call with errNotBuilt.
func notBuilt[In any](context.Context, *sdk.CallToolRequest, In) (*sdk.CallToolResult, any, error) {
	return nil, nil, errNotBuilt
}

// addTools registers quarry's four tools on srv.
func (s *Server) addTools(srv *sdk.Server) {
	sdk.AddTool(srv, tool(toolDescribe, describeSchemaDescription, objectSchema(nil)), notBuilt[noInput])
	sdk.AddTool(srv, tool(toolQuery, queryDescription, objectSchema(map[string]*jsonschema.Schema{
		"sql":   {Type: "string", MinLength: new(1)},
		"limit": limitSchema(maxRows),
	}, "sql")), handler(s.query))
	sdk.AddTool(srv, tool(toolSyncStatus, syncStatusDescription, objectSchema(nil)), notBuilt[noInput])
	sdk.AddTool(srv, tool(toolDataQuality, dataQualityDescription, objectSchema(map[string]*jsonschema.Schema{
		"status": {Type: "string", Enum: []any{"open", "ignored", "fixed", "all"}, Default: []byte(`"open"`)},
		"type":   {Type: "string", Enum: findingTypes()},
		"limit":  limitSchema(defaultFindLimit),
	})), notBuilt[dataQualityInput])
}

// tool describes one tool; its result is a JSON object.
func tool(name, description string, input *jsonschema.Schema) *sdk.Tool {
	return &sdk.Tool{
		Name:         name,
		Description:  description,
		InputSchema:  input,
		OutputSchema: &jsonschema.Schema{Type: "object"},
	}
}

// objectSchema is the schema of an arguments object that refuses unknown properties.
func objectSchema(properties map[string]*jsonschema.Schema, required ...string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           properties,
		Required:             required,
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// limitSchema is a row limit between 1 and maxRows that defaults to def.
func limitSchema(def int) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:    "integer",
		Minimum: new(1.0),
		Maximum: new(float64(maxRows)),
		Default: []byte(strconv.Itoa(def)),
	}
}

// findingTypes lists the finding types data_quality accepts.
func findingTypes() []any {
	types := finding.Types()
	enum := make([]any, len(types))
	for i, t := range types {
		enum[i] = string(t)
	}
	return enum
}
