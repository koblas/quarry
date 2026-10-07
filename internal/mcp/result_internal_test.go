// White-box: logLine, errorLog and accountRefusal are unexported; handler's cancel arm needs a run failing under a cancelled context,
// the refusals they classify come from a report.Server, and the request context cannot be driven through a client.
package mcp

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errDriverInterrupt = errors.New("interrupt error: interrupted")

type stoppedCase struct {
	name    string
	stopped stoppedFunc
	want    string
}

func stoppedCases() []stoppedCase {
	return []stoppedCase{
		{
			name: "query", stopped: queryStoppedLine,
			want: "query stopped after 2 seconds; aggregate or filter it in SQL, then try again",
		},
		{name: "sync_status", stopped: stoppedLine("sync_status"), want: "sync_status stopped after 2 seconds; try again"},
	}
}

// interruptedRun is a tool run that fails as the store does once its context has ended.
func interruptedRun(ctx context.Context, _ struct{}) (any, error) {
	return nil, store.InterruptedBy(ctx, errDriverInterrupt)
}

func Test_handler_passes_a_cancel_on_as_its_own_error_not_the_timeout_line(t *testing.T) {
	for _, c := range stoppedCases() {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			call := handler(2*time.Second, c.stopped, interruptedRun)

			_, _, err := call(ctx, nil, struct{}{})

			require.ErrorIs(t, err, context.Canceled)
			require.NotErrorIs(t, err, context.DeadlineExceeded)
			assert.NotContains(t, err.Error(), "stopped after")
		})
	}
}

func Test_handler_answers_a_deadline_with_the_tools_timeout_line(t *testing.T) {
	for _, c := range stoppedCases() {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
			defer cancel()
			call := handler(2*time.Second, c.stopped, interruptedRun)

			_, _, err := call(ctx, nil, struct{}{})

			assert.EqualError(t, err, c.want)
		})
	}
}

// logged runs a tools/call for tool "query" through errorLog(w) with next answering res and recording line when it is not "".
func logged(ctx context.Context, res sdk.Result, line string) string {
	var w bytes.Buffer
	call := &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: "query"}}
	next := func(ctx context.Context, _ string, _ sdk.Request) (sdk.Result, error) {
		if line != "" {
			recordLog(ctx, line)
		}
		return res, nil
	}

	_, _ = errorLog(&w)(next)(ctx, "tools/call", call)

	return w.String()
}

func refusalResult(text string) *sdk.CallToolResult {
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: text}}}
}

func Test_errorLog_writes_the_line_the_handler_recorded_not_the_text_the_client_gets(t *testing.T) {
	const modelText = "query failed: Conversion Error: Could not convert string 'Chequing' to INT32"

	got := logged(t.Context(), refusalResult(modelText), "query failed: Conversion Error; details went to the client only")

	assert.Equal(t, "quarry: mcp: query: query failed: Conversion Error; details went to the client only\n", got)
}

func Test_errorLog_logs_an_argument_refusal_for_a_refusal_with_no_recorded_line(t *testing.T) {
	got := logged(t.Context(), refusalResult("validating arguments: sql is 'SELECT 1; DROP'"), "")

	assert.Equal(t, "quarry: mcp: query: refused the call's arguments; details went to the client only\n", got)
}

func Test_errorLog_writes_nothing_for_a_call_whose_context_is_done(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	assert.Empty(t, logged(ctx, refusalResult("context canceled"), "recorded"))
}

func Test_errorLog_writes_nothing_for_a_call_the_server_answered_with_a_protocol_error(t *testing.T) {
	var nilResult *sdk.CallToolResult

	assert.Empty(t, logged(t.Context(), nilResult, ""))
}

func Test_errorLog_writes_nothing_for_a_call_that_succeeded(t *testing.T) {
	assert.Empty(t, logged(t.Context(), &sdk.CallToolResult{}, ""))
}

// fakeQueryStore records the maxRows Query is asked for.
type fakeQueryStore struct {
	report.Store

	asked []int
}

func (f *fakeQueryStore) Query(_ context.Context, _ string, maxRows int) (store.QueryResult, error) {
	f.asked = append(f.asked, maxRows)
	return store.QueryResult{}, nil
}

func Test_query_caps_a_limit_the_schema_did_not_check(t *testing.T) {
	cases := map[string]int{"zero": 0, "negative": -3, "above the cap": maxRows + 1}

	for name, limit := range cases {
		t.Run(name, func(t *testing.T) {
			st := &fakeQueryStore{}
			srv := NewServer(WithReport(func(context.Context, string) (*report.Server, error) {
				return report.NewServer(report.WithStore(st)), nil
			}))

			_, err := srv.query(t.Context(), queryInput{SQL: "SELECT 1", Limit: limit})

			require.NoError(t, err)
			assert.Equal(t, []int{maxRows + 1}, st.asked)
		})
	}
}

