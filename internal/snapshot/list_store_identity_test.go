package snapshot_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipOnCaseSensitiveVolume skips t unless the volume under dir resolves a file name in any letter case.
func skipOnCaseSensitiveVolume(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "caseprobe"), nil, 0o600))
	if _, err := os.Stat(filepath.Join(dir, "CASEPROBE")); err != nil {
		t.Skip("the volume is case-sensitive: a differently-cased name does not resolve")
	}
}

func Test_list_marks_the_stores_snapshot_recorded_in_a_different_letter_case(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	skipOnCaseSensitiveVolume(t, home)
	dir := threeSnapshots(t, home)
	recorded := filepath.Join(dir, strings.ToLower(idMiddle)+".sqlite")

	listing, err := newListServer(home, &fakeStoreProbe{builtFrom: recorded}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{false, true, false}, storeFlags(listing))
}

func Test_list_marks_the_snapshot_whose_id_differs_from_the_recorded_one_only_in_letter_case(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	threeSnapshots(t, home)
	moved := filepath.Join(home, "moved-away", strings.ToLower(idMiddle)+".sqlite")

	listing, err := newListServer(home, &fakeStoreProbe{builtFrom: moved}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{false, true, false}, storeFlags(listing))
}

func Test_list_marks_the_snapshot_the_recorded_path_hard_links_to(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := threeSnapshots(t, home)
	link := filepath.Join(home, "linked.sqlite")
	require.NoError(t, os.Link(filepath.Join(dir, idNewest+".sqlite"), link))

	listing, err := newListServer(home, &fakeStoreProbe{builtFrom: link}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{true, false, false}, storeFlags(listing))
}
