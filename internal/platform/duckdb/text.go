package duckdb

import (
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
)

// nullText is how DuckDB spells a NULL, at the top level and nested.
const nullText = "NULL"

// Infinite DATE and TIMESTAMP values print as these words; the driver hands
// them over as the sentinel instants their storage uses.
const (
	infinityText    = "infinity"
	negInfinityText = "-infinity"
)

// kindJSON is the JSON type: the driver decodes it, losing the text DuckDB would print.
const kindJSON = "JSON"

// nestedSpecial are the characters that make DuckDB quote a nested value.
const nestedSpecial = `"'(),:=[]{}`

// cellValue is v's DuckDB text (what CAST(v AS VARCHAR) gives) plus its typed
// form; false for a value it cannot render.
func cellValue(t *typeNode, v any) (Value, bool) {
	if v == nil {
		return Value{Null: true, Text: nullText}, true
	}
	text, ok := valueText(t, v)
	if !ok {
		return Value{}, false
	}
	return Value{Text: text, Native: nativeValue(t, v, text)}, true
}

// nativeValue is v's typed form: nil, bool, int64, uint64, float32, float64,
// a time.Time for a finite DATE or TIMESTAMP, else text.
func nativeValue(t *typeNode, v any, text string) any {
	switch v := v.(type) {
	case bool, float32, float64:
		return v
	case int8, int16, int32, int64:
		return reflect.ValueOf(v).Int()
	case uint8, uint16, uint32, uint64:
		return reflect.ValueOf(v).Uint()
	case time.Time:
		if text == infinityText || text == negInfinityText {
			return text
		}
		switch t.kindName() {
		case "DATE", "TIMESTAMP", "TIMESTAMP_S", "TIMESTAMP_MS", "TIMESTAMP_NS", "TIMESTAMPTZ":
			return v
		}
	}
	return text
}

// valueText renders v as DuckDB's text for type t, dispatching on v's Go
// type and consulting t where the Go type alone is ambiguous.
func valueText(t *typeNode, v any) (string, bool) {
	if v == nil {
		return nullText, true
	}
	if t.kindName() == kindJSON {
		return "", false
	}
	switch v := v.(type) {
	case bool:
		return strconv.FormatBool(v), true
	case int8, int16, int32, int64:
		return strconv.FormatInt(reflect.ValueOf(v).Int(), 10), true
	case uint8, uint16, uint32, uint64:
		return strconv.FormatUint(reflect.ValueOf(v).Uint(), 10), true
	case float32:
		return floatText(float64(v), 32), true
	case float64:
		return floatText(v, 64), true
	case *big.Int:
		return v.String(), true
	case duckdbdriver.Decimal:
		return decimalText(v), true
	case string:
		return v, true
	case []byte:
		return bytesText(t.kindName(), v)
	case time.Time:
		return timeText(t.kindName(), v)
	case duckdbdriver.Interval:
		return intervalText(v), true
	case []any:
		return listText(t.elemType(), v)
	case duckdbdriver.OrderedMap:
		return mapText(t, v)
	case duckdbdriver.Union:
		return valueText(t.member(v.Tag), v.Value)
	case map[string]any:
		return structText(t, v)
	default:
		return "", false
	}
}

// floatText is DuckDB's float text: shortest round-tripping digits, exponent
// form below 1e-4 and from 1e16; nan, inf, -inf.
func floatText(f float64, bitSize int) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	exponential := strconv.FormatFloat(f, 'e', -1, bitSize)
	exp, _ := strconv.Atoi(exponential[strings.IndexByte(exponential, 'e')+1:])
	if exp < -4 || exp >= 16 {
		return exponential
	}
	fixed := strconv.FormatFloat(f, 'f', -1, bitSize)
	if !strings.Contains(fixed, ".") {
		fixed += ".0"
	}
	return fixed
}

// decimalText renders d with exactly d.Scale decimal places, from its
// unscaled integer: never through a float.
func decimalText(d duckdbdriver.Decimal) string {
	digits := new(big.Int).Abs(d.Value).String()
	sign := ""
	if d.Value.Sign() < 0 {
		sign = "-"
	}
	scale := int(d.Scale)
	if scale == 0 {
		return sign + digits
	}
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale-len(digits)+1) + digits
	}
	return sign + digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
}

