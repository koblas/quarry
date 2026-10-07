package claudeplugin_test

import (
	"io/fs"
	"os/exec"
	"testing"

	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_install_runs_only_the_steps_that_are_missing(t *testing.T) {
	const (
		projectCopy = `[{"id":"quarry@quarry","scope":"project","enabled":true,"projectPath":"/src/foo"}]`
		offCopy     = `[{"id":"quarry@quarry","scope":"user","enabled":false}]`
	)
	cases := []struct {
		name         string
		marketplaces string
		plugins      string
		wantCalls    []string
		want         claudeplugin.Result
	}{
		{
			name:         "nothing present adds the marketplace then installs the plugin",
			marketplaces: "[]", plugins: "[]",
			wantCalls: []string{marketplaceList, pluginList, addMarketplace, installPlugin},
			want:      claudeplugin.Result{MarketplaceAdded: true, PluginInstalled: true},
		},
		{
			name:         "only the marketplace present installs the plugin",
			marketplaces: oursMarketplace, plugins: "[]",
			wantCalls: []string{marketplaceList, pluginList, installPlugin},
			want:      claudeplugin.Result{PluginInstalled: true},
		},
		{
			name:         "both present runs no step",
			marketplaces: oursMarketplace, plugins: userPlugin,
			wantCalls: []string{marketplaceList, pluginList},
			want:      claudeplugin.Result{},
		},
		{
			name:         "a user copy without the marketplace adds the marketplace only",
			marketplaces: "[]", plugins: userPlugin,
			wantCalls: []string{marketplaceList, pluginList, addMarketplace},
			want:      claudeplugin.Result{MarketplaceAdded: true},
		},
		{
			name:         "a project-only copy still gets the user copy installed, with no turned-off flag",
			marketplaces: oursMarketplace, plugins: projectCopy,
			wantCalls: []string{marketplaceList, pluginList, installPlugin},
			want:      claudeplugin.Result{PluginInstalled: true},
		},
		{
			name:         "a turned-off user copy runs no step and sets the turned-off flag",
			marketplaces: oursMarketplace, plugins: offCopy,
			wantCalls: []string{marketplaceList, pluginList},
			want:      claudeplugin.Result{UserCopyOff: true},
		},
		{
			name:         "a turned-off user copy without the marketplace adds it and sets the flag",
			marketplaces: "[]", plugins: offCopy,
			wantCalls: []string{marketplaceList, pluginList, addMarketplace},
			want:      claudeplugin.Result{MarketplaceAdded: true, UserCopyOff: true},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude(c.marketplaces, c.plugins)

			res, err := newServer(t, fake).Install(t.Context())

			require.NoError(t, err)
			assert.Equal(t, c.want, res)
			assert.Equal(t, c.wantCalls, fake.calls)
		})
	}
}

func Test_install_reports_that_a_step_ran_only_when_one_did(t *testing.T) {
	cases := []struct {
		name string
		res  claudeplugin.Result
		want bool
	}{
		{"no step", claudeplugin.Result{UserCopyOff: true}, false},
		{"the add step", claudeplugin.Result{MarketplaceAdded: true}, true},
		{"the install step", claudeplugin.Result{PluginInstalled: true}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.res.Ran())
		})
	}
}

func Test_install_refuses_a_foreign_marketplace_before_any_step(t *testing.T) {
	fake := newFakeClaude(`[{"name":"quarry","source":"directory","path":"/src"}]`, "[]")

	res, err := newServer(t, fake).Install(t.Context())

	require.ErrorIs(t, err, claudeplugin.ErrForeignMarketplace)
	assert.Equal(t, claudeplugin.Result{}, res)
	assert.Equal(t, []string{marketplaceList, pluginList}, fake.calls)
}

func Test_install_stops_when_the_add_step_exits_non_zero(t *testing.T) {
	fake := newFakeClaude("[]", "[]")
	fake.answer(addMarketplace, reply{output: "add failed", status: 1})

	res, err := newServer(t, fake).Install(t.Context())

	var exit *claudeplugin.ExitError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, &claudeplugin.ExitError{
		Argv: "claude plugin marketplace add --scope user koblas/quarry", Status: 1, Output: []byte("add failed"),
	}, exit)
	assert.Equal(t, claudeplugin.Result{}, res)
	assert.Equal(t, []string{marketplaceList, pluginList, addMarketplace}, fake.calls)
}