// overlapWriter counts writes that begin while another is still in progress.
type overlapWriter struct {
	active   atomic.Int32
	overlaps atomic.Int32
}

func (w *overlapWriter) Write(p []byte) (int, error) {
	if w.active.Add(1) > 1 {
		w.overlaps.Add(1)
	}
	for range 10 {
		runtime.Gosched()
	}
	w.active.Add(-1)
	return len(p), nil
}

func Test_errorLog_never_writes_two_lines_at_once(t *testing.T) {
	const calls = 64
	var w overlapWriter
	log := errorLog(&w)(func(ctx context.Context, _ string, _ sdk.Request) (sdk.Result, error) {
		recordLog(ctx, "boom")
		return refusalResult("boom"), nil
	})
	call := &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: "query"}}
	var wg sync.WaitGroup

	for range calls {
		wg.Go(func() { _, _ = log(t.Context(), "tools/call", call) })
	}
	wg.Wait()

	assert.Zero(t, w.overlaps.Load())
}

const (
	refusalHome  = "/Users/dave"
	refusalStore = refusalHome + "/Library/Application Support/quarry/quarry.duckdb"
	atRefusal    = "~/Library/Application Support/quarry/quarry.duckdb"
)

var errTestFault = errors.New("disk on fire")

// refusingStore fails Status with openErr and lists accounts.
type refusingStore struct {
	report.Store

	openErr  error
	accounts store.AccountList
}

func (s refusingStore) Status(context.Context) (store.Status, error) {
	return store.Status{}, s.openErr
}

func (s refusingStore) Accounts(context.Context) (store.AccountList, error) { return s.accounts, nil }

func (s refusingStore) Search(context.Context, store.SearchParams) (store.Search, error) {
	return store.Search{UnknownCategory: true}, nil
}

func newRefusingServer(openErr error, names ...string) *report.Server {
	list := store.AccountList{}
	for i, name := range names {
		list.Accounts = append(list.Accounts, store.AccountBalance{ID: "id-" + string(rune('a'+i)), Name: name})
	}
	return report.NewServer(report.WithStore(refusingStore{openErr: openErr, accounts: list}), report.WithHome(refusalHome))
}

func storeRefusalFor(t *testing.T, openErr *store.OpenError) error {
	t.Helper()
	_, err := newRefusingServer(openErr).Status(t.Context())
	require.Error(t, err)
	return err
}

func categoryRefusalFor(t *testing.T, arg string) error {
	t.Helper()
	_, err := newRefusingServer(nil).Search(t.Context(), report.SearchRequest{Category: &arg})
	require.Error(t, err)
	return err
}

func amountRefusalFor(t *testing.T, least, most *string) error {
	t.Helper()
	_, err := report.ParseSearchAmounts(least, most)
	require.Error(t, err)
	return err
}

func accountRefusalFor(t *testing.T, arg string, names ...string) error {
	t.Helper()
	_, err := newRefusingServer(nil, names...).Spend(t.Context(), report.SpendRequest{Accounts: []string{arg}})
	require.Error(t, err)
	return err
}

// monthRefusalFor is the refusal report.ParseMonth returns for value on 2026-10-06.
func monthRefusalFor(t *testing.T, value string) error {
	t.Helper()
	_, err := report.ParseMonth(&value, time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC))
	require.Error(t, err)
	return err
}

// unknownSecurityRefusal is acb's refusal of a security that names none, as report.ACB returns it.
func unknownSecurityRefusal(arg string) error {
	return report.RefusalError{Kind: report.RefusalUnknownSecurity, Arg: arg}
}

