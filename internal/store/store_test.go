package store_test

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_IsInvestmentAccount(t *testing.T) {
	cases := []struct {
		name        string
		accountType string
		want        bool
	}{
		{name: "brokerage is an investment account", accountType: "brokerage", want: true},
		{name: "retirement is an investment account", accountType: "retirement", want: true},
		{name: "chequing is not", accountType: "chequing", want: false},
		{name: "an empty type is not", accountType: "", want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, store.IsInvestmentAccount(c.accountType))
		})
	}
}
