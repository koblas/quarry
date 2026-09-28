package v9_test

import (
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Reference_reads_ZACCOUNTs_columns_from_the_embedded_schema(t *testing.T) {
	schema, err := v9.Reference(t.Context())

	require.NoError(t, err)
	assert.Contains(t, schema["ZACCOUNT"], "Z_PK")
	assert.Contains(t, schema["ZACCOUNT"], "ZNAME")
}
