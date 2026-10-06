// Package main: main cannot be imported, so these tests live beside the unexported run and
// the test helpers the cmd/quarry tests share.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fieldDeclaration is the field names one reference file's prose uses for one output: the --json
// document of a command, or the columns of a recipe.
type fieldDeclaration struct {
	file   string
	argv   []string
	recipe string
	names  []string
}

func (d fieldDeclaration) label() string {
	if d.recipe != "" {
		return d.file + " " + d.recipe
	}
	return d.file + " " + strings.Join(d.argv, " ")
}

var declaredFields = []fieldDeclaration{
	{file: "spending.md", argv: []string{"spend", "--json"}, names: []string{
		"category", "rows", "totals", "spent", "account_filter", "since", "until", "currency", "warnings",
	}},
	{file: "spending.md", argv: []string{"spend", "--by", "payee", "--json"}, names: []string{"payee"}},
	{file: "spending.md", argv: []string{"spend", "--by", "tag", "--json"}, names: []string{"tag"}},
	{file: "spending.md", argv: []string{"spend", "--by", "month", "--json"}, names: []string{"partial"}},
	{file: "spending.md", recipe: spendingTrendFile, names: []string{"period", "currency", "spent"}},
	{file: "cash-flow.md", argv: []string{"cashflow", "--json"}, names: []string{
		"periods", "income", "spent", "net", "savings_rate_pct", "totals", "partial", "currency", "warnings",
	}},
	{file: "cash-flow.md", recipe: incomeByCatFile, names: []string{"category", "currency", "income"}},
	{file: "recurring-and-anomalies.md", argv: []string{"recurring", "--since", "2000", "--json"}, names: []string{
		"series", "payee", "cadence", "new", "first_charge", "last_charge", "charge_count", "state", "amount", "first_amount",
		"per_year", "totals", "price_changes", "date", "from", "to", "change_pct", "currency",
		"native_currency", "native_amount", "native_first_amount",
	}},
	{file: "recurring-and-anomalies.md", argv: []string{"anomalies", "--json"}, names: []string{
		"anomalies", "amount", "usual", "times", "baseline", "earlier", "native_amount", "native_usual", "checked", "not_judged",
	}},
	{file: "monthly-summary.md", argv: []string{"summary", "--json"}, names: []string{
		"month", "since", "until", "currency", "snapshot", "covers_month", "dates", "findings", "anomalies", "recurring",
		"net_worth", "changes", "warnings",
	}},
	{file: "search.md", argv: []string{"search", "Savings Sweep", "--json"}, names: []string{
		"transfer", "excluded", "matched", "limit", "truncated", "currency", "amount",
	}},
	{file: "findings.md", argv: []string{"findings", "--json"}, names: []string{
		"findings", "id", "type", "status", "fix", "items", "counts", "open", "ignored", "fixed", "warnings",
	}},
}

// jsonKeys is every object key, at any depth, of the JSON document in raw.
func jsonKeys(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	var doc any
	require.NoError(t, json.Unmarshal(raw, &doc), string(raw))
	keys := map[string]bool{}
	var walk func(node any)
	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				keys[key] = true
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(doc)
	return keys
}

// absentNames is the names that are not in have.
func absentNames(have map[string]bool, names []string) []string {
	var absent []string
	for _, name := range names {
		if !have[name] {
			absent = append(absent, name)
		}
	}
	return absent
}

// unmentionedNames is the names that prose does not write between backticks.
func unmentionedNames(prose string, names []string) []string {
	return slices.DeleteFunc(slices.Clone(names), func(name string) bool { return strings.Contains(prose, "`"+name+"`") })
}

// producedNames is the names the declared output carries: the keys of the command's --json document,
// or the column names of the recipe's result.
func producedNames(t *testing.T, d fieldDeclaration) map[string]bool {
	t.Helper()
	if d.recipe != "" {
		names := map[string]bool{}
		for _, column := range runShippedRecipe(t, d.recipe).Columns {
			names[column.Name] = true
		}
		return names
	}
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, runWith(context.Background(), d.argv, spendEnv(&stdout, &stderr)), stderr.String())
	return jsonKeys(t, stdout.Bytes())
}

func Test_declared_field_check_flags_a_name_the_output_lacks(t *testing.T) {
	keys := jsonKeys(t, []byte(`{"rows":[{"spent":"1.00","tags":{"tag":null}}]}`))

	assert.Equal(t, []string{"bogus"}, absentNames(keys, []string{"rows", "spent", "tag", "bogus"}))
	assert.Equal(t, []string{"spent"}, unmentionedNames("It lists `rows` and spent.", []string{"rows", "spent"}))
}

func Test_every_field_the_reference_prose_declares_is_in_the_output_and_the_prose(t *testing.T) {
	recipeScenario(t)

	for _, d := range declaredFields {
		t.Run(d.label(), func(t *testing.T) {
			produced := producedNames(t, d)

			require.NotEmpty(t, produced)
			assert.Empty(t, absentNames(produced, d.names), "not in the output")
			assert.Empty(t, unmentionedNames(repoFile(t, referencesDir+"/"+d.file), d.names), "not in the prose")
		})
	}
}
