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

const anomaliesEmpty = "no unusually large charges from 2026-01-01 to 2026-09-29"

func leftOutAnomaliesWarning(name string) string {
	return "account \"" + name + "\" is not used in reports in Quicken, so anomalies leaves it out; " +
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync"
}

func linkedAnomaliesWarning(name string) string {
	return "account \"" + name + "\" uses linked account tracking in Quicken, so anomalies leaves it out, as Quicken's reports do"
}

// ordinaryCharge is three 2025 charges of a payee and a 100.00 one on 2026-03-02, not above twice their median.
func ordinaryCharge() []store.Charge {
	return payeeHistory("Bell Canada", time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC), 10000, 9000, 9300, 9605)
}

// onAccount is rows moved to the Chequing account with id.
func onAccount(rows []store.Charge, id string) []store.Charge {
	moved := make([]store.Charge, len(rows))
	for i, r := range rows {
		r.Account = store.Account{ID: id, Name: "Chequing", Currency: "CAD"}
		moved[i] = r
	}
	return moved
}

func Test_anomalies_captions_the_named_accounts_and_passes_their_ids_to_the_report(t *testing.T) {
	var got store.ChargeParams
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.charges = store.Charges{Rows: onAccount(ordinaryCharge(), visaID)}
	fake.gotCharges = &got

	err := executeAnomalies(t, fake, &stdout, &stderr, "--account", "visa infinite", "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, []string{visaID, chequingID}, got.AccountIDs)
	assert.Contains(t, stdout.String(), "Unusually large charges 2026-01-01 to 2026-09-29 in Visa Infinite, Chequing, amounts in CAD\n\n")
}

func Test_anomalies_escapes_a_line_break_in_a_named_accounts_caption(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := accountsStore(store.Account{ID: chequingID, Name: "Chq\nOne"})

	err := executeAnomalies(t, fake, &stdout, &stderr, "--account", chequingID)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `in Chq\nOne, amounts in CAD`+"\n\n")
}

func Test_anomalies_json_names_the_accounts_it_was_limited_to(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.charges = store.Charges{Rows: onAccount(ordinaryCharge(), chequingID)}

	err := executeAnomalies(t, fake, &stdout, &stderr, "--json", "--account", "chequing")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "\"account_filter\": [\n    {\n      \"id\": \""+chequingID+"\",\n      \"name\": \"Chequing\"\n    }\n  ],\n")
}

func Test_anomalies_prints_each_warning_on_stderr_and_in_the_json_warnings(t *testing.T) {
	inChequing := store.Charges{Rows: onAccount(ordinaryCharge(), chequingID)}
	cases := []struct {
		name    string
		charges store.Charges
		args    []string
		want    []string
	}{
		{
			name:    "an account not in reports and one under linked tracking, each once in the order named",
			charges: inChequing,
			args:    []string{"--account", linkedID, "--account", "Old Card", "--account", chequingID, "--account", bothID, "--account", "old card"},
			want:    []string{linkedAnomaliesWarning("Netskope 401(k)"), leftOutAnomaliesWarning("Old Card"), linkedAnomaliesWarning("Old 401(k)")},
		},
		{
			name:    "the store's span when no charge falls in the period",
			charges: store.Charges{Transactions: span(t, "2003-01-04", "2026-09-26")},
			want:    []string{anomaliesEmpty + "; the store's transactions run 2003-01-04 to 2026-09-26"},
		},
		{
			name: "an empty store",
			want: []string{anomaliesEmpty + "; the store has no transactions"},
		},
		{
			name:    "the named accounts' span when none of their charges falls in the period",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", chequingID},
			want:    []string{anomaliesEmpty + " in the named accounts; their transactions run 2019-03-02 to 2024-11-30"},
		},
		{
			name: "named accounts with no transactions",
			args: []string{"--account", chequingID},
			want: []string{anomaliesEmpty + " in the named accounts; they have no transactions"},
		},
		{
			name:    "charges that fall only in other accounts than the one named",
			charges: store.Charges{Rows: onAccount(ordinaryCharge(), visaID)},
			args:    []string{"--account", chequingID},
			want:    []string{anomaliesEmpty + " in the named accounts; they have no transactions"},
		},
		{
			name:    "the left-out warnings before the empty note when a reported account is also named",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", "Old Card", "--account", chequingID},
			want: []string{
				leftOutAnomaliesWarning("Old Card"),
				anomaliesEmpty + " in the named accounts; their transactions run 2019-03-02 to 2024-11-30",
			},
		},
		{
			name:    "only the left-out warnings when every named account is left out",
			charges: store.Charges{Transactions: span(t, "2019-03-02", "2024-11-30")},
			args:    []string{"--account", linkedID, "--account", "Old Card"},
			want:    []string{linkedAnomaliesWarning("Netskope 401(k)"), leftOutAnomaliesWarning("Old Card")},
		},
		{
			name:    "only the left-out warning when every named account is left out and the store is empty",
			charges: store.Charges{},
			args:    []string{"--account", oldBankID},
			want:    []string{leftOutAnomaliesWarning("Old Bank")},
		},
		{
			name:    "no note when charges were checked but none is unusual",
			charges: inChequing,
			args:    []string{"--account", chequingID},
			want:    []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := namedAccounts()
			fake.charges = c.charges

			err := executeAnomalies(t, fake, &stdout, &stderr, append([]string{"--json"}, c.args...)...)

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

func Test_anomalies_reads_the_charges_once_and_takes_the_empty_note_from_that_read(t *testing.T) {
	var reads int
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{charges: store.Charges{Transactions: span(t, "2003-01-04", "2026-09-26")}, chargesReads: &reads}

	err := executeAnomalies(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Equal(t, "quarry: warning: "+anomaliesEmpty+"; the store's transactions run 2003-01-04 to 2026-09-26\n", stderr.String())
}
