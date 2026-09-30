package cli_test

import (
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
)

func Test_ReportedError_reads_already_reported(t *testing.T) {
	assert.EqualError(t, cli.ReportedError{}, "already reported")
}
