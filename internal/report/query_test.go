package report_test

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func queryRows(n int) [][]store.QueryValue {
	rows := make([][]store.QueryValue, n)
	for i := range rows {
		rows[i] = []store.QueryValue{{Text: strconv.Itoa(i + 1), Native: int64(i + 1)}}
	}
	return rows
}

func Test_query_fetches_one_row_more_than_the_limit(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "a positive limit fetches one more row", limit: 3, want: 4},
		{name: "limit 0 fetches every row", limit: 0, want: 0},
		{name: "a negative limit fetches every row", limit: -1, want: 0},
		{name: "the largest limit fetches every row without overflowing", limit: math.MaxInt, want: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotMaxRows int
			srv := report.NewServer(report.WithStore(fakeStore{gotMaxRows: &gotMaxRows}))

			_, err := srv.Query(t.Context(), "SELECT 1", c.limit)

			require.NoError(t, err)
			assert.Equal(t, c.want, gotMaxRows)
		})
	}
}

func Test_query_truncates_only_past_the_limit(t *testing.T) {
	cases := []struct {
		name      string
		available int
		limit     int
		want      report.QueryResult
	}{
		{
			name: "exactly limit rows are not truncated", available: 3, limit: 3,
			want: report.QueryResult{QueryResult: store.QueryResult{Rows: queryRows(3)}},
		},
		{
			name: "one row past the limit is truncated to the limit", available: 4, limit: 3,
			want: report.QueryResult{QueryResult: store.QueryResult{Rows: queryRows(3)}, Truncated: true},
		},
		{
			name: "limit 0 keeps every row", available: 4, limit: 0,
			want: report.QueryResult{QueryResult: store.QueryResult{Rows: queryRows(4)}},
		},
		{
			name: "a negative limit keeps every row", available: 4, limit: -1,
			want: report.QueryResult{QueryResult: store.QueryResult{Rows: queryRows(4)}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotMaxRows int
			srv := report.NewServer(report.WithStore(fakeStore{rows: queryRows(c.available), gotMaxRows: &gotMaxRows}))

			got, err := srv.Query(t.Context(), "SELECT 1", c.limit)

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func Test_query_returns_the_store_fault(t *testing.T) {
	var gotMaxRows int
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead, gotMaxRows: &gotMaxRows}))

	_, err := srv.Query(t.Context(), "SELECT 1", 500)

	require.ErrorIs(t, err, errDiskRead)
}

func Test_ClassifyQueryFailure_names_the_reason_a_query_failed(t *testing.T) {
	unprintable := &store.UnprintableValueError{Column: "payload", Type: "UNION"}
	rejected := &store.QueryError{Reason: `Catalog Error: Table with name nope does not exist!`}
	cases := []struct {
		name string
		err  error
		want report.QueryFailure
	}{
		{
			name: "an unprintable column, wrapped", err: fmt.Errorf("run: %w", unprintable),
			want: report.QueryFailure{Kind: report.QueryFailureUnprintable, Detail: "payload", Err: unprintable},
		},
		{
			name: "a query the database rejected, wrapped", err: fmt.Errorf("run: %w", rejected),
			want: report.QueryFailure{Kind: report.QueryFailureRejected, Detail: rejected.Reason, Err: rejected},
		},
		{
			name: "an empty query", err: fmt.Errorf("run: %w", store.ErrEmptyQuery),
			want: report.QueryFailure{Kind: report.QueryFailureEmpty, Err: store.ErrEmptyQuery},
		},
		{
			name: "a query that would change the store", err: fmt.Errorf("run: %w", store.ErrReadOnlyQuery),
			want: report.QueryFailure{Kind: report.QueryFailureReadOnly, Err: store.ErrReadOnlyQuery},
		},
		{
			name: "a query reaching outside the store", err: fmt.Errorf("run: %w", store.ErrExternalAccess),
			want: report.QueryFailure{Kind: report.QueryFailureExternalAccess, Err: store.ErrExternalAccess},
		},
		{
			name: "an interrupted query", err: store.Interrupted(context.Canceled),
			want: report.QueryFailure{Kind: report.QueryFailureInterrupted, Err: store.ErrQueryInterrupted},
		},
		{
			name: "a typed rejection beats an interrupt that wraps it", err: store.Interrupted(rejected),
			want: report.QueryFailure{Kind: report.QueryFailureRejected, Detail: rejected.Reason, Err: rejected},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.ClassifyQueryFailure(c.err))
		})
	}
}

func Test_ClassifyQueryFailure_leaves_a_refusal_and_an_unknown_error_unclassified(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "a refusal", err: report.RefusalError{}},
		{name: "an unknown error", err: errDiskRead},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, report.QueryFailure{Kind: report.QueryFailureOther}, report.ClassifyQueryFailure(c.err))
		})
	}
}
