package v9_test

import (
	"testing"

	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/stretchr/testify/assert"
)

// fileEntities is the Z_PRIMARYKEY of a file that inserted an entity at Z_ENT 2.
var fileEntities = map[string][]int64{
	"Account":                  {1},
	"AccountGroup":             {2},
	"Budget":                   {10},
	"CashFlowTransactionEntry": {16},
	"DownloadSession":          {31},
	"QuickFillRuleSplitEntry":  {56},
	"UserTag":                  {77},
	"Transaction":              {79},
	"CashFlowTransaction":      {80},
}

func Test_resolve_name_renumbers_each_form_an_entity_number_appears_in(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "join table", in: "Z_15USERTAGS", want: "Z_16USERTAGS"},
		{name: "join column", in: "Z_76USERTAGS", want: "Z_77USERTAGS"},
		{name: "disambiguator stays literal", in: "Z_76USERTAGS1", want: "Z_77USERTAGS1"},
		{name: "entity before the field", in: "Z78_TRANSACTION", want: "Z79_TRANSACTION"},
		{name: "entity before a differently named field", in: "Z78_ORIGINALTRANSACTION", want: "Z79_ORIGINALTRANSACTION"},
		{name: "number that grows a digit", in: "Z_9BUDGETS", want: "Z_10BUDGETS"},
		{name: "number that does not move", in: "Z_1ACCOUNTS1", want: "Z_1ACCOUNTS1"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := v9.ResolveName(c.in, fileEntities)

			assert.Equal(t, c.want, got)
			assert.True(t, ok)
		})
	}
}

func Test_resolve_name_passes_a_name_without_an_entity_number_through(t *testing.T) {
	cases := []string{"Z_PK", "Z_ENT", "Z_OPT", "Z_PRIMARYKEY", "Z_METADATA", "Z_MODELCACHE", "ZACCOUNT", "Z_1_X", "Z78TRANSACTION"}

	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			got, ok := v9.ResolveName(in, fileEntities)

			assert.Equal(t, in, got)
			assert.True(t, ok)
		})
	}
}

func Test_resolve_name_keeps_the_reference_name_when_it_cannot_resolve(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		entities map[string][]int64
	}{
		{name: "entity absent from the file", in: "Z_76USERTAGS", entities: map[string][]int64{"Account": {1}}},
		{name: "entity listed with no number", in: "Z_76USERTAGS", entities: map[string][]int64{"UserTag": {}}},
		{name: "number the reference does not use", in: "Z_5USERTAGS", entities: fileEntities},
		{name: "number beyond int64", in: "Z_99999999999999999999USERTAGS", entities: fileEntities},
		{name: "no entities at all", in: "Z78_TRANSACTION", entities: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := v9.ResolveName(c.in, c.entities)

			assert.Equal(t, c.in, got)
			assert.False(t, ok)
		})
	}
}

func Test_resolve_name_keeps_the_reference_name_when_z_name_is_duplicated(t *testing.T) {
	entities := map[string][]int64{"UserTag": {77, 78}}

	got, ok := v9.ResolveName("Z_76USERTAGS", entities)

	assert.Equal(t, "Z_76USERTAGS", got)
	assert.False(t, ok)
}
