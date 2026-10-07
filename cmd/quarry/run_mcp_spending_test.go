package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	spendingLogPrefix      = "quarry: mcp: spending: "
	spendingConfigLog      = "cannot read quarry's config file; run quarry spend to see why"
	unparseableConfig      = "snapshots.keep = 0\n"
	linkedLineForSpending  = `account "Linked" uses linked account tracking in Quicken, so spending leaves it out, as Quicken's reports do`
	oldCardLineForSpending = `account "Old Card" is not used in reports in Quicken, so spending leaves it out; ` +
		`to include it, turn on reports for it in Quicken's account settings, then run quarry sync`
	spendingBeforeFirstRateLine = "3 transactions dated before 2026-03-01, the first exchange rate in the store, are listed in USD, not converted to CAD"
	unknownKeyConfig            = "bogus = 1\n"
	windowRefusedLog            = "refused the call's since or until; details went to the client only"
	unknownAccountLog           = "refused the call's accounts: one names no account; details went to the client only"
	ambiguousAccountLog         = "refused the call's accounts: one names more than one account; details went to the client only"
)

func Test_run_mcp_spending_returns_the_spend_json_document(t *testing.T) {
	cases := []struct {
		name      string
		store     func(*testing.T, string)
		cliArgs   []string
		arguments map[string]any
	}{
		{
			name: "all five given", store: populatedAnalysisStore,
			cliArgs: []string{
				"spend", "--since", "2026-01", "--until", "2026-08", "--by", "payee", "--currency", "CAD",
				"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card",
			},
			arguments: map[string]any{
				"since": "2026-01", "until": "2026-08", "by": "payee", "currency": "CAD",
				"accounts": []string{"Chequing", "US Chequing", "Linked", "Old Card"},
			},
		},
		{name: "none given", store: populatedAnalysisStore, cliArgs: []string{"spend"}, arguments: map[string]any{}},
		{
			name: "native currency", store: populatedAnalysisStore,
			cliArgs: []string{"spend", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{
			name: "grouped by tag", store: multiTagAnalysisStore,
			cliArgs: []string{"spend", "--by", "tag"}, arguments: map[string]any{"by": "tag"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, cliArgs: c.cliArgs, tool: "spending", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, inToolWords(got.cliWarnings, "spend", "spending"), got.toolWarnings)
		})
	}
}

func Test_run_mcp_spending_reads_the_config_only_when_currency_is_absent(t *testing.T) {
	cases := []struct {
		name         string
		config       string
		cliArgs      []string
		arguments    map[string]any
		wantCurrency string
	}{
		{name: "currency absent, config says USD", config: `reporting.currency = "USD"` + "\n", cliArgs: []string{"spend"}, arguments: map[string]any{}, wantCurrency: "USD"},
		{name: "currency absent, no config", cliArgs: []string{"spend"}, arguments: map[string]any{}, wantCurrency: "CAD"},
		{
			name: "currency given, config unparseable", config: unparseableConfig,
			cliArgs: []string{"spend", "--currency", "CAD"}, arguments: map[string]any{"currency": "CAD"}, wantCurrency: "CAD",
		},
		{name: "currency absent, config with an unknown key", config: unknownKeyConfig, cliArgs: []string{"spend"}, arguments: map[string]any{}, wantCurrency: "CAD"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: populatedAnalysisStore, config: c.config, cliArgs: c.cliArgs, tool: "spending", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, inToolWords(got.cliWarnings, "spend", "spending"), got.toolWarnings)
			var doc struct {
				Currency string `json:"currency"`
			}
			require.NoError(t, json.Unmarshal([]byte(got.toolBody), &doc))
			assert.Equal(t, c.wantCurrency, doc.Currency)
		})
	}

	t.Run("currency absent, config unparseable", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		populatedAnalysisStore(t, home)
		writeConfig(t, home, unparseableConfig)
		ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
		defer cancel()
		var cliStdout, cliStderr bytes.Buffer
		require.Equal(t, 1, runWith(ctx, []string{"spend"}, spendEnvAt(&cliStdout, &cliStderr, toolClock)))
		refusal := strings.TrimSuffix(strings.TrimPrefix(cliStderr.String(), "quarry: "), "\n")
		peer := startClockedMCP(ctx, t)

		result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "spending", Arguments: map[string]any{}})
		require.NoError(t, err)
		require.NoError(t, peer.session.Close())
		peer.waitForExit(ctx, t)

		assert.True(t, result.IsError)
		assert.Equal(t, refusal, textOf(result))
		assert.Contains(t, refusal, configShown)
		assert.Equal(t, spendingLogPrefix+spendingConfigLog+"\n", peer.stderr.String())
	})
}

