// White-box: parseTypeName's refusals and the renderer's fallbacks are reached
// only through type names and Go values the driver does not produce.
package duckdb

import (
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/assert"
)

func Test_parse_type_name_refuses_a_malformed_name(t *testing.T) {
	cases := []struct {
		name     string
		typeName string
	}{
		{name: "empty", typeName: ""},
		{name: "STRUCT without members", typeName: "STRUCT"},
		{name: "STRUCT member name unquoted", typeName: "STRUCT(a INTEGER)"},
		{name: "STRUCT member name unterminated", typeName: `STRUCT("a`},
		{name: "STRUCT member name without a space", typeName: `STRUCT("a"INTEGER)`},
		{name: "STRUCT member without a type", typeName: `STRUCT("a" )`},
		{name: "STRUCT members without a separator", typeName: `STRUCT("a" INTEGER;"b" INTEGER)`},
		{name: "MAP without types", typeName: "MAP"},
		{name: "MAP with one type", typeName: "MAP(INTEGER)"},
		{name: "MAP without a value type", typeName: "MAP(INTEGER, )"},
		{name: "MAP unclosed", typeName: "MAP(INTEGER, INTEGER"},
		{name: "DECIMAL without width and scale", typeName: "DECIMAL"},
		{name: "ARRAY unclosed", typeName: "INTEGER[2"},
		{name: "trailing text", typeName: "INTEGER extra"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := parseTypeName(c.typeName)

			assert.False(t, ok)
		})
	}
}

func Test_struct_fields_sort_by_name_when_the_type_does_not_name_them(t *testing.T) {
	var inner duckdbdriver.OrderedMap
	inner.Set("k", int32(1))
	value := map[string]any{"z": []any{int32(2)}, "a": inner}
	cases := []struct {
		name     string
		typeName string
	}{
		{name: "type name does not parse", typeName: "STRUCT(unparsed)"},
		{name: "type names a different field", typeName: `STRUCT("z" INTEGER[], "b" MAP(VARCHAR, INTEGER))`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			typ, _ := parseTypeName(c.typeName)

			v, ok := cellValue(typ, value)

			assert.True(t, ok)
			assert.Equal(t, "{'a': {k=1}, 'z': [2]}", v.Text)
		})
	}
}

func Test_cell_value_refuses_a_value_it_cannot_print(t *testing.T) {
	var badKey, badValue duckdbdriver.OrderedMap
	badKey.Set(struct{}{}, int32(1))
	badValue.Set("k", struct{}{})
	cases := []struct {
		name     string
		typeName string
		value    any
	}{
		{name: "unknown Go type", typeName: "INTEGER", value: struct{}{}},
		{name: "bytes of an unknown type", typeName: "GEOMETRY", value: []byte{1}},
		{name: "UUID of the wrong length", typeName: "UUID", value: []byte{1}},
		{name: "time of an unknown type", typeName: "UNKNOWN", value: time.Time{}},
		{name: "decoded JSON", typeName: "JSON", value: float64(1)},
		{name: "LIST element", typeName: "INTEGER[]", value: []any{struct{}{}}},
		{name: "MAP key", typeName: "MAP(INTEGER, INTEGER)", value: badKey},
		{name: "MAP value", typeName: "MAP(VARCHAR, INTEGER)", value: badValue},
		{name: "STRUCT field", typeName: `STRUCT("a" INTEGER)`, value: map[string]any{"a": struct{}{}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			typ, _ := parseTypeName(c.typeName)

			_, ok := cellValue(typ, c.value)

			assert.False(t, ok)
		})
	}
}
