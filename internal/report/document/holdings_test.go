package document_test

import (
	"math/big"
	"testing"

	"github.com/koblas/quarry/internal/report/document"
	"github.com/stretchr/testify/assert"
)

func Test_big_money_renders_cents_of_any_size_ungrouped(t *testing.T) {
	const pastInt64 = "18446744073709551616"
	pastInt64Cents, _ := new(big.Int).SetString(pastInt64, 10)
	cases := []struct {
		name  string
		cents *big.Int
		want  string
	}{
		{name: "zero is 0.00", cents: big.NewInt(0), want: "0.00"},
		{name: "under a unit keeps its leading zero", cents: big.NewInt(5), want: "0.05"},
		{name: "a whole unit", cents: big.NewInt(100), want: "1.00"},
		{name: "a negative under a unit is -0.05, not -1.95", cents: big.NewInt(-5), want: "-0.05"},
		{name: "a negative is not grouped", cents: big.NewInt(-123_456), want: "-1234.56"},
		{name: "a value past int64 keeps every digit", cents: pastInt64Cents, want: "184467440737095516.16"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, document.BigMoney(c.cents))
		})
	}
}

func Test_big_money_does_not_change_the_value_it_renders(t *testing.T) {
	cents := big.NewInt(-123_456)

	_ = document.BigMoney(cents)

	assert.Equal(t, "-123456", cents.String())
}
