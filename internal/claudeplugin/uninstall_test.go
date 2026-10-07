package claudeplugin_test

import (
	"context"
	"io/fs"
	"os/exec"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/koblas/quarry/internal/platform/toolrun"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	uninstallPlugin   = "plugin uninstall --scope user quarry@quarry"
	removeMarketplace = "plugin marketplace remove --scope user quarry"
)

func Test_uninstall_refuses_a_foreign_marketplace_before_any_step(t *testing.T) {
	const (
		foreign = `{"name":"quarry","source":"directory","path":"/src"}`
		ours    = `{"name":"quarry","source":"github","repo":"koblas/quarry"}`
	)
	cases := []struct {
		name         string
		marketplaces string
	}{
		{"a foreign marketplace alone", "[" + foreign + "]"},
		{"ours listed before a foreign one", "[" + ours + "," + foreign + "]"},
		{"ours listed after a foreign one", "[" + foreign + "," + ours + "]"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude(c.marketplaces, userPlugin)

			res, err := newServer(t, fake).Uninstall(t.Context())

			require.ErrorIs(t, err, claudeplugin.ErrForeignMarketplace)
			assert.Equal(t, claudeplugin.UninstallResult{}, res)
			assert.Equal(t, []string{marketplaceList, pluginList}, fake.calls)
		})
	}
}

func Test_uninstall_runs_only_the_steps_that_are_present(t *testing.T) {
	const offCopy = `[{"id":"quarry@quarry","scope":"user","enabled":false}]`
	cases := []struct {
		name         string
		marketplaces string
		plugins      string
		wantCalls    []string
		want         claudeplugin.UninstallResult
	}{
		{
			name:         "both present uninstalls the plugin then removes the marketplace",
			marketplaces: oursMarketplace, plugins: userPlugin,
			wantCalls: []string{marketplaceList, pluginList, uninstallPlugin, removeMarketplace},
			want:      claudeplugin.UninstallResult{PluginUninstalled: true, MarketplaceRemoved: true},
		},
		{
			name:         "neither present runs no step",
			marketplaces: "[]", plugins: "[]",
			wantCalls: []string{marketplaceList, pluginList},
			want:      claudeplugin.UninstallResult{},
		},
		{
			name:         "plugin only uninstalls the plugin",
			marketplaces: "[]", plugins: userPlugin,
			wantCalls: []string{marketplaceList, pluginList, uninstallPlugin},
			want:      claudeplugin.UninstallResult{PluginUninstalled: true},
		},
		{
			name:         "marketplace only removes the marketplace",
			marketplaces: oursMarketplace, plugins: "[]",
			wantCalls: []string{marketplaceList, pluginList, removeMarketplace},
			want:      claudeplugin.UninstallResult{MarketplaceRemoved: true},
		},
		{
			name:         "a turned-off user copy is still uninstalled",
			marketplaces: oursMarketplace, plugins: offCopy,
			wantCalls: []string{marketplaceList, pluginList, uninstallPlugin, removeMarketplace},
			want:      claudeplugin.UninstallResult{PluginUninstalled: true, MarketplaceRemoved: true},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude(c.marketplaces, c.plugins)

			res, err := newServer(t, fake).Uninstall(t.Context())

			require.NoError(t, err)
			assert.Equal(t, c.want, res)
			assert.Equal(t, c.wantCalls, fake.calls)
		})
	}
}

func Test_uninstall_reports_that_a_step_ran_only_when_one_did(t *testing.T) {
	cases := []struct {
		name string
		res  claudeplugin.UninstallResult
		want bool
	}{
		{"no step", claudeplugin.UninstallResult{}, false},
		{"the uninstall step", claudeplugin.UninstallResult{PluginUninstalled: true}, true},
		{"the remove step", claudeplugin.UninstallResult{MarketplaceRemoved: true}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.res.Ran())
		})
	}
}

func Test_uninstall_refuses_before_any_child_when_claude_is_not_found(t *testing.T) {
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
			fake := newFakeClaude(oursMarketplace, userPlugin)
			path := newFakePath()
			path.results["claude"] = c.result

			res, err := newServerFinding(t, fake, path).Uninstall(t.Context())

			require.ErrorIs(t, err, claudeplugin.ErrClaudeNotFound)
			require.ErrorIs(t, err, c.result.err)
			assert.Equal(t, claudeplugin.UninstallResult{}, res)
			assert.Empty(t, fake.calls)
		})
	}
}

func Test_uninstall_runs_each_child_at_the_path_lookpath_found(t *testing.T) {
	const at = "/home/ada/.local/bin/claude"
	fake := newFakeClaude(oursMarketplace, userPlugin)
	path := newFakePath()
	path.results["claude"] = found{path: at}

	_, err := newServerFinding(t, fake, path).Uninstall(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{at, at, at, at}, fake.names)
	assert.Equal(t, []string{"claude"}, path.asked)
}

func Test_uninstall_stops_when_the_plugin_step_exits_non_zero(t *testing.T) {
	fake := newFakeClaude(oursMarketplace, userPlugin)
	fake.answer(uninstallPlugin, reply{output: "uninstall failed", status: 1})

	res, err := newServer(t, fake).Uninstall(t.Context())

	var exit *claudeplugin.ExitError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, &claudeplugin.ExitError{
		Argv: "claude plugin uninstall --scope user quarry@quarry", Status: 1, Output: []byte("uninstall failed"),
	}, exit)
	assert.Equal(t, claudeplugin.UninstallResult{}, res)
	assert.Equal(t, []string{marketplaceList, pluginList, uninstallPlugin}, fake.calls)
}

