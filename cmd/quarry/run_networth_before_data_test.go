package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	beforeSnapshotLine = "no account has a balance on 2026-03-01; the first balance is on 2026-03-02"
	beforeHistoryLine  = "no account has a balance at any month end from 2026-01-31 to 2026-02-28; the first balance is on 2026-03-02"

	beforeSnapshotWarning = "quarry: warning: " + beforeSnapshotLine + "\n"
	beforeHistoryWarning  = "quarry: warning: " + beforeHistoryLine + "\n"
)

// seedUncountedOnlyStore builds a store whose only transaction is in an account left out of Quicken's reports.
func seedUncountedOnlyStore(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	notInReports := chequingAccount("acct-out", 1)
	notInReports.NotInReports = true
	replaceStore(t, home, spendRows([]store.Account{notInReports},
		spendSplit{id: "out", account: "acct-out", currency: "CAD", day: day(2026, time.January, 5), cents: 500}))
}

func Test_run_networth_before_the_first_transaction_prints_the_caption_and_the_first_balance_warning(t *testing.T) {
	seedNetWorthStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2026-03-01"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Net worth on 2026-03-01, amounts in CAD\n\nType  Currency  Balance  In CAD\n", stdout.String())
	assert.Equal(t, beforeSnapshotWarning, stderr.String())
}

func Test_run_networth_before_the_first_transaction_lists_each_month_end_without_a_total_and_warns(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStdout string
		wantStderr string
	}{
		{
			name:       "history",
			args:       []string{"--since", "2026-01", "--until", "2026-02"},
			wantStdout: "Net worth at each month end 2026-01-31 to 2026-02-28, amounts in CAD\n\nMonth end   Total\n2026-01-31\n2026-02-28\n",
			wantStderr: beforeHistoryWarning,
		},
		{
			name:       "native snapshot",
			args:       []string{"--as-of", "2026-03-01", "--currency", "native"},
			wantStdout: "Net worth on 2026-03-01\n\nType  Currency  Balance\n",
			wantStderr: beforeSnapshotWarning,
		},
		{
			name:       "native history",
			args:       []string{"--since", "2026-01", "--until", "2026-02", "--currency", "native"},
			wantStdout: "Net worth at each month end 2026-01-31 to 2026-02-28\n\nMonth end   Currency  Total\n2026-01-31\n2026-02-28\n",
			wantStderr: beforeHistoryWarning,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedNetWorthStore(t)
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), append([]string{"networth"}, c.args...),
				spendEnvAt(&stdout, &stderr, holdingsClock()))

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, c.wantStdout, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_networth_json_before_the_first_transaction_keeps_the_empty_dates_and_carries_the_warning(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		want     map[string]any
		wantLine string
	}{
		{
			name: "snapshot",
			args: []string{"--as-of", "2026-03-01"},
			want: map[string]any{
				"as_of": "2026-03-01", "since": nil, "until": nil,
				"dates": []any{map[string]any{"date": "2026-03-01", "balances": []any{}, "totals": []any{}}},
			},
			wantLine: beforeSnapshotLine,
		},
		{
			name: "history",
			args: []string{"--since", "2026-01", "--until", "2026-02"},
			want: map[string]any{
				"as_of": nil, "since": "2026-01-01", "until": "2026-02-28",
				"dates": []any{
					map[string]any{"date": "2026-01-31", "balances": []any{}, "totals": []any{}},
					map[string]any{"date": "2026-02-28", "balances": []any{}, "totals": []any{}},
				},
			},
			wantLine: beforeHistoryLine,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedNetWorthStore(t)
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), append([]string{"networth", "--json"}, c.args...),
				spendEnvAt(&stdout, &stderr, holdingsClock()))

			require.Equal(t, 0, exitCode, stderr.String())
			var got map[string]any
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
			c.want["currency"] = "CAD"
			c.want["warnings"] = []any{c.wantLine}
			assert.Equal(t, c.want, got)
			assert.Equal(t, "quarry: warning: "+c.wantLine+"\n", stderr.String())
		})
	}
}

func Test_run_networth_says_no_account_in_the_reports_has_data_when_only_an_uncounted_account_does(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantWarn   string
		wantStdout string
	}{
		{
			name:       "snapshot",
			args:       []string{"--as-of", "2026-03-01"},
			wantWarn:   "quarry: warning: no account has a balance on 2026-03-01; no account in Quicken's reports has transactions or holdings\n",
			wantStdout: "Net worth on 2026-03-01, amounts in CAD\n\nType  Currency  Balance  In CAD\n",
		},
		{
			name: "history",
			args: []string{"--since", "2026-01", "--until", "2026-02"},
			wantWarn: "quarry: warning: no account has a balance at any month end from 2026-01-31 to 2026-02-28; " +
				"no account in Quicken's reports has transactions or holdings\n",
			wantStdout: "Net worth at each month end 2026-01-31 to 2026-02-28, amounts in CAD\n\nMonth end   Total\n2026-01-31\n2026-02-28\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedUncountedOnlyStore(t)
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), append([]string{"networth"}, c.args...),
				spendEnvAt(&stdout, &stderr, holdingsClock()))

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, c.wantStdout, stdout.String())
			assert.Equal(t, c.wantWarn, stderr.String())
		})
	}
}

func Test_run_networth_on_the_first_balance_day_does_not_say_no_account_has_a_balance(t *testing.T) {
	seedNetWorthStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2026-03-02"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "chequing")
	assert.NotContains(t, stderr.String(), "no account has a balance")
}