func Test_logLine_classifies_each_refusal(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "an account that names none", err: accountRefusalFor(t, "Nope", "Chequing"), want: unknownAccountLog},
		{name: "an account that names several", err: accountRefusalFor(t, "Visa", "Visa", "Visa"), want: ambiguousAccountLog},
		{name: "a category that names none", err: categoryRefusalFor(t, "Fod"), want: unknownCategoryLog},
		{name: "the same, worded for the model", err: categoryRefusal(categoryRefusalFor(t, "Fod")), want: unknownCategoryLog},
		{name: "a security that names none", err: unknownSecurityRefusal("XYZ"), want: unknownSecurityLog},
		{name: "the same, worded for the model", err: acbRefusal(unknownSecurityRefusal("XYZ")), want: unknownSecurityLog},
		{name: "a year after this one", err: acbYearRefusal(report.ACBYearError{Kind: report.ACBYearAfterThisYear, Value: "2027"}), want: yearRefusedLog},
		{name: "a month that is not a month", err: monthRefusal(monthRefusalFor(t, "2026-9")), want: monthRefusedLog},
		{name: "a month that has not ended", err: monthRefusal(monthRefusalFor(t, "2026-10")), want: monthRefusedLog},
		{name: "blank search text", err: textRefusal(report.CheckSearchText(new(" "))), want: textRefusedLog},
		{name: "a min that is not an amount", err: amountRefusal(amountRefusalFor(t, new("-12"), nil)), want: amountRefusedLog},
		{name: "a min above the max", err: amountRefusal(amountRefusalFor(t, new("50"), new("20"))), want: amountRefusedLog},
		{
			name: "a store that fails when opened, in the engine's words",
			err:  storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultOther, Path: refusalStore, Reason: `Could not read from file "` + refusalStore + `": Is a directory`}),
			want: "cannot read the store at " + atRefusal + "; details went to the client only",
		},
		{
			name: "a store that fails while read, in quarry's words",
			err:  storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultOther, Path: refusalStore, Reason: "the store has no import history"}),
			want: "cannot read the store at " + atRefusal + "; details went to the client only",
		},
		{
			name: "a store outside home",
			err:  storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultOther, Path: "/srv/quarry/quarry.duckdb", Reason: "x"}),
			want: "cannot read the store at /srv/quarry/quarry.duckdb; details went to the client only",
		},
		{
			name: "no store yet", err: storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultMissing, Path: refusalStore}),
			want: "no store at " + atRefusal + " yet; run quarry sync to build it",
		},
		{
			name: "a store from another version", err: storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultOtherFormat, Path: refusalStore}),
			want: "the store at " + atRefusal + " was built by another version of quarry; run quarry sync to rebuild it",
		},
		{
			name: "a store that is not DuckDB", err: storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: refusalStore}),
			want: "cannot read the store at " + atRefusal + ": the file is not a DuckDB database; run quarry sync to rebuild it",
		},
		{
			name: "a store it may not read", err: storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultPermission, Path: refusalStore}),
			want: "cannot read the store at " + atRefusal + ": permission denied; run quarry sync to rebuild it",
		},
		{
			name: "a store another program holds", err: storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultLocked, Path: refusalStore}),
			want: "cannot read the store at " + atRefusal + ": another program has it open for writing; close that program and run the command again",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, logLine(c.err))
		})
	}
}

func Test_logLine_repeats_the_unclassified_accounts_text_the_client_gets(t *testing.T) {
	err := acbRefusal(report.RefusalError{Kind: report.RefusalUnclassifiedAccounts, Count: 2})

	assert.Equal(t, err.Error(), logLine(err))
	assert.Contains(t, err.Error(), "2 accounts are in neither")
}

func Test_logLine_never_carries_the_callers_values(t *testing.T) {
	const distinctive = "Zorblax"
	cases := []struct {
		name string
		err  error
	}{
		{name: "an account that names none", err: accountRefusalFor(t, distinctive, "Chequing")},
		{name: "the same, worded for the model", err: accountRefusal(accountRefusalFor(t, distinctive, "Chequing"))},
		{name: "an account that names several", err: accountRefusalFor(t, distinctive, distinctive, distinctive)},
		{name: "a category that names none, worded for the model", err: categoryRefusal(categoryRefusalFor(t, distinctive))},
		{name: "a min that is not an amount", err: amountRefusal(amountRefusalFor(t, new(distinctive), nil))},
		{name: "a security that names none, worded for the model", err: acbRefusal(unknownSecurityRefusal(distinctive))},
		{name: "a year after this one", err: acbYearRefusal(report.ACBYearError{Kind: report.ACBYearAfterThisYear, Value: distinctive})},
		{name: "a month that is not a month", err: monthRefusal(monthRefusalFor(t, distinctive))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := logLine(c.err)

			assert.NotContains(t, got, distinctive)
			assert.NotContains(t, got, "id-")
		})
	}
}

func Test_logLine_never_carries_the_month_a_call_asked_for_that_has_not_ended(t *testing.T) {
	const distinctiveMonth = "2031-07"

	got := logLine(monthRefusal(monthRefusalFor(t, distinctiveMonth)))

	assert.NotContains(t, got, distinctiveMonth)
}

func Test_logLine_never_carries_a_store_reason_in_the_engines_words(t *testing.T) {
	const engineWords = "Zorblax catalog failure"
	err := storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultOther, Path: refusalStore, Reason: engineWords})

	assert.NotContains(t, logLine(err), engineWords)
}

