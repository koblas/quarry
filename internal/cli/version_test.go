package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pseudoVersion = "v0.0.0-20261121151146-260d0d29f940"
	checkoutRev   = "260d0d29f940c1a7e0b5d3c8a1f2e4b6d8a0c2e4"
)

// buildInfoOf is a binary's build information: its main module version plus
// the VCS settings it recorded, if any.
func buildInfoOf(mainVersion string, settings ...debug.BuildSetting) *debug.BuildInfo {
	return &debug.BuildInfo{
		Main:     debug.Module{Version: mainVersion},
		Settings: settings,
	}
}

func Test_version_prints_the_binarys_version(t *testing.T) {
	cases := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{
			name: "a tagged install shows its tag",
			info: buildInfoOf("v1.0.0"),
			want: "quarry v1.0.0\n",
		},
		{
			name: "a clean checkout shows its pseudo-version",
			info: buildInfoOf(pseudoVersion,
				debug.BuildSetting{Key: "vcs.revision", Value: checkoutRev},
				debug.BuildSetting{Key: "vcs.modified", Value: "false"}),
			want: "quarry " + pseudoVersion + "\n",
		},
		{
			name: "a dirty checkout shows its pseudo-version with +dirty",
			info: buildInfoOf(pseudoVersion+"+dirty",
				debug.BuildSetting{Key: "vcs.revision", Value: checkoutRev},
				debug.BuildSetting{Key: "vcs.modified", Value: "true"}),
			want: "quarry " + pseudoVersion + "+dirty\n",
		},
		{
			name: "a devel build shows its short revision and +dirty",
			info: buildInfoOf("(devel)",
				debug.BuildSetting{Key: "vcs.revision", Value: checkoutRev},
				debug.BuildSetting{Key: "vcs.modified", Value: "true"}),
			want: "quarry (devel) 260d0d29f940+dirty\n",
		},
		{
			name: "a devel build with no VCS info shows (devel)",
			info: buildInfoOf("(devel)"),
			want: "quarry (devel)\n",
		},
		{
			name: "no build info shows (devel)",
			info: nil,
			want: "quarry (devel)\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			env := cli.Env{Stdout: &stdout, Stderr: &stderr, BuildInfo: c.info}

			err := cli.Execute(t.Context(), []string{"version"}, env)

			require.NoError(t, err)
			assert.Equal(t, c.want, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_version_needs_no_store_config_or_home(t *testing.T) {
	var calls []string
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: &stderr,
		Home:      "",
		BuildInfo: buildInfoOf("v1.0.0"),
		NewServer: func(context.Context, ...snapshot.Option) (*snapshot.Server, error) {
			calls = append(calls, "NewServer")
			return nil, io.ErrUnexpectedEOF
		},
		NewReport: func(context.Context, string) (*report.Server, error) {
			calls = append(calls, "NewReport")
			return nil, io.ErrUnexpectedEOF
		},
		NewSnapshots: func(context.Context, string) (*snapshot.Server, error) {
			calls = append(calls, "NewSnapshots")
			return nil, io.ErrUnexpectedEOF
		},
		LoadConfig: func(string) (config.Config, error) {
			calls = append(calls, "LoadConfig")
			return config.Config{}, io.ErrUnexpectedEOF
		},
	}

	err := cli.Execute(t.Context(), []string{"version"}, env)

	require.NoError(t, err)
	assert.Equal(t, "quarry v1.0.0\n", stdout.String())
	assert.Empty(t, calls)
}

const versionHelpLong = `Print the version of this quarry binary: the release tag, such as v1.0.0,
for a binary installed with go install
github.com/koblas/quarry/cmd/quarry@<version>, or Go's pseudo-version for
one built from a checkout, which names the commit and ends in +dirty when
the checkout had uncommitted changes.

version reads only the binary itself: it needs no store, reads no config
and never looks at Quicken.

--json adds the full commit, whether the checkout had uncommitted changes,
and the Go version quarry was built with.`

const helpWrapWidth = 76

var errVersionDiskFull = errors.New("no space left on device")

func Test_version_help_prints_the_ruled_long_text(t *testing.T) {
	var stdout bytes.Buffer

	err := cli.Execute(t.Context(), []string{"version", "--help"}, cli.Env{Stdout: &stdout, Stderr: io.Discard})

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(stdout.String(), versionHelpLong), stdout.String())
	for line := range strings.SplitSeq(versionHelpLong, "\n") {
		assert.LessOrEqual(t, len(line), helpWrapWidth, line)
	}
}

func Test_version_reports_a_failed_stdout_write(t *testing.T) {
	env := cli.Env{Stdout: failingWriter{err: errVersionDiskFull}, Stderr: io.Discard, BuildInfo: buildInfoOf("v1.0.0")}

	err := cli.Execute(t.Context(), []string{"version"}, env)

	require.ErrorIs(t, err, errVersionDiskFull)
	assert.EqualError(t, err, "cannot write the result to stdout: no space left on device")
}
