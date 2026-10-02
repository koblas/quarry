package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const recurringEmpty = "no recurring charges from 2026-01-01 to 2026-09-29"

func leftOutRecurringWarning(name string) string {
	return "account \"" + name + "\" is not used in reports in Quicken, so recurring leaves it out; " +
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync"
}

func linkedRecurringWarning(name string) string {
	return "account \"" + name + "\" uses linked account tracking in Quicken, so recurring leaves it out, as Quicken's reports do"
}

// chargesOn is four monthly Netflix.com charges, all on the account with id.
func chargesOn(id string) store.Charges {
	charges := monthlyCharges("Netflix.com", 999, 999, 999, 999)
	for i := range charges.Rows {
		charges.Rows[i].Account = store.Account{ID: id, Name: "Chequing", Currency: "CAD"}
	}
	return charges
}

// withCharges is fake answering the recurring read with charges.
func withCharges(fake fakeReportStore, charges store.Charges) fakeReportStore {
	fake.charges = charges
	return fake
}

func Test_recurring_captions_the_named_accounts_and_passes_their_ids_to_the_report(t *testing.T) {
	var got store.ChargeParams
	var stdout, stderr bytes.Buffer
	fake := withCharges(namedAccounts(), chargesOn(visaID))
	fake.gotCharges = &got

	err := executeRecurring(t, fake, &stdout, &stderr, "--since", "2000", "--account", "visa infinite", "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, []string{visaID, chequingID}, got.AccountIDs)
	assert.Contains(t, stdout.String(), "Recurring charges 2000-01-01 to 2026-09-29 in Visa Infinite, Chequing, amounts in CAD\n\n")
}

func Test_recurring_json_names_the_accounts_it_was_limited_to(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := withCharges(namedAccounts(), chargesOn(chequingID))

	err := executeRecurring(t, fake, &stdout, &stderr, "--since", "2000", "--json", "--account", "chequing")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "\"account_filter\": [\n    {\n      \"id\": \""+chequingID+"\",\n      \"name\": \"Chequing\"\n    }\n  ],\n")
}

func Test_recurring_prints_each_warning_on_stderr_and_in_the_json_warnings(t *testing.T) {
	cases := []struct {
		name    string
		charges store.Charges
		args    []string
		want    []string
	}{
		{
			name:    "an account not in reports and one under linked tracking, each once in the order named",
			charges: chargesOn(chequingID),
			args:    []string{"--account", linkedID, "--account", "Old Card", "--account", chequingID, "--account", bothID, "--account", "old card"},
			want:    []string{linkedRecurringWarning("Netskope 401(k)"), leftOutRecurringWarning("Old Card"), linkedRecurringWarning("Old 401(k)")},
		},
		{
			name:    "the store's span when no series runs in the period",
			charges: store.Charges{Transactions: span(t, "2003-01-04", "2026-09-26")},
			want:    []string{recurringEmpty + "; the store's transactions run 2003-01-04 to 2026-09-26"},
		},
		{
			name: "an empty store",
			want: []string{recurringEmpty + "; the store has no transactions"},
		},
		{
			name:    "the named accounts' span when none of their series runs in the period",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", chequingID},
			want:    []string{recurringEmpty + " in the named accounts; their transactions run 2019-03-02 to 2024-11-30"},
		},
		{
			name: "named accounts with no transactions",
			args: []string{"--account", chequingID},
			want: []string{recurringEmpty + " in the named accounts; they have no transactions"},
		},
		{
			name:    "series that run only in other accounts than the one named",
			charges: chargesOn(visaID),
			args:    []string{"--since", "2000", "--account", chequingID},
			want:    []string{"no recurring charges from 2000-01-01 to 2026-09-29 in the named accounts; they have no transactions"},
		},
		{
			name:    "the left-out warnings before the empty note when a reported account is also named",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", "Old Card", "--account", chequingID},
			want: []string{
				leftOutRecurringWarning("Old Card"),
				recurringEmpty + " in the named accounts; their transactions run 2019-03-02 to 2024-11-30",
			},
		},
		{
			name:    "only the left-out warnings when every named account is left out",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", linkedID, "--account", "Old Card"},
			want:    []string{linkedRecurringWarning("Netskope 401(k)"), leftOutRecurringWarning("Old Card")},
		},
		{
			name:    "only the left-out warning when every named account is left out and the store is empty",
			charges: store.Charges{},
			args:    []string{"--account", oldBankID},
			want:    []string{leftOutRecurringWarning("Old Bank")},
		},
		{
			name:    "no note when a series is listed",
			charges: chargesOn(chequingID),
			args:    []string{"--since", "2000", "--account", chequingID},
			want:    []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := withCharges(namedAccounts(), c.charges)

			err := executeRecurring(t, fake, &stdout, &stderr, append([]string{"--json"}, c.args...)...)

			require.NoError(t, err)
			var doc struct {
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
			assert.Equal(t, c.want, doc.Warnings)
			assert.Equal(t, warningLines(c.want), stderr.String())
		})
	}
}

// warningLines is the stderr text of warnings: each on its own prefixed line.
func warningLines(warnings []string) string {
	var text strings.Builder
	for _, w := range warnings {
		text.WriteString("quarry: warning: " + w + "\n")
	}
	return text.String()
}

func Test_recurring_reads_the_charges_once_and_takes_the_empty_note_from_that_read(t *testing.T) {
	var reads int
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: store.Charges{Transactions: span(t, "2003-01-04", "2026-09-26")}, chargesReads: &reads}

	err := executeRecurring(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Equal(t, "quarry: warning: "+recurringEmpty+"; the store's transactions run 2003-01-04 to 2026-09-26\n", stderr.String())
}

func Test_recurring_text_prints_caption_and_header_only_for_an_empty_period(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeRecurring(t, fakeReportStore{}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "Recurring charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		"Payee  Currency  Every  Amount  Per year  First  Last  Status  Price changes\n", stdout.String())
}