func Test_run_mcp_spending_refuses_a_bad_window_in_mcp_words(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
		want      string
	}{
		{
			name: "since is not a date", arguments: map[string]any{"since": "2024-13"},
			want: `since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			name: "since is empty", arguments: map[string]any{"since": ""},
			want: `since "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			name: "until is not a date", arguments: map[string]any{"until": "2024-13"},
			want: `until "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			name: "since is after until", arguments: map[string]any{"since": "2025", "until": "2024"},
			want: "since 2025 is after until 2024",
		},
		{
			name: "until is before the default since", arguments: map[string]any{"until": "2025-03"},
			want: "until 2025-03 is before the default since 2026-01-01; pass since too",
		},
		{
			name: "since is in the future with no until", arguments: map[string]any{"since": "2099"},
			want: "since 2099 is after today; pass until to include future-dated transactions",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			populatedAnalysisStore(t, home)
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "spending", Arguments: c.arguments})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, spendingLogPrefix+windowRefusedLog+"\n", peer.stderr.String())
		})
	}
}

func Test_run_mcp_spending_refuses_an_account_without_its_name_on_stderr(t *testing.T) {
	refuseAccountKeepingItsNameOffStderr(t, "spending", spendingLogPrefix)
}

func Test_run_mcp_spending_words_its_warnings_with_the_tool_name(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	populatedAnalysisStore(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startClockedMCP(ctx, t)

	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{
		Name: "spending", Arguments: map[string]any{"accounts": []string{"Linked", "Old Card", "US Chequing"}},
	})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.False(t, result.IsError, textOf(result))
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	assert.Equal(t, []string{linkedLineForSpending, oldCardLineForSpending, spendingBeforeFirstRateLine}, doc.Warnings)
}

func Test_tool_warning_mapping_rewrites_only_the_left_out_template(t *testing.T) {
	cliWarnings := []string{
		`account "Visa" uses linked account tracking in Quicken, so spend leaves it out, as Quicken's reports do`,
		`account "spend" is not used in reports in Quicken, so spend leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync`,
		"3 transactions dated before 2017-01-03, the first exchange rate in the store, are listed in USD, not converted to CAD",
		"no spending from 2026-01-01 to 2026-10-03; the store's transactions run 2003-01-02 to 2026-09-30",
	}

	mapped := inToolWords(cliWarnings, "spend", "spending")

	assert.Equal(t, []string{
		`account "Visa" uses linked account tracking in Quicken, so spending leaves it out, as Quicken's reports do`,
		`account "spend" is not used in reports in Quicken, so spending leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync`,
		cliWarnings[2],
		cliWarnings[3],
	}, mapped)
	assert.NotEqual(t, cliWarnings, mapped)
}

const (
	rowCap          = 500
	spendingCapLine = "spending lists the first 500 rows of 501; totals count every row; pass a shorter period or fewer accounts, or query v_spending for the rest"
)

// payeeStore is a store with one charge from each of n distinct payees.
func payeeStore(n int) func(*testing.T, string) {
	return func(t *testing.T, home string) {
		t.Helper()
		charges := make([]chargeTxn, n)
		for i := range charges {
			charges[i] = groceryCharge(fmt.Sprintf("Payee %03d", i), day(2026, time.March, 1+i%28), int64(1000+i))
		}
		replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	}
}

// decodeSpendingBody is a compact spending document, as runBothSurfaces returns it, decoded.
func decodeSpendingBody(t *testing.T, body string) document.Spending {
	t.Helper()
	var doc document.Spending
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	return doc
}

func Test_run_mcp_spending_cuts_a_list_over_500_rows_with_the_cap_warning(t *testing.T) {
	got := runBothSurfaces(t, toolDocumentRun{
		store: payeeStore(rowCap + 1), cliArgs: []string{"spend", "--by", "payee"},
		tool: "spending", arguments: map[string]any{"by": "payee"},
	})

	cli, tool := decodeSpendingBody(t, got.cliBody), decodeSpendingBody(t, got.toolBody)

	require.Len(t, cli.Rows, rowCap+1)
	assert.Equal(t, cli.Rows[:rowCap], tool.Rows)
	assert.Equal(t, cli.Totals, tool.Totals)
	require.NotEmpty(t, got.toolWarnings)
	assert.Equal(t, spendingCapLine, got.toolWarnings[len(got.toolWarnings)-1])
	assert.Equal(t, inToolWords(got.cliWarnings, "spend", "spending"), got.toolWarnings[:len(got.toolWarnings)-1])
}

