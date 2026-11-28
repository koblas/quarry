package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_every_key_load_reads_is_on_the_config_reference(t *testing.T) {
	page, err := os.ReadFile("../../docs/reference/config.md")
	require.NoError(t, err)
	keys := config.KnownKeys()
	require.GreaterOrEqual(t, len(keys), 10, "found too few keys")

	for _, key := range keys {
		if !strings.Contains(key, ".") {
			continue
		}
		assert.Contains(t, string(page), "`"+key+"`", "config key %s is not on docs/reference/config.md", key)
	}
}

func Test_the_config_reference_example_loads_without_a_warning(t *testing.T) {
	example := exampleFrom(t, "../../docs/reference/config.md")

	_, cfg, err := load(t, example)

	require.NoError(t, err)
	assert.Empty(t, cfg.Warnings)
}

// exampleFrom is the first ```toml block on the Markdown page at path.
func exampleFrom(t *testing.T, path string) string {
	t.Helper()
	page, err := os.ReadFile(path)
	require.NoError(t, err)
	_, after, found := strings.Cut(string(page), "```toml\n")
	require.True(t, found, "%s has no toml example", path)
	example, _, _ := strings.Cut(after, "```")

	return example
}
