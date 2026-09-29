// White-box: pairTransfers builds rows and counts together, so no snapshot
// can present checkTransferTotals a disagreeing pair.
package importer

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_checkTransferTotals_accepts_rows_that_are_paired_plus_one_sided(t *testing.T) {
	t.Parallel()
	rows := []store.Transfer{{ID: "xfer-1"}, {ID: "xfer-2"}, {ID: "xfer-3"}}
	check := store.TransferCheck{Paired: 2, OneSided: []store.OneSidedTransfer{{ID: "xfer-3"}}}

	err := checkTransferTotals(rows, check)

	require.NoError(t, err)
}

func Test_checkTransferTotals_refuses_rows_that_are_not_paired_plus_one_sided(t *testing.T) {
	t.Parallel()
	oneSided := []store.OneSidedTransfer{{ID: "xfer-3"}}
	cases := []struct {
		name  string
		rows  []store.Transfer
		check store.TransferCheck
	}{
		{
			name:  "a row neither paired nor one-sided",
			rows:  []store.Transfer{{ID: "xfer-1"}, {ID: "xfer-2"}, {ID: "xfer-3"}, {ID: "xfer-4"}},
			check: store.TransferCheck{Paired: 2, OneSided: oneSided},
		},
		{
			name:  "a count with no row behind it",
			rows:  []store.Transfer{{ID: "xfer-1"}, {ID: "xfer-2"}},
			check: store.TransferCheck{Paired: 2, OneSided: oneSided},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			err := checkTransferTotals(c.rows, c.check)

			assert.ErrorIs(t, err, errTransferTotals)
		})
	}
}
