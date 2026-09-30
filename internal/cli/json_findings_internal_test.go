// White-box: renderFindingsJSON's field mapping is unexported; distinct counts and a null
// payee are driven directly over a report.FindingsListing.
package cli

import (
	"encoding/json"
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_renderFindingsJSON_encodes_each_of_the_five_counts_under_its_own_key(t *testing.T) {
	listing := report.FindingsListing{Counts: finding.Counts{Open: 5, Ignored: 4, Fixed: 12, New: 2, NewlyFixed: 1}}

	data, err := renderFindingsJSON(listing, nil)

	require.NoError(t, err)
	var doc struct {
		Counts json.RawMessage `json:"counts"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.JSONEq(t, `{"open": 5, "ignored": 4, "fixed": 12, "new": 2, "newly_fixed": 1}`, string(doc.Counts))
}

func Test_renderFindingsJSON_encodes_a_finding_item_without_a_payee_as_null(t *testing.T) {
	listing := report.FindingsListing{Groups: []report.FindingsGroup{{
		Type: finding.Uncategorized,
		Findings: []store.Finding{{
			ID: "uncategorized:no-payee", Type: finding.Uncategorized,
			Items: []store.FindingItem{{Payee: "", Date: findingDay(2012, 1, 1)}},
		}},
	}}}

	data, err := renderFindingsJSON(listing, nil)

	require.NoError(t, err)
	var doc struct {
		Findings []struct {
			Items []map[string]any `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.Len(t, doc.Findings, 1)
	require.Len(t, doc.Findings[0].Items, 1)
	assert.Contains(t, doc.Findings[0].Items[0], "payee")
	assert.Nil(t, doc.Findings[0].Items[0]["payee"])
}
