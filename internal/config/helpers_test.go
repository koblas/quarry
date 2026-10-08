package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	shownPath = "~/Library/Application Support/quarry/config.toml"
	fixLine   = "; fix the file and run the command again"
)

// newHome is a fresh home and the config file's path under it, its folder made.
func newHome(tb testing.TB) (string, string) {
	tb.Helper()
	home := tb.TempDir()
	path := filepath.Join(home, "Library", "Application Support", "quarry", "config.toml")
	require.NoError(tb, os.MkdirAll(filepath.Dir(path), 0o700))

	return home, path
}

// load writes content as the config file under a fresh home and loads it.
func load(tb testing.TB, content string) (string, config.Config, error) {
	tb.Helper()
	home, path := newHome(tb)
	require.NoError(tb, os.WriteFile(path, []byte(content), 0o600))
	cfg, err := config.Load(home, path)

	return home, cfg, err
}

// skipIfReadable skips the test when path can be read despite its mode, as it can by root.
func skipIfReadable(tb testing.TB, path string) {
	tb.Helper()
	if _, err := os.ReadFile(path); err == nil {
		tb.Skip("running with permission to read a mode 000 file")
	}
}

// refusal loads content and returns the refusal text, requiring an empty Config with it.
func refusal(tb testing.TB, content string) string {
	tb.Helper()
	_, cfg, err := load(tb, content)
	require.Error(tb, err)
	assert.Zero(tb, cfg)

	return err.Error()
}

// loadPath writes content as the config file under a fresh home and returns the home and the file's path;
// err is the error loading it gives.
func loadPath(tb testing.TB, content string) (string, string, error) {
	tb.Helper()
	home, path := newHome(tb)
	require.NoError(tb, os.WriteFile(path, []byte(content), 0o600))
	_, err := config.Load(home, path)

	return home, path, err
}
