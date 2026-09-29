// White-box: hashFile has no injectable reader port, so its io.Copy
// failure needs a real fault. A directory opens fine on both darwin and
// Linux but fails its first Read with EISDIR, reliably reaching it.
package snapshot

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_hashFile_wraps_a_read_failure_after_a_successful_open(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, _, err := hashFile(dir)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "read snapshot: read "))
	assert.True(t, strings.HasSuffix(err.Error(), "is a directory"), err.Error())
	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr)
	assert.Equal(t, "read", pathErr.Op)
}
