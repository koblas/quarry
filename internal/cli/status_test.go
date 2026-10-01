package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusStore answers Status from one build and Findings from another, as a sync between two reads would.
type statusStore struct {
	report.Store

	status   store.Status
	findings store.FindingList
}

func (f statusStore) Status(context.Context) (store.Status, error) { return f.status, nil }

func (f statusStore) Findings(context.Context) (store.FindingList, error) { return f.findings, nil }

func Test_status_counts_the_findings_from_the_same_read_as_the_rest_of_the_status(t *testing.T) {
	fake := statusStore{
		status:   store.Status{Findings: []store.Finding{{ID: "duplicate:txn-1+txn-2", Type: finding.Duplicate}}},
		findings: store.FindingList{Findings: []store.Finding{{ID: "duplicate:txn-3+txn-4", Type: finding.Duplicate}, {ID: "duplicate:txn-5+txn-6", Type: finding.Duplicate}}},
	}
	var stdout bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &bytes.Buffer{},
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
		LoadConfig: func(string) (config.Config, error) { return config.Config{}, nil },
	}

	err := cli.Execute(t.Context(), []string{"status"}, env)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Findings  1 open; run quarry findings to list them\n")
}
