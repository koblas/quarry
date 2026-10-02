package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_a_successful_call_writes_nothing_to_stderr(t *testing.T) {
	h := newHarness(t, &fakeStore{result: rowsOf(1)}, nil)

	result := h.query(t, map[string]any{"sql": "SELECT 1"})

	require.False(t, result.IsError, textOf(t, result))
	assert.Empty(t, h.stderr.String())
}

func Test_a_result_is_the_document_as_compact_text_and_as_structured_content(t *testing.T) {
	h := newHarness(t, &fakeStore{result: rowsOf(1)}, nil)

	result := h.query(t, map[string]any{"sql": "SELECT 1"})

	const want = `{"columns":[{"name":"n","type":"BIGINT"}],"rows":[[0]],"row_count":1,"limit":500,"truncated":false,"warnings":[]}`
	assert.Equal(t, want, textOf(t, result)) //nolint:testifylint // the key order is the contract and JSONEq ignores it
	assert.JSONEq(t, want, jsonOf(t, result.StructuredContent))
}

func Test_a_refused_call_is_one_text_block_and_one_stderr_line(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke)

	result := h.query(t, map[string]any{"sql": "SELECT 1"})

	assert.True(t, result.IsError)
	assert.Equal(t, "the report factory broke", textOf(t, result))
	assert.Nil(t, result.StructuredContent)
	assert.Equal(t, logPrefixQuery+"the report factory broke\n", h.stderr.String())
}

func Test_a_call_the_schema_refuses_writes_one_stderr_line(t *testing.T) {
	h := newHarness(t, &fakeStore{}, nil)

	result := h.query(t, map[string]any{"limit": 5})

	assert.True(t, result.IsError)
	assert.Equal(t, logPrefixQuery+textOf(t, result)+"\n", h.stderr.String())
	assert.Empty(t, h.store.asked)
}
