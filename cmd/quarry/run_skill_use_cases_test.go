// Package main: main cannot be imported, so these tests live beside the unexported run and
// the test helpers the cmd/quarry tests share.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/report/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skillUseCase is one in-scope question: the SKILL.md section 4 row that answers it, the
// command line run for it, and the check on that command's JSON.
type skillUseCase struct {
	name     string
	question string
	argv     []string
	answer   func(t *testing.T, stdout []byte)
}

func decodeDoc[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var doc T
	require.NoError(t, json.Unmarshal(raw, &doc), string(raw))
	return doc
}

func skillUseCases() []skillUseCase {
	const (
		recurringQuestion = "Subscriptions and recurring charges; when they started; price changes"
		spendQuestion     = "Spending by category, payee, tag or month"
	)
	return []skillUseCase{
		{
			name: "which subscriptions started this year", question: recurringQuestion,
			argv: []string{"recurring", "--json"},
			answer: func(t *testing.T, stdout []byte) {
				t.Helper()
				var started []string
				for _, series := range decodeDoc[document.Recurring](t, stdout).Series {
					if series.New {
						started = append(started, series.Payee)
					}
				}
				assert.Equal(t, []string{"Netflix"}, started)
			},
		},
		{
			name: "spending by payee for a period", question: spendQuestion,
			argv: []string{"spend", "--by", "payee", "--since", "2026", "--until", "2026", "--json"},
			answer: func(t *testing.T, stdout []byte) {
				t.Helper()
				doc := decodeDoc[struct {
					Rows   []document.SpendingPayeeRow `json:"rows"`
					Totals []document.SpendingTotal    `json:"totals"`
				}](t, stdout)
				spent := map[string]string{}
				for _, row := range doc.Rows {
					spent[payeeName(row.Payee)+"|"+row.Currency] = row.Spent
				}
				assert.Equal(t, "87.92", spent["Netflix|CAD"])
				assert.NotContains(t, spent, "Savings Sweep|CAD")
				assert.Contains(t, doc.Totals, document.SpendingTotal{Currency: "CAD", Spent: "595.79"})
			},
		},
		{
			name: "recurring charges with a price change", question: recurringQuestion,
			argv: []string{"recurring", "--json", "--since", "2000"},
			answer: func(t *testing.T, stdout []byte) {
				t.Helper()
				doc := decodeDoc[document.Recurring](t, stdout)
				var netflix document.RecurringSeries
				for _, series := range doc.Series {
					if series.Payee == "Netflix" {
						netflix = series
					}
				}
				assert.Equal(t, "2000-01-01", doc.Since)
				assert.Equal(t, "2026-02-12", netflix.FirstCharge)
				assert.Equal(t, []document.RecurringPriceChange{
					{Date: "2026-06-12", Currency: "CAD", From: "9.99", To: "11.99", ChangePct: 20},
				}, netflix.PriceChanges)
			},
		},
		{
			name: "unusually large charges", question: "Unusually large charges",
			argv: []string{"anomalies", "--json"},
			answer: func(t *testing.T, stdout []byte) {
				t.Helper()
				listed := decodeDoc[document.Anomalies](t, stdout).Anomalies
				require.Len(t, listed, 1)
				assert.Equal(t, "Hardware", *listed[0].Payee)
				assert.Equal(t, "250.00", listed[0].Amount)
				assert.Equal(t, "40.00", listed[0].Usual)
			},
		},
		{
			name: "duplicates and uncategorized items", question: "What to clean up in Quicken",
			argv: []string{"findings", "--json"},
			answer: func(t *testing.T, stdout []byte) {
				t.Helper()
				entries := decodeDoc[document.FindingsList](t, stdout).Findings
				ids := make([]string, 0, len(entries))
				for _, entry := range entries {
					ids = append(ids, entry.ID)
				}
				assert.ElementsMatch(t, []string{"duplicate:txn-gas-1+txn-gas-2", "uncategorized:no-payee"}, ids)
			},
		},
		{
			name: "cash flow and savings rate by year", question: "Income, spending, net and savings rate by month or year",
			argv: []string{"cashflow", "--by", "year", "--since", "2026", "--until", "2026", "--json"},
			answer: func(t *testing.T, stdout []byte) {
				t.Helper()
				assert.Contains(t, decodeDoc[document.CashFlow](t, stdout).Periods, document.CashFlowPeriodRow{
					Period: "2026", Currency: "CAD", Income: "10258.00", Spent: "595.79", Net: "9662.21", SavingsRatePct: new(94.2),
				})
			},
		},
		{
			name: "find a transaction by payee", question: "Find a transaction",
			argv: []string{"search", "Corner Deli", "--json"},
			answer: func(t *testing.T, stdout []byte) {
				t.Helper()
				found := decodeDoc[document.Search](t, stdout).Transactions
				require.Len(t, found, 1)
				assert.Equal(t, "txn-food", found[0].TransactionID)
				assert.Equal(t, "2026-05-05", found[0].Date)
				assert.Equal(t, "-11.00", found[0].Amount)
			},
		},
	}
}

// payeeName is the payee, or "" for rows with none.
func payeeName(payee *string) string {
	if payee == nil {
		return ""
	}
	return *payee
}