func Test_install_reports_the_added_marketplace_when_the_install_step_exits_non_zero(t *testing.T) {
	fake := newFakeClaude("[]", "[]")
	fake.answer(installPlugin, reply{output: "install failed", status: 2})

	res, err := newServer(t, fake).Install(t.Context())

	var exit *claudeplugin.ExitError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, &claudeplugin.ExitError{
		Argv: "claude plugin install --scope user quarry@quarry", Status: 2, Output: []byte("install failed"),
	}, exit)
	assert.Equal(t, claudeplugin.Result{MarketplaceAdded: true}, res)
}

func Test_install_returns_the_runner_error_from_a_step_unchanged(t *testing.T) {
	cases := []struct {
		name string
		argv string
		want claudeplugin.Result
	}{
		{"add step", addMarketplace, claudeplugin.Result{}},
		{"install step", installPlugin, claudeplugin.Result{MarketplaceAdded: true}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude("[]", "[]")
			fake.answer(c.argv, reply{err: errBoom})

			res, err := newServer(t, fake).Install(t.Context())

			assert.Same(t, errBoom, err)
			assert.Equal(t, c.want, res)
		})
	}
}

func Test_install_runs_each_child_as_claude_when_no_lookpath_is_set(t *testing.T) {
	fake := newFakeClaude("[]", "[]")

	_, err := newServer(t, fake).Install(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "claude", "claude", "claude"}, fake.names)
}

func Test_install_runs_each_child_at_the_path_lookpath_found(t *testing.T) {
	fake := newFakeClaude("[]", "[]")
	path := newFakePath()
	path.results["claude"] = found{path: "/home/ada/.local/bin/claude"}
	const at = "/home/ada/.local/bin/claude"

	_, err := newServerFinding(t, fake, path).Install(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{at, at, at, at}, fake.names)
	assert.Equal(t, []string{marketplaceList, pluginList, addMarketplace, installPlugin}, fake.calls)
}

func Test_install_refuses_before_any_child_when_claude_is_not_found(t *testing.T) {
	cases := []struct {
		name   string
		result found
	}{
		{"claude is not in any PATH directory", found{err: &exec.Error{Name: "claude", Err: exec.ErrNotFound}}},
		{"claude resolves only relative to the current directory", found{path: "./claude", err: &exec.Error{Name: "claude", Err: exec.ErrDot}}},
		{"claude is in a directory the user cannot search", found{err: &exec.Error{Name: "claude", Err: fs.ErrPermission}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude("[]", "[]")
			path := newFakePath()
			path.results["claude"] = c.result

			res, err := newServerFinding(t, fake, path).Install(t.Context())

			require.ErrorIs(t, err, claudeplugin.ErrClaudeNotFound)
			require.ErrorIs(t, err, c.result.err)
			assert.Equal(t, claudeplugin.Result{}, res)
			assert.Empty(t, fake.calls)
			assert.Equal(t, []string{"claude"}, path.asked)
		})
	}
}

func Test_install_flags_quarry_missing_from_the_path_only_after_the_steps_succeed(t *testing.T) {
	cases := []struct {
		name   string
		result found
		want   bool
	}{
		{"quarry is found", found{path: "/opt/bin/quarry"}, false},
		{"quarry is not found", found{err: &exec.Error{Name: "quarry", Err: exec.ErrNotFound}}, true},
		{"quarry resolves only relative to the current directory", found{path: "./quarry", err: &exec.Error{Name: "quarry", Err: exec.ErrDot}}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := newFakePath()
			path.results["quarry"] = c.result

			res, err := newServerFinding(t, newFakeClaude(oursMarketplace, userPlugin), path).Install(t.Context())

			require.NoError(t, err)
			assert.Equal(t, c.want, res.QuarryNotOnPath)
			assert.Equal(t, []string{"claude", "quarry"}, path.asked)
		})
	}
}

func Test_install_does_not_look_for_quarry_when_a_step_fails(t *testing.T) {
	fake := newFakeClaude("[]", "[]")
	fake.answer(installPlugin, reply{err: errBoom})
	path := newFakePath()
	path.results["quarry"] = found{err: &exec.Error{Name: "quarry", Err: exec.ErrNotFound}}

	res, err := newServerFinding(t, fake, path).Install(t.Context())

	require.Error(t, err)
	assert.False(t, res.QuarryNotOnPath)
	assert.Equal(t, []string{"claude"}, path.asked)
}
