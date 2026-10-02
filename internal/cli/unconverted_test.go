package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	noRatesWarning = "the store has no exchange rates, so amounts are listed in each account's own currency; " +
		"run quarry sync to fetch them"
	multiTagWarning = "1 split carries more than one tag, so the rows add up to more than the total"
)

var firstRate = time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)

// warningsDoc is the part of a --json document these tests read.
type warningsDoc struct {
	Warnings []string `json:"warnings"`
}

// unconvertedCases are the lines the unconverted count gives, whichever command reads it.
var unconvertedCases = []struct {
	name        string
	currency    string
	unconverted store.Unconverted
	want        []string
}{
	{
		name: "no rates in the store", currency: "CAD",
		unconverted: store.Unconverted{Transactions: 2},
		want:        []string{noRatesWarning},
	},
	{
		name: "one transaction before the first rate takes is", currency: "CAD",
		unconverted: store.Unconverted{Transactions: 1, FirstRate: firstRate},
		want:        []string{"1 transaction dated before 2026-01-02, the first exchange rate in the store, is listed in USD, not converted to CAD"},
	},
	{
		name: "two transactions before the first rate take are", currency: "CAD",
		unconverted: store.Unconverted{Transactions: 2, FirstRate: firstRate},
		want:        []string{"2 transactions dated before 2026-01-02, the first exchange rate in the store, are listed in USD, not converted to CAD"},
	},
	{
		name: "a thousand transactions are grouped", currency: "CAD",
		unconverted: store.Unconverted{Transactions: 1000, FirstRate: firstRate},
		want:        []string{"1,000 transactions dated before 2026-01-02, the first exchange rate in the store, are listed in USD, not converted to CAD"},
	},
	{
		name: "a USD report names CAD as the native currency", currency: "USD",
		unconverted: store.Unconverted{Transactions: 2, FirstRate: firstRate},
		want:        []string{"2 transactions dated before 2026-01-02, the first exchange rate in the store, are listed in CAD, not converted to USD"},
	},
	{
		name: "a count of zero warns of nothing even with rates in the store", currency: "CAD",
		unconverted: store.Unconverted{FirstRate: firstRate},
		want:        []string{},
	},
}

func Test_spend_warns_of_unconverted_transactions_in_text_and_json(t *testing.T) {
	for _, c := range unconvertedCases {
		t.Run(c.name, func(t *testing.T) {
			fake := withSpending(fakeReportStore{})
			fake.spending.Unconverted = c.unconverted
			var textOut, textErr, jsonOut, jsonErr bytes.Buffer

			textRun := executeSpend(t, fake, spendNow, &textOut, &textErr, "--currency", c.currency)
			jsonRun := executeSpend(t, fake, spendNow, &jsonOut, &jsonErr, "--currency", c.currency, "--json")

			require.NoError(t, textRun)
			require.NoError(t, jsonRun)
			assert.Equal(t, warningText(c.want), textErr.String())
			assert.Equal(t, warningText(c.want), jsonErr.String())
			var doc warningsDoc
			require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc))
			assert.Equal(t, c.want, doc.Warnings)
		})
	}
}

func Test_cashflow_warns_of_unconverted_transactions_in_text_and_json(t *testing.T) {
	for _, c := range unconvertedCases {
		t.Run(c.name, func(t *testing.T) {
			fake := withCashFlow(fakeReportStore{})
			fake.cashFlow.Unconverted = c.unconverted
			var textOut, textErr, jsonOut, jsonErr bytes.Buffer

			textRun := executeCashFlow(t, fake, &textOut, &textErr, "--currency", c.currency)
			jsonRun := executeCashFlow(t, fake, &jsonOut, &jsonErr, "--currency", c.currency, "--json")

			require.NoError(t, textRun)
			require.NoError(t, jsonRun)
			assert.Equal(t, warningText(c.want), textErr.String())
			assert.Equal(t, warningText(c.want), jsonErr.String())
			var doc warningsDoc
			require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc))
			assert.Equal(t, c.want, doc.Warnings)
		})
	}
}

func Test_spend_by_tag_puts_the_exchange_rate_warning_between_the_left_out_account_and_the_multi_tag_note(t *testing.T) {
	fake := namedAccounts()
	fake.spending = tagSpending(1).spending
	fake.spending.Unconverted = store.Unconverted{Transactions: 2}
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--by", "tag", "--account", oldCardID, "--json")

	require.NoError(t, err)
	var doc warningsDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Warnings, 3)
	assert.Contains(t, doc.Warnings[0], "Old Card")
	assert.Equal(t, []string{noRatesWarning, multiTagWarning}, doc.Warnings[1:])
	assert.Equal(t, warningText(doc.Warnings), stderr.String())
}

func Test_cashflow_puts_the_exchange_rate_warning_after_the_left_out_account(t *testing.T) {
	fake := namedAccounts()
	fake.cashFlow = withCashFlow(fakeReportStore{}).cashFlow
	fake.cashFlow.Unconverted = store.Unconverted{Transactions: 2}
	var stdout, stderr bytes.Buffer

	err := executeCashFlow(t, fake, &stdout, &stderr, "--account", oldCardID, "--json")

	require.NoError(t, err)
	var doc warningsDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Warnings, 2)
	assert.Contains(t, doc.Warnings[0], "Old Card")
	assert.Equal(t, noRatesWarning, doc.Warnings[1])
	assert.Equal(t, warningText(doc.Warnings), stderr.String())
}

// warningText is the stderr text of warnings: each on its own prefixed line.
func warningText(warnings []string) string {
	var text bytes.Buffer
	for _, w := range warnings {
		text.WriteString("quarry: warning: " + w + "\n")
	}
	return text.String()
}