func Test_each_use_case_question_is_answered_by_the_command_the_skill_names(t *testing.T) {
	recipeScenario(t)

	for _, c := range skillUseCases() {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), c.argv, spendEnv(&stdout, &stderr))

			require.Equal(t, 0, exitCode, stderr.String())
			c.answer(t, stdout.Bytes())
		})
	}
}

func Test_use_case_argv_matches_the_command_the_skill_names(t *testing.T) {
	skill := repoFile(t, skillPath)

	for _, c := range skillUseCases() {
		t.Run(c.name, func(t *testing.T) {
			cell, err := skillRunCell(skill, c.question)

			require.NoError(t, err)
			assert.Empty(t, argvMismatches(cell, c.argv))
		})
	}
}

var errNoSkillRow = errors.New("no section 4 row for the question")

// skillRunCell is the Run cell of the section 4 row of skill whose Question cell is question.
func skillRunCell(skill, question string) (string, error) {
	_, afterHeading, found := strings.Cut(skill, "\n## 4. ")
	if !found {
		return "", errNoSkillRow
	}
	section, _, _ := strings.Cut(afterHeading, "\n## ")
	for line := range strings.SplitSeq(section, "\n") {
		cells, isRow := strings.CutPrefix(line, "| "+question+" | ")
		if isRow {
			return strings.TrimSuffix(cells, " |"), nil
		}
	}
	return "", errNoSkillRow
}

// argvMismatches lists where argv departs from cell's first code span: each literal word up to the
// first <placeholder> must match (a|b offers alternatives), and every --flag argv uses must appear in the cell.
func argvMismatches(cell string, argv []string) []string {
	_, afterTick, _ := strings.Cut(cell, "`")
	span, _, _ := strings.Cut(afterTick, "`")
	words := strings.Fields(strings.ReplaceAll(span, `\|`, "|"))
	if len(words) == 0 || words[0] != "quarry" {
		return []string{"cell does not start with a quarry command: " + cell}
	}
	var out []string
	for i, word := range words[1:] {
		if strings.HasPrefix(word, "<") {
			break
		}
		if i >= len(argv) || !slices.Contains(strings.Split(word, "|"), argv[i]) {
			out = append(out, "argv word "+word+" not matched at position "+strconv.Itoa(i))
		}
	}
	for _, arg := range argv {
		if strings.HasPrefix(arg, "--") && !strings.Contains(cell, arg) {
			out = append(out, "flag "+arg+" is not in the cell")
		}
	}
	return out
}

func Test_skill_run_cell_finds_the_row_of_section_4_for_a_question(t *testing.T) {
	skill := "# t\n\n## 4. Pick the command\n\n| Question | Run |\n| --- | --- |\n" +
		"| Spending | `quarry spend --by category\\|payee --since <date> --json` |\n" +
		"| Other | `quarry other --json` |\n\n## 5. Next\n\n| Spending | `quarry elsewhere` |\n"

	cell, err := skillRunCell(skill, "Spending")

	require.NoError(t, err)
	assert.Equal(t, "`quarry spend --by category\\|payee --since <date> --json`", cell)
}

func Test_skill_run_cell_reports_a_question_with_no_row_in_section_4(t *testing.T) {
	skill := "## 4. Pick the command\n\n| Spending | `quarry spend` |\n\n## 5. Next\n\n| Elsewhere | `quarry elsewhere` |\n"

	_, err := skillRunCell(skill, "Elsewhere")

	require.ErrorIs(t, err, errNoSkillRow)
}

func Test_skill_run_cell_reports_a_skill_with_no_section_4(t *testing.T) {
	_, err := skillRunCell("## 5. Next\n\n| Spending | `quarry spend` |\n", "Spending")

	require.ErrorIs(t, err, errNoSkillRow)
}

func Test_argv_matches_the_cell_through_an_alternative_and_before_a_placeholder(t *testing.T) {
	const cell = "`quarry spend --by category\\|payee --since <date> --json`"

	problems := argvMismatches(cell, []string{"spend", "--by", "payee", "--since", "2026", "--json"})

	assert.Empty(t, problems)
}

func Test_argv_mismatches_name_each_departure_from_the_cell(t *testing.T) {
	cases := []struct {
		name string
		cell string
		argv []string
		want []string
	}{
		{"a different subcommand", "`quarry spend --json`", []string{"spending", "--json"}, []string{"argv word spend not matched at position 0"}},
		{"a value outside the alternatives", "`quarry spend --by category\\|payee`", []string{"spend", "--by", "tag"}, []string{"argv word category|payee not matched at position 2"}},
		{"a literal word the argv leaves out", "`quarry recurring --json`", []string{"recurring"}, []string{"argv word --json not matched at position 1"}},
		{"a flag the cell never names", "`quarry spend --json`", []string{"spend", "--json", "--limit"}, []string{"flag --limit is not in the cell"}},
		{"a cell that is not a quarry command", "`references/sql/spending-trend.sql`", []string{"spend"}, []string{"cell does not start with a quarry command: `references/sql/spending-trend.sql`"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, argvMismatches(c.cell, c.argv))
		})
	}
}