// bytesText renders a UUID or BLOB, the two types the driver hands over as bytes.
func bytesText(kind string, b []byte) (string, bool) {
	switch kind {
	case "UUID":
		if len(b) != 16 {
			return "", false
		}
		h := hex.EncodeToString(b)
		return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], true
	case "BLOB":
		var s strings.Builder
		for _, c := range b {
			if c >= ' ' && c <= '~' && c != '"' && c != '\'' && c != '\\' {
				s.WriteByte(c)
			} else {
				fmt.Fprintf(&s, `\x%02X`, c)
			}
		}
		return s.String(), true
	}
	return "", false
}

// timeText renders the date and time types the driver hands over as time.Time.
func timeText(kind string, v time.Time) (string, bool) {
	switch kind {
	case "DATE":
		days := v.Unix() / secondsPerDay
		return infinityOr(days, math.MaxInt32, func() string { return dateText(v) }), true
	case "TIMESTAMP":
		return infinityOr(v.UnixMicro(), math.MaxInt64, func() string { return timestampText(v) }), true
	case "TIMESTAMP_S":
		return infinityOr(v.Unix(), math.MaxInt64, func() string { return timestampText(v) }), true
	case "TIMESTAMP_MS":
		return infinityOr(v.UnixMilli(), math.MaxInt64, func() string { return timestampText(v) }), true
	case "TIMESTAMP_NS":
		return infinityOr(v.UnixNano(), math.MaxInt64, func() string { return timestampText(v) }), true
	case "TIMESTAMPTZ":
		return infinityOr(v.UnixMicro(), math.MaxInt64, func() string {
			// DuckDB renders TIMESTAMPTZ in its TimeZone setting, which defaults to the process's zone.
			local := v.In(time.Local) //nolint:gosmopolitan // DuckDB's default TimeZone is the local zone
			return timestampText(local) + offsetText(local)
		}), true
	case "TIME":
		return clockText(v), true
	case "TIMETZ":
		return clockText(v) + offsetText(v), true
	}
	return "", false
}

const secondsPerDay = 24 * 60 * 60

// infinityOr is the infinity word when units is DuckDB's ±sentinel for
// the type, otherwise finite().
func infinityOr(units, sentinel int64, finite func() string) string {
	switch units {
	case sentinel:
		return infinityText
	case -sentinel:
		return negInfinityText
	}
	return finite()
}

// dateText is YYYY-MM-DD, with at least four year digits and " (BC)" after
// a year before 1.
func dateText(v time.Time) string {
	year, suffix := v.Year(), ""
	if year <= 0 {
		year, suffix = 1-year, " (BC)"
	}
	return fmt.Sprintf("%04d-%02d-%02d%s", year, int(v.Month()), v.Day(), suffix)
}

func timestampText(v time.Time) string {
	return dateText(v) + " " + clockText(v)
}

// clockText is HH:MM:SS plus the fraction of a second, trailing zeros dropped.
func clockText(v time.Time) string {
	return fmt.Sprintf("%02d:%02d:%02d", v.Hour(), v.Minute(), v.Second()) + fractionText(int64(v.Nanosecond()), 9)
}

// fractionText is "." plus n as a digits-wide fraction without trailing
// zeros, or "" when n is 0.
func fractionText(n int64, digits int) string {
	if n == 0 {
		return ""
	}
	return "." + strings.TrimRight(fmt.Sprintf("%0*d", digits, n), "0")
}

// offsetText is v's UTC offset as ±HH, with :MM and :SS only when non-zero.
func offsetText(v time.Time) string {
	_, offset := v.Zone()
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	s := fmt.Sprintf("%s%02d", sign, offset/3600)
	if minutes, seconds := offset/60%60, offset%60; minutes != 0 || seconds != 0 {
		s += fmt.Sprintf(":%02d", minutes)
		if seconds != 0 {
			s += fmt.Sprintf(":%02d", seconds)
		}
	}
	return s
}

