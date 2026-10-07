package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acbToolConfig = "[accounts]\nnon-registered = [\"acct-cad\", \"acct-usd\"]\nregistered = [\"acct-rrsp\"]\n"

const acbToolAdjustmentConfig = acbToolConfig + `
[[acb.adjustment]]
security = "sec-acme"
date = 2025-01-15
return-of-capital = 100.00
`

const acbToolReinvestedConfig = acbToolConfig + `
[[acb.adjustment]]
security = "sec-acme"
date = 2025-01-15
reinvested-distribution = 100.00
`

// acbToolRows is acbRows plus two securities in acct-cad that took in shares with no cost: one added, one reinvested.
func acbToolRows() store.Rows {
	rows := acbRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-gift", SourceID: 4, Name: "Gift Fund", Ticker: new("GIFT"), Currency: new("CAD")},
		store.Security{ID: "sec-drip", SourceID: 5, Name: "Drip Fund", Ticker: new("DRIP"), Currency: new("CAD")},
	)
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		acbTrade("inv-gift-add", 8, "acct-cad", "sec-gift", store.ActionAddShares, "CAD", day(2025, time.February, 3), 5_000_000, 0),
		acbTrade("inv-drip-reinvest", 9, "acct-cad", "sec-drip", store.ActionReinvestDividend, "CAD", day(2025, time.March, 1), 1_000_000, 0),
	)
	return rows
}

func seedACBToolStore(t *testing.T, home string) {
	t.Helper()
	replaceStoreWithRates(t, home, acbToolRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))
}

func Test_run_mcp_acb_returns_the_acb_json_document(t *testing.T) {
	cases := []struct {
		name      string
		config    string
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "none given", config: acbToolConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{name: "year given", config: acbToolConfig, cliArgs: []string{"acb", "--year", "2025"}, arguments: map[string]any{"year": 2025}},
		{
			name: "security given", config: acbToolConfig,
			cliArgs: []string{"acb", "--security", "sec-drip"}, arguments: map[string]any{"security": []string{"sec-drip"}},
		},
		{
			name: "year and security given", config: acbToolConfig,
			cliArgs:   []string{"acb", "--year", "2025", "--security", "sec-gift"},
			arguments: map[string]any{"year": 2025, "security": []string{"sec-gift"}},
		},
		{name: "one adjustment configured", config: acbToolAdjustmentConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{name: "one reinvested distribution configured", config: acbToolReinvestedConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: seedACBToolStore, config: c.config, cliArgs: c.cliArgs, tool: "acb", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, acbInToolWords(got.cliWarnings), got.toolWarnings)
		})
	}
}

func Test_run_mcp_acb_lists_a_reinvested_distribution_as_an_event_that_raises_the_acb(t *testing.T) {
	got := runBothSurfaces(t, toolDocumentRun{
		store: seedACBToolStore, config: acbToolReinvestedConfig, cliArgs: []string{"acb", "--security", "sec-acme"}, tool: "acb",
		arguments: map[string]any{"security": []string{"sec-acme"}},
	})

	type event struct {
		Action string `json:"action"`
		CAD    string `json:"cad"`
		ACB    string `json:"acb"`
	}
	var doc struct {
		Securities []struct {
			Events []event `json:"events"`
		} `json:"securities"`
	}
	require.NoError(t, json.Unmarshal([]byte(got.toolBody), &doc), got.toolBody)
	require.Len(t, doc.Securities, 1)
	assert.Contains(t, doc.Securities[0].Events, event{Action: "reinvested distribution", CAD: "-100.00", ACB: "1700.00"})
}

// acbRefusals are both surfaces' answers to one refused acb call: the CLI's refusal without its "quarry: " prefix,
// and the tool's text and stderr.
type acbRefusals struct{ cli, tool, toolStderr string }

