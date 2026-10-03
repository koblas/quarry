// White-box: logLine and accountRefusal are unexported, and the refusals they classify come from a report.Server
// over a store that fails on demand, which a client of the tools cannot arrange per fault.
package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func accountRefusalFor(t *testing.T, arg string, names ...string) error {
	t.Helper()
	_, err := newRefusingServer(nil, names...).Spend(t.Context(), report.SpendRequest{Accounts: []string{arg}})
	require.Error(t, err)
	return err
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

func Test_logLine_never_carries_the_callers_account_text(t *testing.T) {
	const distinctive = "Zorblax"
	cases := []struct {
		name string
		err  error
	}{
		{name: "an account that names none", err: accountRefusalFor(t, distinctive, "Chequing")},
		{name: "the same, worded for the model", err: accountRefusal(accountRefusalFor(t, distinctive, "Chequing"))},
		{name: "an account that names several", err: accountRefusalFor(t, distinctive, distinctive, distinctive)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := logLine(c.err)

			assert.NotContains(t, got, distinctive)
			assert.NotContains(t, got, "id-")
		})
	}
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
