package document_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewFindingEntry_encodes_shares_without_cost_with_its_date_account_and_investment_keys(t *testing.T) {
	listed := report.ListedFinding{
		Status: finding.StatusOpen,
		ID:     "shares-without-cost:itxn-7", Type: finding.SharesWithoutCost,
		Items: []store.FindingItem{{
			Date: time.Date(2016, time.March, 1, 0, 0, 0, 0, time.UTC), AccountID: "acct-3", Account: "Questrade Margin", Currency: "CAD",
			InvestmentTransactionID: new("itxn-7"), SecurityID: new("sec-4"), Security: "XEQT", Shares: 100_000_000,
		}},
	}

	data, err := json.Marshal(document.NewFindingEntry(listed))

	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id": "shares-without-cost:itxn-7", "type": "shares-without-cost", "status": "open",
		"first_found_at": null, "fixed_at": null, "fix": `+string(mustJSON(t, finding.SharesWithoutCost.Fix().Sentence))+`,
		"items": [{
			"transaction_id": null, "split_id": null, "payee_id": null, "category_id": null, "date": "2016-03-01",
			"account_id": "acct-3", "account": "Questrade Margin", "currency": "CAD", "payee": null, "category": null,
			"amount": null, "other_account": null, "other_account_id": null, "transactions": null, "splits": null,
			"investment_transaction_id": "itxn-7", "security_id": "sec-4", "security": "XEQT", "shares": "100.000000"
		}]
	}`, string(data))
}
