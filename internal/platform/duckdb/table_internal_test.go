// driverRefusal is unexported; the driver never produces an unmatched or
// out-of-range refusal on demand, so those arms are driven directly.
package duckdb

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	errOtherDriverFault   = errors.New("database/sql/driver: API error: invalid input")
	errIndexPastTheResult = errors.New("database/sql/driver: API error: unsupported data type: VARIANT: index: 1")
	errIndexTooLarge      = errors.New("unsupported data type: VARIANT: index: 99999999999999999999")
)

func Test_driver_refusal_returns_an_error_it_cannot_place_unchanged(t *testing.T) {
	columns := []Column{{Name: "label", Type: "VARCHAR"}}
	cases := []struct {
		name string
		err  error
	}{
		{name: "another driver error", err: errOtherDriverFault},
		{name: "a column index past the result", err: errIndexPastTheResult},
		{name: "a column index too large for an int", err: errIndexTooLarge},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Same(t, c.err, driverRefusal(c.err, columns))
		})
	}
}