// acbRefusedPair runs the CLI acb with cliArgs and the acb tool with arguments over one store and config, each refused.
func acbRefusedPair(t *testing.T, accounts []store.Account, cliArgs []string, arguments map[string]any) acbRefusals {
	t.Helper()
	home := newHome(t)
	writeConfig(t, home, acbPooledConfig)
	replaceStoreWithRates(t, home, acbUnclassifiedRows(accounts...))
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 1, runWith(ctx, cliArgs, spendEnvAt(&stdout, &stderr, toolClock)))
	peer := startClockedMCP(ctx, t)

	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "acb", Arguments: arguments})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.True(t, result.IsError)
	return acbRefusals{
		cli:        strings.TrimSuffix(strings.TrimPrefix(stderr.String(), "quarry: "), "\n"),
		tool:       textOf(result),
		toolStderr: peer.stderr.String(),
	}
}

func Test_run_mcp_acb_refuses_unclassified_accounts_in_the_clis_words_naming_the_tool_that_lists_them(t *testing.T) {
	accounts := []store.Account{acbOpenBrokerage("acct-unc", 2), acbClosedRetirement("acct-old", 3)}

	got := acbRefusedPair(t, accounts, []string{"acb"}, map[string]any{})

	want := strings.Replace(got.cli, "quarry findings --type unclassified-account --status all", "data_quality with type unclassified-account and status all", 1)
	assert.Contains(t, got.cli, "2 accounts are in neither")
	assert.Equal(t, want, got.tool)
	assert.Equal(t, "quarry: mcp: acb: "+want+"\n", got.toolStderr)
}

func Test_run_mcp_acb_refuses_an_unknown_security_in_the_clis_words_without_the_name_on_stderr(t *testing.T) {
	got := acbRefusedPair(t, nil, []string{"acb", "--security", "XYZ"}, map[string]any{"security": []string{"XYZ"}})

	want := strings.Replace(got.cli, "quarry acb --json", "acb with no arguments", 1)
	assert.Contains(t, got.cli, `no security named "XYZ"`)
	assert.Equal(t, want, got.tool)
	assert.Equal(t, "quarry: mcp: acb: refused the call's security; details went to the client only\n", got.toolStderr)
}

// acbToolTwinRows is acbToolRows plus Acme Twin, which shares Acme's ticker.
func acbToolTwinRows() store.Rows {
	rows := acbToolRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-twin", SourceID: 6, Name: "Acme Twin", Ticker: new("ACME"), Currency: new("CAD")})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		acbTrade("inv-twin-buy", 10, "acct-cad", "sec-twin", store.ActionBuy, "CAD", day(2025, time.April, 1), 3_000_000, -30_000))
	return rows
}

func Test_run_mcp_acb_warns_of_securities_the_security_param_leaves_out(t *testing.T) {
	const (
		twins = `"ACME" is 2 securities in Quicken ("Acme Corp", "Acme Twin"); quarry keeps a separate ACB for each; if they are the same, merge them in Quicken`
		drip  = `"Drip Fund" has reinvested dividends with no cost, so its ACB is too low and its gains too high; ` +
			"enter their cost in Quicken; acb with security sec-drip lists them"
		gift = `"Gift Fund" has shares added with no cost, so its ACB is too low and its gains too high; ` +
			"data_quality with type shares-without-cost lists them"
		maple = `"Maple Fund" is held only in registered accounts, so it has no ACB`
	)
	cases := []struct {
		name     string
		security string
		want     []string
	}{
		{name: "one named security leaves the others' lines in", security: "sec-acme", want: []string{drip, gift, twins}},
		{name: "a registered-only security is named", security: "sec-maple", want: []string{maple, drip, gift, twins}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: func(t *testing.T, home string) {
					t.Helper()
					replaceStoreWithRates(t, home, acbToolTwinRows(), usdRate(day(2024, time.January, 2), 1_250_000))
				},
				config: acbToolConfig, cliArgs: []string{"acb", "--security", c.security}, tool: "acb",
				arguments: map[string]any{"security": []string{c.security}},
			})

			assert.Equal(t, c.want, got.toolWarnings)
		})
	}
}

