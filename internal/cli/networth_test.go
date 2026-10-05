package cli_test

import (
	"bytes"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_networth_returns_a_failed_report_open(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"networth"}, refusedEnv(&stdout, &stderr))

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}
