// The skill eval fixture is read by the recipe tests, so it lives in package main
// beside the other cmd/quarry tests.
package main

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
)

// skillEvalStore builds the store the skill's recipes and use cases are evaluated on under home.
func skillEvalStore(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1)}))
}