func Test_run_mcp_acb_keeps_the_document_key_orders_the_cli_prints(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
	}{
		{
			name:    "a year ends with its gain, the return of capital gain, then the superficial losses",
			pattern: `"gain":"[^"]*","return_of_capital_gain":"0.00","possible_superficial_losses":\d+,"unknown_cost_sales"`,
		},
		{name: "an event ends with unknown_cost", pattern: `"gain":(null|"[^"]*"),"unknown_cost":(true|false)\}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: seedACBToolStore, config: acbToolConfig, cliArgs: []string{"acb"}, tool: "acb", arguments: map[string]any{},
			})

			assert.Regexp(t, c.pattern, got.toolBody)
		})
	}
}

func Test_run_mcp_acb_matches_the_cli_on_each_edge_row_with_and_without_year_and_security(t *testing.T) {
	closedAccount := func(t *testing.T, home string) {
		t.Helper()
		rows := acbToolRows()
		rows.Accounts[0].Closed = true
		replaceStoreWithRates(t, home, rows, usdRate(day(2024, time.January, 2), 1_250_000))
	}
	noRates := func(t *testing.T, home string) {
		t.Helper()
		replaceStoreWithRates(t, home, acbToolRows())
	}
	const returnOfCapitalOnly = acbToolConfig + `
[[acb.adjustment]]
security = "sec-acme"
date = 2024-12-01
return-of-capital = 5000.00
`
	cases := []struct {
		name      string
		store     func(t *testing.T, home string)
		config    string
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "a closed account, none given", store: closedAccount, config: acbToolConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{
			name: "a closed account, year given", store: closedAccount, config: acbToolConfig,
			cliArgs: []string{"acb", "--year", "2025"}, arguments: map[string]any{"year": 2025},
		},
		{
			name: "a closed account, security given", store: closedAccount, config: acbToolConfig,
			cliArgs: []string{"acb", "--security", "sec-acme"}, arguments: map[string]any{"security": []string{"sec-acme"}},
		},
		{name: "a USD trade with no rate, none given", store: noRates, config: acbToolConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{
			name: "a USD trade with no rate, year given", store: noRates, config: acbToolConfig,
			cliArgs: []string{"acb", "--year", "2026"}, arguments: map[string]any{"year": 2026},
		},
		{
			name: "a USD trade with no rate, security given", store: noRates, config: acbToolConfig,
			cliArgs: []string{"acb", "--security", "sec-vti"}, arguments: map[string]any{"security": []string{"sec-vti"}},
		},
		{
			name: "a year with only a return of capital gain, none given", store: seedACBToolStore, config: returnOfCapitalOnly,
			cliArgs: []string{"acb"}, arguments: map[string]any{},
		},
		{
			name: "a year with only a return of capital gain, year given", store: seedACBToolStore, config: returnOfCapitalOnly,
			cliArgs: []string{"acb", "--year", "2024"}, arguments: map[string]any{"year": 2024},
		},
		{
			name: "a year with only a return of capital gain, security given", store: seedACBToolStore, config: returnOfCapitalOnly,
			cliArgs: []string{"acb", "--security", "sec-acme"}, arguments: map[string]any{"security": []string{"sec-acme"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{store: c.store, config: c.config, cliArgs: c.cliArgs, tool: "acb", arguments: c.arguments})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, acbInToolWords(got.cliWarnings), got.toolWarnings)
		})
	}
}

func Test_run_mcp_acb_opens_each_adjustment_warning_with_the_absolute_config_path(t *testing.T) {
	cases := []struct {
		name       string
		adjustment string
		want       string
	}{
		{
			name:       "a security the store does not have",
			adjustment: "security = \"sec-99\"\ndate = 2024-06-30\n",
			want:       `acb.adjustment item 1 names "sec-99", which is not a security in quarry's store; quarry skips it`,
		},
		{
			name:       "a security no non-registered account holds on the date",
			adjustment: "security = \"sec-acme\"\ndate = 2023-01-01\n",
			want:       `acb.adjustment item 1 is for "Acme Corp", which no non-registered account holds on 2023-01-01; quarry skips it`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: seedACBToolStore, config: acbToolConfig + "\n[[acb.adjustment]]\n" + c.adjustment + "return-of-capital = 5.00\n",
				cliArgs: []string{"acb"}, tool: "acb", arguments: map[string]any{},
			})

			require.NotEmpty(t, got.toolWarnings)
			assert.Equal(t, configPath(os.Getenv("HOME"))+": "+c.want, got.toolWarnings[0])
			assert.Equal(t, acbInToolWords(got.cliWarnings), got.toolWarnings)
		})
	}
}

