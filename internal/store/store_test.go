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

func Test_query_column_numeric(t *testing.T) {
	cases := []struct {
		name     string
		typeName string
		want     bool
	}{
		{name: "TINYINT", typeName: "TINYINT", want: true},
		{name: "SMALLINT", typeName: "SMALLINT", want: true},
		{name: "INTEGER", typeName: "INTEGER", want: true},
		{name: "BIGINT", typeName: "BIGINT", want: true},
		{name: "HUGEINT", typeName: "HUGEINT", want: true},
		{name: "BIGNUM", typeName: "BIGNUM", want: true},
		{name: "UTINYINT", typeName: "UTINYINT", want: true},
		{name: "USMALLINT", typeName: "USMALLINT", want: true},
		{name: "UINTEGER", typeName: "UINTEGER", want: true},
		{name: "UBIGINT", typeName: "UBIGINT", want: true},
		{name: "UHUGEINT", typeName: "UHUGEINT", want: true},
		{name: "FLOAT", typeName: "FLOAT", want: true},
		{name: "DOUBLE", typeName: "DOUBLE", want: true},
		{name: "DECIMAL", typeName: "DECIMAL(18,2)", want: true},
		{name: "a LIST of integers is not", typeName: "INTEGER[]", want: false},
		{name: "an ARRAY of integers is not", typeName: "INTEGER[2]", want: false},
		{name: "a LIST of DECIMALs is not", typeName: "DECIMAL(4,1)[]", want: false},
		{name: "VARCHAR is not", typeName: "VARCHAR", want: false},
		{name: "a STRUCT of an integer is not", typeName: `STRUCT("a" INTEGER)`, want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, store.QueryColumn{Type: c.typeName}.Numeric())
		})
	}
}

func Test_unprintable_value_error_names_the_column_and_its_type(t *testing.T) {
	err := &store.UnprintableValueError{Column: "doc", Type: "JSON"}

	assert.EqualError(t, err, `cannot print column "doc" of type JSON`)
}

func Test_query_error_reads_as_its_reason(t *testing.T) {
	err := &store.QueryError{Reason: "Binder Error: Referenced column \"x\" not found in FROM clause!"}

	assert.EqualError(t, err, "Binder Error: Referenced column \"x\" not found in FROM clause!")
}