func Test_logLine_keeps_a_refusal_of_no_kind_verbatim(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultOther, Path: refusalStore, Reason: "x"}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := newRefusingServer(openErr).Status(ctx)
	require.Error(t, err)

	assert.Equal(t, "status interrupted", logLine(err))
}

func Test_accountRefusal_words_unknown_and_leaves_the_rest(t *testing.T) {
	const listing = "; call describe_schema to list the accounts"
	cases := []struct {
		name string
		arg  string
		want string
	}{
		{name: "an ordinary name", arg: "Nope", want: `no account named "Nope"` + listing},
		{name: "an empty name shows as empty quotes", arg: "", want: `no account named ""` + listing},
		{name: "a quote and a verb stay as typed", arg: `Vi"sa %d`, want: `no account named "Vi\"sa %d"` + listing},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := accountRefusal(accountRefusalFor(t, c.arg, "Chequing"))

			require.EqualError(t, got, c.want)
			assert.Equal(t, unknownAccountLog, logLine(got))
			refusal, ok := errors.AsType[report.RefusalError](got)
			require.True(t, ok)
			assert.Equal(t, report.RefusalUnknownAccount, refusal.Kind)
		})
	}

	passthrough := []struct {
		name string
		err  error
	}{
		{name: "an ambiguous name", err: accountRefusalFor(t, "Visa", "Visa", "Visa")},
		{name: "a store refusal", err: storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultMissing, Path: refusalStore})},
		{name: "an error that is no refusal", err: errTestFault},
	}

	for _, c := range passthrough {
		t.Run(c.name, func(t *testing.T) {
			got := accountRefusal(c.err)

			assert.Equal(t, c.err, got)
		})
	}
}

func Test_amountRefusal_words_each_amount_error_from_its_parts(t *testing.T) {
	const hint = `is not an amount; use digits with up to 2 decimals and no sign, such as "25" or "19.99"`
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "a min with a sign", err: amountRefusalFor(t, new("-12"), nil), want: `min "-12" ` + hint},
		{name: "a max with a sign", err: amountRefusalFor(t, nil, new("-3")), want: `max "-3" ` + hint},
		{name: "a quote stays escaped", err: amountRefusalFor(t, new(`1"2`), nil), want: `min "1\"2" ` + hint},
		{name: "min above max shows the raw values", err: amountRefusalFor(t, new("50.00"), new("20")), want: "min 50.00 is more than max 20"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := amountRefusal(c.err)

			require.EqualError(t, got, c.want)
			assert.Equal(t, amountRefusedLog, logLine(got))
		})
	}

	t.Run("an error that is no amount error comes back as is", func(t *testing.T) {
		got := amountRefusal(errTestFault)

		assert.Equal(t, errTestFault, got)
	})
}

func Test_textRefusal_words_blank_text_and_leaves_the_rest(t *testing.T) {
	got := textRefusal(report.ErrBlankSearchText)

	require.EqualError(t, got, "text is blank; leave it out to search by date, account, category or amount alone")
	assert.Equal(t, textRefusedLog, logLine(got))
	assert.Equal(t, errTestFault, textRefusal(errTestFault))
}

func Test_categoryRefusal_words_unknown_and_leaves_the_rest(t *testing.T) {
	const listing = "; call describe_schema to list the categories"
	cases := []struct {
		name string
		arg  string
		want string
	}{
		{name: "an ordinary name", arg: "Fod", want: `no category named "Fod"` + listing},
		{name: "an empty name shows as empty quotes", arg: "", want: `no category named ""` + listing},
		{name: "a quote and a verb stay as typed", arg: `Fo"d %d`, want: `no category named "Fo\"d %d"` + listing},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := categoryRefusal(categoryRefusalFor(t, c.arg))

			require.EqualError(t, got, c.want)
			assert.Equal(t, unknownCategoryLog, logLine(got))
			refusal, ok := errors.AsType[report.RefusalError](got)
			require.True(t, ok)
			assert.Equal(t, report.RefusalUnknownCategory, refusal.Kind)
		})
	}

	passthrough := []struct {
		name string
		err  error
	}{
		{name: "an unknown account", err: accountRefusalFor(t, "Nope", "Chequing")},
		{name: "an ambiguous account", err: accountRefusalFor(t, "Visa", "Visa", "Visa")},
		{name: "a store refusal", err: storeRefusalFor(t, &store.OpenError{Fault: store.OpenFaultMissing, Path: refusalStore})},
		{name: "an error that is no refusal", err: errTestFault},
	}

	for _, c := range passthrough {
		t.Run(c.name, func(t *testing.T) {
			got := categoryRefusal(c.err)

			assert.Equal(t, c.err, got)
		})
	}
}
