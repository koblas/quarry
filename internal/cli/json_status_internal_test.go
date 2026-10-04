// White-box: renderStatusJSON's null forms and time handling are unexported
// formatting rules best driven directly over a store.Status.
package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fullStatusJSON = `{
  "store": {
    "path": "/Users/dave/Library/Application Support/quarry/quarry.duckdb",
    "format_version": 2,
    "quarry_version": "v0.4.0",
    "built_at": "2026-09-27T14:31:02Z",
    "rows": {
      "accounts": 7,
      "categories": 312,
      "payees": 1873,
      "tags": 14,
      "transactions": 18204,
      "splits": 21977,
      "split_tags": 5,
      "transfers": 3141,
      "investment_transactions": 1605,
      "securities": 84,
      "prices": 99352
    }
  },
  "snapshot": {
    "id": "20260927T143005Z",
    "path": "/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite",
    "taken_at": "2026-09-27T14:30:05Z",
    "source": "/Users/dave/Documents/Home.quicken",
    "sha256": "abc123"
  },
  "dates": {
    "first": "2003-01-04",
    "last": "2026-09-26"
  },
  "balances": {
    "checked": 35,
    "never_reconciled": 3,
    "investment_accounts": 4
  },
  "splits": {
    "checked": 18204
  },
  "transfers": {
    "paired": 3112,
    "cross_currency": 41,
    "one_sided": 29
  },
  "findings": {
    "open": 12,
    "ignored": 4,
    "fixed": 7,
    "new": 2,
    "newly_fixed": 1
  },
  "rates": {
    "first": "2003-01-04",
    "last": "2026-09-28",
    "fetch_error": null
  },
  "not_imported": {
    "investment_transactions": 1605
  },
  "warnings": []
}
`

func jsonFindingsFixture() document.FindingsTally {
	return document.FindingsTally{Counts: finding.Counts{Open: 12, Ignored: 4, Fixed: 7, New: 2, NewlyFixed: 1}, IgnoreKnown: true}
}

func Test_renderStatusJSON(t *testing.T) {
	newStatus := func() store.Status {
		st := statusFixture()
		st.FormatVersion = 2
		st.QuarryVersion = "v0.4.0"
		st.BuiltAt = time.Date(2026, 9, 27, 14, 31, 2, 0, time.UTC)
		st.Run.Snapshot.SHA256 = "abc123"
		st.Run.Counts.Accounts, st.Run.Counts.SplitTags = 7, 5
		st.Run.TransfersCrossCurrency = 41
		return st
	}
	renderWith := func(t *testing.T, st store.Status, findings document.FindingsTally, warnings []string) string {
		t.Helper()
		out, err := renderStatusJSON(st, findings, warnings)
		require.NoError(t, err)
		return string(out)
	}
	render := func(t *testing.T, st store.Status) string {
		t.Helper()
		return renderWith(t, st, jsonFindingsFixture(), nil)
	}

	t.Run("full document", func(t *testing.T) {
		assert.Equal(t, fullStatusJSON, render(t, newStatus())) //nolint:testifylint // the bytes are the contract: key order and indent
	})

	t.Run("no transactions", func(t *testing.T) {
		st := newStatus()
		st.FirstDate, st.LastDate = time.Time{}, time.Time{}

		got := render(t, st)

		assert.Contains(t, got, "\"first\": null,\n    \"last\": null\n")
	})

	t.Run("snapshot time not recorded", func(t *testing.T) {
		st := newStatus()
		st.Run.Snapshot.TakenAt = time.Time{}

		assert.Contains(t, render(t, st), "\"taken_at\": null,\n")
	})

	t.Run("source not recorded", func(t *testing.T) {
		st := newStatus()
		st.Run.Snapshot.Source = ""

		assert.Contains(t, render(t, st), "\"source\": null,\n")
	})

	t.Run("whitespace-only source stays a string", func(t *testing.T) {
		st := newStatus()
		st.Run.Snapshot.Source = " "

		assert.Contains(t, render(t, st), "\"source\": \" \",\n")
	})

	t.Run("built_at in another zone is written in UTC", func(t *testing.T) {
		st := newStatus()
		st.BuiltAt = time.Date(2026, 9, 27, 10, 31, 2, 0, time.FixedZone("EDT", -4*60*60))

		assert.Contains(t, render(t, st), "\"built_at\": \"2026-09-27T14:31:02Z\",\n")
	})

	t.Run("times and dates ignore the local zone", func(t *testing.T) {
		useZone(t, time.FixedZone("PST", -8*60*60))

		got := render(t, newStatus())

		assert.Contains(t, got, "\"taken_at\": \"2026-09-27T14:30:05Z\",\n")
		assert.Contains(t, got, "\"first\": \"2003-01-04\",\n")
	})

	t.Run("an unreadable ignore list makes ignored null and keeps the other counts", func(t *testing.T) {
		findings := jsonFindingsFixture()
		findings.IgnoreKnown = false

		got := renderWith(t, newStatus(), findings, nil)

		assert.Contains(t, got, "\"findings\": {\n    \"open\": 12,\n    \"ignored\": null,\n    \"fixed\": 7,\n    \"new\": 2,\n    \"newly_fixed\": 1\n  },\n")
	})

	t.Run("warnings are listed as given", func(t *testing.T) {
		got := renderWith(t, newStatus(), jsonFindingsFixture(), []string{"cannot tell which findings you ignored: x"})

		assert.True(t, strings.HasSuffix(got, "\"warnings\": [\n    \"cannot tell which findings you ignored: x\"\n  ]\n}\n"))
	})
}
