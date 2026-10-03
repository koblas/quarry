package document_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_NewAccountFilters_returns_an_empty_list_for_no_accounts(t *testing.T) {
	filter := document.NewAccountFilters(nil)

	assert.NotNil(t, filter)
	assert.Empty(t, filter)
}

func Test_NewAccountFilters_keeps_each_account_id_and_name_in_order(t *testing.T) {
	accounts := []store.Account{{ID: "a-2", Name: "Chequing", Type: "chequing"}, {ID: "a-1", Name: "Visa"}}

	filter := document.NewAccountFilters(accounts)

	assert.Equal(t, []document.AccountFilter{{ID: "a-2", Name: "Chequing"}, {ID: "a-1", Name: "Visa"}}, filter)
}