func Test_uninstall_reports_the_uninstalled_plugin_when_the_marketplace_step_exits_non_zero(t *testing.T) {
	fake := newFakeClaude(oursMarketplace, userPlugin)
	fake.answer(removeMarketplace, reply{output: "remove failed", status: 2})

	res, err := newServer(t, fake).Uninstall(t.Context())

	var exit *claudeplugin.ExitError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, &claudeplugin.ExitError{
		Argv: "claude plugin marketplace remove --scope user quarry", Status: 2, Output: []byte("remove failed"),
	}, exit)
	assert.Equal(t, claudeplugin.UninstallResult{PluginUninstalled: true}, res)
}

func Test_uninstall_returns_the_runner_error_from_a_step_unchanged(t *testing.T) {
	cases := []struct {
		name string
		argv string
		want claudeplugin.UninstallResult
	}{
		{"plugin step", uninstallPlugin, claudeplugin.UninstallResult{}},
		{"marketplace step", removeMarketplace, claudeplugin.UninstallResult{PluginUninstalled: true}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude(oursMarketplace, userPlugin)
			fake.answer(c.argv, reply{err: errBoom})

			res, err := newServer(t, fake).Uninstall(t.Context())

			assert.Same(t, errBoom, err)
			assert.Equal(t, c.want, res)
		})
	}
}

func Test_uninstall_reports_an_interrupt_naming_the_command_it_stopped(t *testing.T) {
	lists := []string{marketplaceList, pluginList}
	cases := []struct {
		name         string
		marketplaces string
		plugins      string
		answers      map[string]reply
		wantArgv     string
		wantCalls    []string
		wantResult   claudeplugin.UninstallResult
	}{
		{
			name: "during the marketplace list", marketplaces: oursMarketplace, plugins: userPlugin,
			answers:  map[string]reply{marketplaceList: {cancel: true, ctxErr: true}},
			wantArgv: "claude " + marketplaceList, wantCalls: []string{marketplaceList},
		},
		{
			name: "during the plugin list", marketplaces: oursMarketplace, plugins: userPlugin,
			answers:  map[string]reply{pluginList: {cancel: true, ctxErr: true}},
			wantArgv: "claude " + pluginList, wantCalls: lists,
		},
		{
			name: "after the plugin list with the plugin present", marketplaces: oursMarketplace, plugins: userPlugin,
			answers:  map[string]reply{pluginList: {output: userPlugin, cancel: true}},
			wantArgv: "claude " + uninstallPlugin, wantCalls: lists,
		},
		{
			name: "after the plugin list with the plugin absent", marketplaces: oursMarketplace, plugins: "[]",
			answers:  map[string]reply{pluginList: {output: "[]", cancel: true}},
			wantArgv: "claude " + removeMarketplace, wantCalls: lists,
		},
		{
			name: "during the plugin uninstall", marketplaces: oursMarketplace, plugins: userPlugin,
			answers:  map[string]reply{uninstallPlugin: {cancel: true, ctxErr: true}},
			wantArgv: "claude " + uninstallPlugin, wantCalls: []string{marketplaceList, pluginList, uninstallPlugin},
		},
		{
			name: "after the plugin uninstall succeeds", marketplaces: oursMarketplace, plugins: userPlugin,
			answers:  map[string]reply{uninstallPlugin: {cancel: true}},
			wantArgv: "claude " + removeMarketplace, wantCalls: []string{marketplaceList, pluginList, uninstallPlugin},
			wantResult: claudeplugin.UninstallResult{PluginUninstalled: true},
		},
		{
			name: "during the marketplace remove", marketplaces: oursMarketplace, plugins: userPlugin,
			answers:  map[string]reply{removeMarketplace: {cancel: true, ctxErr: true}},
			wantArgv: "claude " + removeMarketplace, wantCalls: []string{marketplaceList, pluginList, uninstallPlugin, removeMarketplace},
			wantResult: claudeplugin.UninstallResult{PluginUninstalled: true},
		},
		{
			name: "a signal while the context is done", marketplaces: oursMarketplace, plugins: userPlugin,
			answers: map[string]reply{
				removeMarketplace: {cancel: true, err: &toolrun.SignalError{Signal: syscall.SIGKILL}},
			},
			wantArgv: "claude " + removeMarketplace, wantCalls: []string{marketplaceList, pluginList, uninstallPlugin, removeMarketplace},
			wantResult: claudeplugin.UninstallResult{PluginUninstalled: true},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude(c.marketplaces, c.plugins)
			for argv, r := range c.answers {
				fake.answer(argv, r)
			}
			ctx := fake.cancellable(t)

			res, err := newServer(t, fake).Uninstall(ctx)

			var interrupted *claudeplugin.InterruptedError
			require.ErrorAs(t, err, &interrupted)
			assert.Equal(t, c.wantArgv, interrupted.Argv)
			require.ErrorIs(t, err, context.Canceled)
			assert.NotErrorAs(t, err, new(*claudeplugin.ExitError))
			assert.Equal(t, c.wantCalls, fake.calls)
			assert.Equal(t, c.wantResult, res)
		})
	}
}

func Test_uninstall_reports_a_signal_that_stopped_a_step(t *testing.T) {
	fake := newFakeClaude(oursMarketplace, userPlugin)
	fake.answer(removeMarketplace, reply{output: "partial", status: -1, err: &toolrun.SignalError{Signal: syscall.SIGKILL}})

	res, err := newServer(t, fake).Uninstall(t.Context())

	var exit *claudeplugin.ExitError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, syscall.SIGKILL, exit.Signal)
	assert.Equal(t, claudeplugin.UninstallResult{PluginUninstalled: true}, res)
}