// intervalText renders iv as DuckDB does: signed year, month and day parts
// (plural unless ±1), then a signed HH:MM:SS[.ffffff] part; 00:00:00 when all are zero.
func intervalText(iv duckdbdriver.Interval) string {
	var parts []string
	unit := func(n int64, name string) {
		if n == 0 {
			return
		}
		if n != 1 && n != -1 {
			name += "s"
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, name))
	}
	unit(int64(iv.Months/12), "year")
	unit(int64(iv.Months%12), "month")
	unit(int64(iv.Days), "day")

	if iv.Micros != 0 {
		const microsPerSecond = 1_000_000
		sign, seconds, micros := "", iv.Micros/microsPerSecond, iv.Micros%microsPerSecond
		if iv.Micros < 0 {
			sign, seconds, micros = "-", -seconds, -micros
		}
		parts = append(parts, fmt.Sprintf("%s%02d:%02d:%02d", sign, seconds/3600, seconds/60%60, seconds%60)+
			fractionText(micros, 6))
	}
	if len(parts) == 0 {
		return "00:00:00"
	}
	return strings.Join(parts, " ")
}

// listText renders a LIST or ARRAY as [a, b, ...].
func listText(elem *typeNode, items []any) (string, bool) {
	texts := make([]string, len(items))
	for i, item := range items {
		text, ok := nestedText(elem, item)
		if !ok {
			return "", false
		}
		texts[i] = text
	}
	return "[" + strings.Join(texts, ", ") + "]", true
}

// mapText renders a MAP as {k=v, ...} in the map's own order.
func mapText(t *typeNode, m duckdbdriver.OrderedMap) (string, bool) {
	keyType, valueType := t.mapTypes()
	keys, values := m.Keys(), m.Values()
	texts := make([]string, len(keys))
	for i := range keys {
		key, ok := nestedText(keyType, keys[i])
		if !ok {
			return "", false
		}
		value, ok := nestedText(valueType, values[i])
		if !ok {
			return "", false
		}
		texts[i] = key + "=" + value
	}
	return "{" + strings.Join(texts, ", ") + "}", true
}

// structText renders a STRUCT as {'a': 1, ...}, fields in the order t
// declares them, or sorted by name when t does not name exactly m's fields.
func structText(t *typeNode, m map[string]any) (string, bool) {
	names := structFieldOrder(t, m)
	texts := make([]string, len(names))
	for i, name := range names {
		value, ok := nestedText(t.member(name), m[name])
		if !ok {
			return "", false
		}
		texts[i] = "'" + escapeNested(name) + "': " + value
	}
	return "{" + strings.Join(texts, ", ") + "}", true
}

func structFieldOrder(t *typeNode, m map[string]any) []string {
	if t.kindName() == kindStruct && len(t.fields) == len(m) {
		names := make([]string, 0, len(t.fields))
		for _, f := range t.fields {
			if _, ok := m[f.name]; !ok {
				break
			}
			names = append(names, f.name)
		}
		if len(names) == len(m) {
			return names
		}
	}
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// nestedText renders v inside a LIST, STRUCT or MAP, quoting a scalar as
// DuckDB does; NULL, nested and UNION values are never quoted.
func nestedText(t *typeNode, v any) (string, bool) {
	text, ok := valueText(t, v)
	if !ok {
		return "", false
	}
	switch v.(type) {
	case nil, []any, duckdbdriver.OrderedMap, map[string]any, duckdbdriver.Union:
		return text, true
	}
	if needsQuotes(text) {
		return "'" + escapeNested(text) + "'", true
	}
	return text, true
}

// needsQuotes reports whether DuckDB quotes s nested in another value: empty,
// edge whitespace, null, or a nestedSpecial character.
func needsQuotes(s string) bool {
	return s == "" || isASCIISpace(s[0]) || isASCIISpace(s[len(s)-1]) ||
		strings.EqualFold(s, "null") || strings.ContainsAny(s, nestedSpecial)
}

func isASCIISpace(c byte) bool {
	return c == ' ' || (c >= '\t' && c <= '\r')
}

// escapeNested backslash-escapes backslashes and single quotes for a quoted nested value or STRUCT field name.
func escapeNested(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s)
}
