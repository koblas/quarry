package main

import (
	"bytes"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingTransport answers every request with an empty Valet answer and keeps the request paths.
type recordingTransport struct{ paths []string }

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.paths = append(r.paths, req.URL.Path)
	return valetResponse(req, http.StatusOK, `{"observations":[]}`), nil
}

// Swaps http.DefaultTransport, which the shipped wiring's Valet client uses: no t.Parallel.
func Test_the_shipped_sync_fetches_exchange_rates_from_the_valet_series(t *testing.T) {
	home := newHome(t)
	transport := &recordingTransport{}
	saved := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = saved })
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-5.00"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := runProcess(t.Context(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, transport.paths, "/valet/observations/FXUSDCAD/json")
}
