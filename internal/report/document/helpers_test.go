package document_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// indented encodes doc as the command line writes it: 2-space indented JSON with a trailing newline.
func indented(t *testing.T, doc any) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(doc))
	return buf.String()
}

// topLevelKeys is the keys of the JSON object in doc, in document order.
func topLevelKeys(t *testing.T, doc []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(doc))
	_, err := dec.Token()
	require.NoError(t, err)
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		require.NoError(t, err)
		name, ok := key.(string)
		require.True(t, ok)
		keys = append(keys, name)
		var skipped json.RawMessage
		require.NoError(t, dec.Decode(&skipped))
	}
	return keys
}
