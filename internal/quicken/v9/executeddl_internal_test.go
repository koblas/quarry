// White-box: executeDDL is unexported and has no other entry point that lets
// a test supply malformed DDL without also depending on the real, large
// embedded reference.sql.
package v9

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_executeDDL_fails_on_malformed_ddl(t *testing.T) {
	_, err := executeDDL(t.Context(), "CREATE TABLE (broken")

	require.Error(t, err)
}
