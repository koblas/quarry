package main

import (
	"io"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_version_refuses_arguments(t *testing.T) {
	exitCode, stdout, stderr := runCapture(t.Context(), []string{"version", "foo"})

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: version takes no arguments\n", stderr.String())
}

func Test_run_version_prints_without_a_home(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runCapture(t.Context(), []string{"version"})

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "quarry (devel)\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_defaultEnv_carries_this_binarys_build_info(t *testing.T) {
	want, ok := debug.ReadBuildInfo()
	require.True(t, ok)

	env := defaultEnv(io.Discard, io.Discard)

	assert.Equal(t, want, env.BuildInfo)
}

func Test_run_version_flag_refusals(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"before a subcommand", []string{"--version", "sync"}, "quarry: unknown flag: --version; Run 'quarry sync --help' for usage.\n"},
		{"after a subcommand", []string{"sync", "--version"}, "quarry: unknown flag: --version; Run 'quarry sync --help' for usage.\n"},
		{"with an extra word", []string{"--version", "extra"}, "quarry: unknown command \"extra\" for \"quarry\"; Run 'quarry --help' for usage.\n"},
		{"on the version command", []string{"version", "--version"}, "quarry: unknown flag: --version; Run 'quarry version --help' for usage.\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exitCode, stdout, stderr := runCapture(t.Context(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.want, stderr.String())
		})
	}
}

func Test_run_version_flag_prints_the_version_line_with_exit_0(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runCapture(t.Context(), []string{"--version"})

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "quarry (devel)\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_run_version_flag_with_help_prints_the_root_help(t *testing.T) {
	t.Setenv("HOME", "")
	_, help, _ := runCapture(t.Context(), []string{"--help"})

	exitCode, stdout, stderr := runCapture(t.Context(), []string{"--version", "--help"})

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, help.String(), stdout.String())
	assert.Contains(t, stdout.String(), "Usage:\n  quarry [flags]")
	assert.Empty(t, stderr.String())
}

func Test_run_root_without_a_subcommand_prints_the_help_it_always_has(t *testing.T) {
	t.Setenv("HOME", "")
	_, help, _ := runCapture(t.Context(), []string{"--help"})
	require.NotEmpty(t, help.String())

	for _, args := range [][]string{{}, {"--json"}} {
		t.Run(strings.Join(append([]string{"quarry"}, args...), " "), func(t *testing.T) {
			exitCode, stdout, stderr := runCapture(t.Context(), args)

			assert.Equal(t, 0, exitCode)
			assert.Equal(t, help.String(), stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_run_root_refuses_an_unknown_command_naming_it(t *testing.T) {
	exitCode, stdout, stderr := runCapture(t.Context(), []string{"bogus"})

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: unknown command \"bogus\" for \"quarry\"; Run 'quarry --help' for usage.\n", stderr.String())
}