func Test_run_mcp_spending_leaves_a_list_of_500_rows_uncut_and_adds_no_cap_warning(t *testing.T) {
	got := runBothSurfaces(t, toolDocumentRun{
		store: payeeStore(rowCap), cliArgs: []string{"spend", "--by", "payee"},
		tool: "spending", arguments: map[string]any{"by": "payee"},
	})

	assert.Equal(t, got.cliBody, got.toolBody)
	assert.Len(t, decodeSpendingBody(t, got.toolBody).Rows, rowCap)
	assert.NotContains(t, got.toolWarnings, spendingCapLine)
	assert.Equal(t, inToolWords(got.cliWarnings, "spend", "spending"), got.toolWarnings)
}

const (
	cashFlowLogPrefix      = "quarry: mcp: cash_flow: "
	linkedLineForCashFlow  = `account "Linked" uses linked account tracking in Quicken, so cash_flow leaves it out, as Quicken's reports do`
	oldCardLineForCashFlow = `account "Old Card" is not used in reports in Quicken, so cash_flow leaves it out; ` +
		`to include it, turn on reports for it in Quicken's account settings, then run quarry sync`
)

func Test_run_mcp_cash_flow_returns_the_cashflow_json_document(t *testing.T) {
	cases := []struct {
		name      string
		cliArgs   []string
		arguments map[string]any
	}{
		{
			name: "all five given",
			cliArgs: []string{
				"cashflow", "--since", "2026-01", "--until", "2026-08", "--by", "month", "--currency", "CAD",
				"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card",
			},
			arguments: map[string]any{
				"since": "2026-01", "until": "2026-08", "by": "month", "currency": "CAD",
				"accounts": []string{"Chequing", "US Chequing", "Linked", "Old Card"},
			},
		},
		{name: "none given", cliArgs: []string{"cashflow"}, arguments: map[string]any{}},
		{
			name:    "native currency",
			cliArgs: []string{"cashflow", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{
			name:    "grouped by year",
			cliArgs: []string{"cashflow", "--by", "year"}, arguments: map[string]any{"by": "year"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: populatedAnalysisStore, cliArgs: c.cliArgs, tool: "cash_flow", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, inToolWords(got.cliWarnings, "cashflow", "cash_flow"), got.toolWarnings)
		})
	}
}

func Test_run_mcp_cash_flow_refuses_an_account_without_its_name_on_stderr(t *testing.T) {
	refuseAccountKeepingItsNameOffStderr(t, "cash_flow", cashFlowLogPrefix)
}

func Test_run_mcp_cash_flow_words_its_left_out_warnings_with_the_tool_name(t *testing.T) {
	got := runBothSurfaces(t, toolDocumentRun{
		store: populatedAnalysisStore, tool: "cash_flow",
		cliArgs:   []string{"cashflow", "--account", "Linked", "--account", "Old Card"},
		arguments: map[string]any{"accounts": []string{"Linked", "Old Card"}},
	})

	assert.Equal(t, []string{linkedLineForCashFlow, oldCardLineForCashFlow}, got.toolWarnings)
}

// seedSummaryStoreAt builds the September summary store into home twice, so the second build fixes Kiosk's finding
// and finds Pharmacy's while Shell's carries.
func seedSummaryStoreAt(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, summaryRows(false))
	replaceStore(t, home, summaryRows(true))
}

// seedSummaryNativeStoreAt builds summaryNativeRows, which holds no exchange rates, into home.
func seedSummaryNativeStoreAt(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, summaryNativeRows())
}

// seedSummaryRatedStoreAt builds summaryNativeRows into home with a USD rate dated January 2.
func seedSummaryRatedStoreAt(t *testing.T, home string) {
	t.Helper()
	replaceStoreWithRates(t, home, summaryNativeRows(), usdRate(day(2026, time.January, 2), 1_350_000))
}

// seedSummaryFirstMonthStoreAt builds firstMonthRows into home.
func seedSummaryFirstMonthStoreAt(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, firstMonthRows())
}

