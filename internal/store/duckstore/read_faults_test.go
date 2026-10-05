package duckstore_test

import (
	"context"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readOp is one store read, called with no arguments beyond the store.
type readOp struct {
	name string
	call func(context.Context, *duckstore.Store) error
}

// rowReads are the reads that scan rows: Status, Accounts, Schema, Charges, Holdings, NetWorth, Findings and Search.
func rowReads() []readOp {
	return []readOp{
		{name: "Status", call: func(ctx context.Context, st *duckstore.Store) error {
			_, err := st.Status(ctx)
			return err
		}},
		{name: "Accounts", call: func(ctx context.Context, st *duckstore.Store) error {
			_, err := st.Accounts(ctx)
			return err
		}},
		{name: "Schema", call: func(ctx context.Context, st *duckstore.Store) error {
			_, err := st.Schema(ctx)
			return err
		}},
		{name: "Charges", call: func(ctx context.Context, st *duckstore.Store) error {
			_, err := st.Charges(ctx, store.ChargeParams{Through: day(2026, 9, 29)})
			return err
		}},
		{name: "Holdings", call: func(ctx context.Context, st *duckstore.Store) error {
			_, err := st.Holdings(ctx, store.HoldingsParams{AsOf: day(2026, 9, 29)})
			return err
		}},
		{name: "NetWorth", call: func(ctx context.Context, st *duckstore.Store) error {
			_, err := st.NetWorth(ctx, store.NetWorthParams{Dates: []time.Time{day(2026, 9, 29)}})
			return err
		}},
		{name: "Findings", call: func(ctx context.Context, st *duckstore.Store) error {
			_, err := st.Findings(ctx)
			return err
		}},
		{name: "Search", call: func(ctx context.Context, st *duckstore.Store) error {
			_, err := st.Search(ctx, store.SearchParams{})
			return err
		}},
	}
}

// allReads are rowReads plus Query, which reads a table instead.
func allReads() []readOp {
	return append(rowReads(), readOp{name: "Query", call: func(ctx context.Context, st *duckstore.Store) error {
		_, err := st.Query(ctx, "SELECT 1", 0)
		return err
	}})
}

func Test_reads_return_the_open_fault(t *testing.T) {
	t.Parallel()
	for _, op := range allReads() {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			fault := ioFault("open store read-only")
			st := newBuiltStore(t, failingOpener(fault))

			err := op.call(t.Context(), st)

			require.ErrorIs(t, err, fault)
			var openErr *store.OpenError
			assert.ErrorAs(t, err, &openErr)
		})
	}
}

func Test_row_reads_return_the_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, op := range rowReads() {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			fault := ioFault(`query rows "SELECT"`)
			st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: fault}))

			err := op.call(t.Context(), st)

			assertOtherFault(t, err, "disk read failed")
			var derr *duckdbdriver.Error
			require.ErrorAs(t, err, &derr)
			assert.ErrorIs(t, err, fault)
		})
	}
}

func Test_row_reads_return_a_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, op := range rowReads() {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(&spyReadDB{scanFault: errScanFailed}))

			err := op.call(t.Context(), st)

			assertOtherFault(t, err, errScanFailed.Error())
			assert.ErrorIs(t, err, errScanFailed)
		})
	}
}

func Test_reads_close_the_connection_on_success_and_on_a_query_fault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fault error
	}{
		{name: "after a successful read", fault: nil},
		{name: "after a query fault", fault: errQueryFailed},
	}
	for _, op := range allReads() {
		for _, c := range cases {
			t.Run(op.name+" "+c.name, func(t *testing.T) {
				t.Parallel()
				spy := &spyReadDB{queryFault: c.fault}
				st := newBuiltStore(t, spyOpener(spy))

				_ = op.call(t.Context(), st)

				assert.Equal(t, 1, spy.closes)
			})
		}
	}
}
