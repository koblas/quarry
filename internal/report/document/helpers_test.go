package document_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// civil is the given calendar day at UTC midnight.
func civil(year int, month time.Month, d int) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}

// day is the given day of 2026.
func day(month time.Month, d int) time.Time {
	return civil(2026, month, d)
}

// march is the given day of March 2026.
func march(d int) time.Time {
	return day(time.March, d)
}

// indented encodes doc as the command line writes it: 2-space indented JSON with a trailing newline.
func indented(tb testing.TB, doc any) string {
	tb.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	require.NoError(tb, enc.Encode(doc))
	return buf.String()
}

// mustJSON is v as compact JSON.
func mustJSON(tb testing.TB, v any) []byte {
	tb.Helper()
	data, err := json.Marshal(v)
	require.NoError(tb, err)
	return data
}

// mustReadBack encodes doc as the command line writes it and decodes that text into into, requiring both to work.
func mustReadBack(tb testing.TB, doc, into any) {
	tb.Helper()
	require.NoError(tb, json.Unmarshal([]byte(indented(tb, doc)), into))
}

// topLevelKeys is the keys of the JSON object in doc, in document order.
func topLevelKeys(tb testing.TB, doc []byte) []string {
	tb.Helper()
	dec := json.NewDecoder(bytes.NewReader(doc))
	_, err := dec.Token()
	require.NoError(tb, err)
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		require.NoError(tb, err)
		name, ok := key.(string)
		require.True(tb, ok)
		keys = append(keys, name)
		var skipped json.RawMessage
		require.NoError(tb, dec.Decode(&skipped))
	}
	return keys
}
