package report_test

import (
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
