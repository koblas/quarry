package report_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

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