func Test_run_mcp_monthly_summary_returns_the_summary_json_document(t *testing.T) {
	cases := []struct {
		name      string
		store     func(t *testing.T, home string)
		config    string
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "last month by default", store: seedSummaryStoreAt, cliArgs: []string{"summary"}, arguments: map[string]any{}},
		{
			name: "an earlier month", store: seedSummaryStoreAt,
			cliArgs: []string{"summary", "--month", "2026-08"}, arguments: map[string]any{"month": "2026-08"},
		},
		{
			name: "native currencies", store: seedSummaryNativeStoreAt,
			cliArgs: []string{"summary", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{
			name: "USD with rates", store: seedSummaryRatedStoreAt,
			cliArgs: []string{"summary", "--currency", "USD"}, arguments: map[string]any{"currency": "USD"},
		},
		{
			name: "a config key quarry ignores", store: seedSummaryStoreAt, config: "colour = \"red\"\n",
			cliArgs: []string{"summary"}, arguments: map[string]any{},
		},
		{
			name: "an unreadable config with a currency", store: seedSummaryStoreAt, config: "[snapshots\nkeep = 24\n",
			cliArgs: []string{"summary", "--currency", "CAD"}, arguments: map[string]any{"currency": "CAD"},
		},
		{
			name: "the first month of data", store: seedSummaryFirstMonthStoreAt,
			cliArgs: []string{"summary", "--month", "2026-09"}, arguments: map[string]any{"month": "2026-09"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, config: c.config, cliArgs: c.cliArgs, tool: "monthly_summary", arguments: c.arguments, now: summaryClock,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, got.cliWarnings, got.toolWarnings)
		})
	}
}

func Test_run_mcp_monthly_summary_words_the_tool_advice_not_the_flag(t *testing.T) {
	predatesSeptember := func(t *testing.T, home string) {
		t.Helper()
		replaceStore(t, home, withSnapshotTaken(summaryRows(true), time.Date(2026, time.September, 28, 14, 2, 0, 0, time.UTC)))
	}
	const predatesLead = "the store was built from a snapshot taken 2026-09-28 14:02 UTC, before September 2026 ended, " +
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, then "
	cases := []struct {
		name      string
		store     func(t *testing.T, home string)
		cliArgs   []string
		arguments map[string]any
		wantCLI   []string
		wantTool  []string
	}{
		{
			name: "a snapshot taken before the default month ended", store: predatesSeptember,
			cliArgs: []string{"summary"}, arguments: map[string]any{},
			wantCLI:  []string{predatesLead + "run quarry summary again"},
			wantTool: []string{predatesLead + "call monthly_summary again"},
		},
		{
			name: "the same for a month the call names", store: predatesSeptember,
			cliArgs: []string{"summary", "--month", "2026-09"}, arguments: map[string]any{"month": "2026-09"},
			wantCLI:  []string{predatesLead + "run quarry summary --month 2026-09 again"},
			wantTool: []string{predatesLead + "call monthly_summary again"},
		},
		{
			name: "a store with no exchange rates", store: seedSummaryNativeStoreAt,
			cliArgs: []string{"summary"}, arguments: map[string]any{},
			wantCLI:  []string{septemberTimeUnknownText(), noRatesLine, netWorthNoRatesLine},
			wantTool: []string{septemberTimeUnknownText(), noRatesLine, strings.Replace(netWorthNoRatesLine, "--currency", "currency", 1)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, cliArgs: c.cliArgs, tool: "monthly_summary", arguments: c.arguments, now: summaryClock,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, c.wantCLI, got.cliWarnings)
			assert.Equal(t, c.wantTool, got.toolWarnings)
		})
	}
}

func Test_run_mcp_monthly_summary_refuses_a_month_in_mcp_words(t *testing.T) {
	cases := []struct {
		name  string
		month string
		want  string
	}{
		{
			name: "a month without its leading zero", month: "2026-9",
			want: `month "2026-9" is not a month; use YYYY-MM, such as 2026-09`,
		},
		{
			name: "the current month", month: "2026-10",
			want: "month 2026-10 has not ended; monthly_summary covers whole months, so pass 2026-09 or earlier",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			seedSummaryStoreAt(t, home)
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startMCPAt(ctx, t, summaryClock)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "monthly_summary", Arguments: map[string]any{"month": c.month}})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, "quarry: mcp: monthly_summary: refused the call's month; details went to the client only\n", peer.stderr.String())
		})
	}
}