func Test_run_mcp_acb_warns_of_a_sale_beyond_the_pool_in_the_cli_words(t *testing.T) {
	got := runBothSurfaces(t, toolDocumentRun{
		store: func(t *testing.T, home string) {
			t.Helper()
			replaceStore(t, home, shortOpenRows())
		},
		config: "[accounts]\nnon-registered = [\"acct-cad\"]\n", cliArgs: []string{"acb"}, tool: "acb", arguments: map[string]any{},
	})

	assert.Equal(t, []string{moneyFundShortWarning}, got.toolWarnings)
	assert.Equal(t, got.cliWarnings, got.toolWarnings)
	assert.Equal(t, got.cliBody, got.toolBody)
}

const (
	holdingsToolRate  = 1_360_000
	holdingsLogPrefix = "quarry: mcp: holdings: "
	asOfRefusedLog    = "refused the call's as_of; details went to the client only"
)

// seedHoldingsToolStore stores holdingsRows with one USD rate, under the HOME runBothSurfaces set.
func seedHoldingsToolStore(t *testing.T, home string) {
	t.Helper()
	replaceStoreWithRates(t, home, holdingsRows(), usdRate(holdingsDay(10), holdingsToolRate))
}

func Test_run_mcp_holdings_returns_the_holdings_json_document(t *testing.T) {
	cases := []struct {
		name      string
		store     func(*testing.T, string)
		cliArgs   []string
		arguments map[string]any
	}{
		{
			name: "as_of and currency given", store: seedHoldingsToolStore,
			cliArgs:   []string{"holdings", "--as-of", "2026-03-12", "--currency", "CAD"},
			arguments: map[string]any{"as_of": "2026-03-12", "currency": "CAD"},
		},
		{name: "none given", store: seedHoldingsToolStore, cliArgs: []string{"holdings"}, arguments: map[string]any{}},
		{
			name: "a year that holds today", store: seedHoldingsToolStore,
			cliArgs: []string{"holdings", "--as-of", "2026"}, arguments: map[string]any{"as_of": "2026"},
		},
		{
			name: "no accounts on a day nothing is held", store: seedHoldingsToolStore,
			cliArgs: []string{"holdings", "--as-of", "2025"}, arguments: map[string]any{"as_of": "2025", "accounts": []string{}},
		},
		{
			name: "no accounts named on a day with holdings", store: seedHoldingsToolStore,
			cliArgs: []string{"holdings", "--as-of", "2026-03-12"}, arguments: map[string]any{"as_of": "2026-03-12", "accounts": []string{}},
		},
		{
			name: "one account in native currencies", store: seedHoldingsToolStore,
			cliArgs:   []string{"holdings", "--as-of", "2026-03-12", "--account", "Brokerage", "--currency", "native"},
			arguments: map[string]any{"as_of": "2026-03-12", "accounts": []string{"Brokerage"}, "currency": "native"},
		},
		{
			name: "a non-investment account", store: func(t *testing.T, home string) {
				t.Helper()
				rows := holdingsRows()
				rows.Accounts = append(rows.Accounts, chequingAccount("acct-chq", 4))
				replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), holdingsToolRate))
			},
			cliArgs:   []string{"holdings", "--as-of", "2026-03-12", "--account", "Chequing"},
			arguments: map[string]any{"as_of": "2026-03-12", "accounts": []string{"Chequing"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, cliArgs: c.cliArgs, tool: "holdings", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, got.cliWarnings, got.toolWarnings)
		})
	}
}

