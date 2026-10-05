package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_networth_before_the_first_transaction_prints_the_caption_and_the_first_balance_warning(t *testing.T) {
	seedNetWorthStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2026-03-01"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Net worth on 2026-03-01, amounts in CAD\n\nType  Currency  Balance  In CAD\n", stdout.String())
	assert.Equal(t, "quarry: warning: no account has a balance on 2026-03-01; the first balance is on 2026-03-02\n", stderr.String())
}
