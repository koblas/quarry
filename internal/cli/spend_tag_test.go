package cli_test

import (
	"bytes"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tagSpending(multiTagSplits int) fakeReportStore {
	return fakeReportStore{spending: store.Spending{
		Rows: []store.SpendingRow{
			{Key: nil, Currency: "CAD", Spent: 4208},
			{Key: new("trip"), Currency: "CAD", Spent: 30000},
		},
		Totals:         []store.SpendingTotal{{Currency: "CAD", Spent: 34208}},
		MultiTagSplits: multiTagSplits,
	}}
}

func Test_spend_by_tag_heads_the_first_column_Tag_and_labels_untagged_splits(t *testing.T) {
	var got store.SpendingParams
	var stdout, stderr bytes.Buffer
	fake := tagSpending(0)
	fake.gotSpending = &got

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--by", "tag")

	require.NoError(t, err)
	assert.Equal(t, store.SpendByTag, got.By)
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		"Tag       Currency   Spent\n"+
		"(no tag)  CAD        42.08\n"+
		"trip      CAD       300.00\n"+
		"Total     CAD       342.08\n", stdout.String())
}

func Test_spend_by_tag_json_names_the_row_key_tag_and_carries_the_warning_unprefixed(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, tagSpending(2), spendNow, &stdout, &stderr, "--by", "tag", "--json")

	require.NoError(t, err)
	assert.JSONEq(t, `{"since":"2026-01-01","until":"2026-09-29","by":"tag","currency":"CAD","account_filter":[],
		"rows":[{"tag":null,"currency":"CAD","spent":"42.08"},{"tag":"trip","currency":"CAD","spent":"300.00"}],
		"totals":[{"currency":"CAD","spent":"342.08"}],
		"warnings":["2 splits carry more than one tag, so the rows add up to more than the total"]}`, stdout.String())
	assert.Equal(t, "quarry: warning: 2 splits carry more than one tag, so the rows add up to more than the total\n", stderr.String())
}

func Test_spend_by_tag_warns_with_the_singular_phrase_for_one_split(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, tagSpending(1), spendNow, &stdout, &stderr, "--by", "tag")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: 1 split carries more than one tag, so the rows add up to more than the total\n", stderr.String())
}

func Test_spend_by_tag_warns_with_thousands_grouping_for_many_splits(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, tagSpending(1234), spendNow, &stdout, &stderr, "--by", "tag")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: 1,234 splits carry more than one tag, so the rows add up to more than the total\n", stderr.String())
}

func Test_spend_by_tag_prints_no_warning_when_no_split_has_two_tags(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, tagSpending(0), spendNow, &stdout, &stderr, "--by", "tag", "--json")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `"warnings": []`)
}

func Test_spend_by_tag_writes_no_warning_when_stdout_fails(t *testing.T) {
	var stderr bytes.Buffer

	err := executeSpend(t, tagSpending(3), spendNow, failingWriter{err: errNoSpace}, &stderr, "--by", "tag")

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.Empty(t, stderr.String())
}

func Test_spend_by_category_prints_no_warning_whatever_the_multi_tag_count(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, tagSpending(5), spendNow, &stdout, &stderr, "--by", "category")

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
}