func Test_run_mcp_holdings_refuses_an_as_of_it_cannot_use_in_mcp_words(t *testing.T) {
	cases := []struct {
		name string
		asOf string
		want string
	}{
		{name: "not a date", asOf: "2024-13", want: `as_of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{
			name: "after today", asOf: "2027-01-01",
			want: "as_of 2027-01-01 is after today; holdings are valued up to today only, so pass an earlier as_of",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			seedHoldingsToolStore(t, home)
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "holdings", Arguments: map[string]any{"as_of": c.asOf}})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, holdingsLogPrefix+asOfRefusedLog+"\n", peer.stderr.String())
		})
	}
}

func Test_run_mcp_holdings_refuses_an_account_without_its_name_on_stderr(t *testing.T) {
	refuseAccountKeepingItsNameOffStderr(t, "holdings", holdingsLogPrefix)
}

const netWorthLogPrefix = "quarry: mcp: net_worth: "

// netWorthSeeder is seed as the store hook of a toolDocumentRun; seed sets the HOME both surfaces read.
func netWorthSeeder(seed func(*testing.T)) func(*testing.T, string) {
	return func(t *testing.T, _ string) {
		t.Helper()
		seed(t)
	}
}

func Test_run_mcp_net_worth_returns_the_networth_json_document(t *testing.T) {
	cases := []struct {
		name      string
		seed      func(*testing.T)
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "today by default", seed: seedNetWorthStore, cliArgs: []string{"networth"}, arguments: map[string]any{}},
		{
			name: "month ends from since to until", seed: seedNetWorthHistoryStore,
			cliArgs:   []string{"networth", "--since", "2026-01", "--until", "2026-03"},
			arguments: map[string]any{"since": "2026-01", "until": "2026-03"},
		},
		{
			name: "month ends from since to today", seed: seedNetWorthHistoryStore,
			cliArgs: []string{"networth", "--since", "2026-01"}, arguments: map[string]any{"since": "2026-01"},
		},
		{
			name: "one day in native currencies", seed: seedNetWorthStore,
			cliArgs:   []string{"networth", "--as-of", "2026-03-12", "--currency", "native"},
			arguments: map[string]any{"as_of": "2026-03-12", "currency": "native"},
		},
		{
			name: "month ends in native currencies", seed: seedNetWorthHistoryStore,
			cliArgs:   []string{"networth", "--since", "2026-01", "--until", "2026-03", "--currency", "native"},
			arguments: map[string]any{"since": "2026-01", "until": "2026-03", "currency": "native"},
		},
		{
			name: "month ends before the first balance", seed: seedNetWorthStore,
			cliArgs:   []string{"networth", "--since", "2026-01", "--until", "2026-02"},
			arguments: map[string]any{"since": "2026-01", "until": "2026-02"},
		},
		{
			name: "one day before the first balance", seed: seedNetWorthStore,
			cliArgs: []string{"networth", "--as-of", "2026-03-01"}, arguments: map[string]any{"as_of": "2026-03-01"},
		},
		{
			name: "no account in the reports has a balance", seed: seedUncountedOnlyStore,
			cliArgs: []string{"networth", "--as-of", "2026-03-01"}, arguments: map[string]any{"as_of": "2026-03-01"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: netWorthSeeder(c.seed), cliArgs: c.cliArgs, tool: "net_worth", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, got.cliWarnings, got.toolWarnings)
		})
	}
}

func Test_run_mcp_net_worth_words_the_rate_advice_for_a_tool_parameter_not_a_flag(t *testing.T) {
	cases := []struct {
		name      string
		seed      func(*testing.T)
		cliArgs   []string
		arguments map[string]any
		wantCLI   string
		wantTool  string
	}{
		{
			name: "one day with a USD balance before the first rate", seed: seedNetWorthStore,
			cliArgs: []string{"networth", "--as-of", "2026-03-05"}, arguments: map[string]any{"as_of": "2026-03-05"},
			wantCLI:  netWorthBeforeFirstRateLine,
			wantTool: "USD balances on 2026-03-05, before 2026-03-10, the first exchange rate in the store, are not converted to CAD and are left out of the CAD total; pass currency native to list them",
		},
		{
			name: "CAD balances in USD with no exchange rate", seed: seedNetWorthCADOnlyStore,
			cliArgs:   []string{"networth", "--as-of", "2026-01-20", "--currency", "USD"},
			arguments: map[string]any{"as_of": "2026-01-20", "currency": "USD"},
			wantCLI:   "the store has no exchange rates, so CAD balances are not converted to USD and are left out of the USD total; pass --currency native to list them, or run quarry sync to fetch rates",
			wantTool:  "the store has no exchange rates, so CAD balances are not converted to USD and are left out of the USD total; pass currency native to list them, or run quarry sync to fetch rates",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: netWorthSeeder(c.seed), cliArgs: c.cliArgs, tool: "net_worth", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, []string{c.wantCLI}, got.cliWarnings)
			assert.Equal(t, []string{c.wantTool}, got.toolWarnings)
		})
	}
}

func Test_run_mcp_net_worth_returns_the_left_out_holding_warnings_the_cli_prints(t *testing.T) {
	seed := func(t *testing.T, home string) {
		t.Helper()
		rows := noRateHoldingRows()
		rows.Securities = append(rows.Securities,
			store.Security{ID: "sec-bare", SourceID: 5, Name: "Bare Fund", Ticker: new("BARE"), Currency: new("CAD")})
		rows.InvestmentTransactions = append(rows.InvestmentTransactions,
			holdingsBuy("inv-bare", 5, "acct-cad", "sec-bare", "CAD", 40_000_000))
		replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	}

	got := runBothSurfaces(t, toolDocumentRun{
		store: seed, cliArgs: []string{"networth", "--as-of", "2026-03-05"}, tool: "net_worth",
		arguments: map[string]any{"as_of": "2026-03-05"},
	})

	assert.Equal(t, got.cliBody, got.toolBody)
	assert.Equal(t, got.cliWarnings, got.toolWarnings)
	assert.Equal(t, []string{
		`"Brokerage" holds 2 securities with no price on or before 2026-03-05, so its balance leaves them out; enter prices in Quicken, then run quarry sync`,
		`"IRA" holds 1 security with no price on or before 2026-03-05, so its balance leaves it out; enter a price in Quicken, then run quarry sync`,
		`"Brokerage" holds 1 USD security valued on 2026-03-05, before 2026-03-10, the first exchange rate in the store, so its CAD balance leaves it out`,
	}, got.toolWarnings)
}

func Test_run_mcp_net_worth_refuses_a_call_it_cannot_value_in_mcp_words(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
		want      string
		wantLog   string
	}{
		{
			name: "as_of after today", arguments: map[string]any{"as_of": "2027-01-01"},
			want:    "as_of 2027-01-01 is after today; net worth is valued up to today only, so pass an earlier as_of",
			wantLog: asOfRefusedLog,
		},
		{
			name: "since after today", arguments: map[string]any{"since": "2027-01-01"},
			want:    "since 2027-01-01 is after today; net worth is valued up to today only, so pass an earlier since",
			wantLog: windowRefusedLog,
		},
		{
			name: "as_of beside since", arguments: map[string]any{"as_of": "2026-03-12", "since": "2026-01"},
			want:    "as_of cannot be combined with since or until; pass as_of for one day, or since and until for month ends",
			wantLog: asOfRefusedLog,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedNetWorthStore(t)
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "net_worth", Arguments: c.arguments})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, netWorthLogPrefix+c.wantLog+"\n", peer.stderr.String())
		})
	}
}
