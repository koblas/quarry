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
		{"a kept marketplace with a remaining copy", claudeplugin.UninstallResult{
			MarketplaceKept: true, Remaining: []claudeplugin.Copy{{Scope: "project"}},
		}, false},
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

func Test_uninstall_keeps_the_marketplace_while_a_non_user_copy_remains(t *testing.T) {
	const user = `{"id":"quarry@quarry","scope":"user","enabled":true}`
	scoped := func(scope string) string {
		return `{"id":"quarry@quarry","scope":"` + scope + `","projectPath":"/src/foo"}`
	}
	withUser := []string{marketplaceList, pluginList, uninstallPlugin}
	cases := []struct {
		name      string
		plugins   string
		wantCalls []string
		want      claudeplugin.UninstallResult
	}{
		{
			name:      "a project copy beside the user copy",
			plugins:   "[" + user + "," + scoped("project") + "]",
			wantCalls: withUser,
			want: claudeplugin.UninstallResult{
				PluginUninstalled: true, MarketplaceKept: true,
				Remaining: []claudeplugin.Copy{{Scope: "project", ProjectPath: "/src/foo"}},
			},
		},
		{
			name:      "a local copy beside the user copy",
			plugins:   "[" + user + "," + scoped("local") + "]",
			wantCalls: withUser,
			want: claudeplugin.UninstallResult{
				PluginUninstalled: true, MarketplaceKept: true,
				Remaining: []claudeplugin.Copy{{Scope: "local", ProjectPath: "/src/foo"}},
			},
		},
		{
			name:      "a managed copy beside the user copy",
			plugins:   "[" + user + "," + scoped("managed") + "]",
			wantCalls: withUser,
			want: claudeplugin.UninstallResult{
				PluginUninstalled: true, MarketplaceKept: true,
				Remaining: []claudeplugin.Copy{{Scope: "managed", ProjectPath: "/src/foo"}},
			},
		},
		{
			name:      "a scope claude has not documented counts as a copy",
			plugins:   "[" + user + "," + scoped("enterprise") + "]",
			wantCalls: withUser,
			want: claudeplugin.UninstallResult{
				PluginUninstalled: true, MarketplaceKept: true,
				Remaining: []claudeplugin.Copy{{Scope: "enterprise", ProjectPath: "/src/foo"}},
			},
		},
		{
			name:      "a project copy alone runs no step",
			plugins:   "[" + scoped("project") + "]",
			wantCalls: []string{marketplaceList, pluginList},
			want: claudeplugin.UninstallResult{
				MarketplaceKept: true,
				Remaining:       []claudeplugin.Copy{{Scope: "project", ProjectPath: "/src/foo"}},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude(oursMarketplace, c.plugins)

			res, err := newServer(t, fake).Uninstall(t.Context())

			require.NoError(t, err)
			assert.Equal(t, c.want, res)
			assert.Equal(t, c.wantCalls, fake.calls)
		})
	}
}

func Test_uninstall_reports_a_non_user_copy_without_a_marketplace_to_keep(t *testing.T) {
	fake := newFakeClaude("[]", `[{"id":"quarry@quarry","scope":"project","projectPath":"/src/foo"}]`)

	res, err := newServer(t, fake).Uninstall(t.Context())

	require.NoError(t, err)
	assert.Equal(t, claudeplugin.UninstallResult{
		Remaining: []claudeplugin.Copy{{Scope: "project", ProjectPath: "/src/foo"}},
	}, res)
	assert.Equal(t, []string{marketplaceList, pluginList}, fake.calls)
}

func Test_uninstall_reports_a_non_user_copy_beside_a_user_copy_without_a_marketplace_to_keep(t *testing.T) {
	fake := newFakeClaude("[]", `[{"id":"quarry@quarry","scope":"user","enabled":true},`+
		`{"id":"quarry@quarry","scope":"project","projectPath":"/src/foo"}]`)

	res, err := newServer(t, fake).Uninstall(t.Context())

	require.NoError(t, err)
	assert.Equal(t, claudeplugin.UninstallResult{
		PluginUninstalled: true,
		Remaining:         []claudeplugin.Copy{{Scope: "project", ProjectPath: "/src/foo"}},
	}, res)
	assert.Equal(t, []string{marketplaceList, pluginList, uninstallPlugin}, fake.calls)
}

func Test_uninstall_does_not_count_another_plugins_copy_as_ours(t *testing.T) {
	cases := []struct {
		name  string
		scope string
	}{
		{"another plugin at project scope", "project"},
		{"another plugin at local scope", "local"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plugins := `[{"id":"quarry@quarry","scope":"user"},` +
				`{"id":"quarry@other","scope":"` + c.scope + `","projectPath":"/src/foo"}]`
			fake := newFakeClaude(oursMarketplace, plugins)

			res, err := newServer(t, fake).Uninstall(t.Context())

			require.NoError(t, err)
			assert.Equal(t, claudeplugin.UninstallResult{PluginUninstalled: true, MarketplaceRemoved: true}, res)
			assert.Equal(t, []string{marketplaceList, pluginList, uninstallPlugin, removeMarketplace}, fake.calls)
		})
	}
}

func Test_uninstall_records_an_empty_project_path_when_claude_gave_no_usable_one(t *testing.T) {
	cases := []struct {
		name  string
		entry string
	}{
		{"the key is absent", `{"id":"quarry@quarry","scope":"project"}`},
		{"the value is a number", `{"id":"quarry@quarry","scope":"project","projectPath":7}`},
		{"the value is null", `{"id":"quarry@quarry","scope":"project","projectPath":null}`},
		{"the value is empty", `{"id":"quarry@quarry","scope":"project","projectPath":""}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude(oursMarketplace, "["+c.entry+"]")

			res, err := newServer(t, fake).Uninstall(t.Context())

			require.NoError(t, err)
			assert.Equal(t, []claudeplugin.Copy{{Scope: "project"}}, res.Remaining)
			assert.True(t, res.MarketplaceKept)
		})
	}
}

func Test_uninstall_lists_the_remaining_copies_in_the_order_claude_listed_them(t *testing.T) {
	plugins := `[` +
		`{"id":"quarry@quarry","scope":"project","projectPath":"/src/b"},` +
		`{"id":"quarry@quarry","scope":"local"},` +
		`{"id":"quarry@quarry","scope":"project","projectPath":"/src/a"}]`
	fake := newFakeClaude(oursMarketplace, plugins)

	res, err := newServer(t, fake).Uninstall(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []claudeplugin.Copy{
		{Scope: "project", ProjectPath: "/src/b"},
		{Scope: "local"},
		{Scope: "project", ProjectPath: "/src/a"},
	}, res.Remaining)
}

func Test_uninstall_reports_no_kept_marketplace_when_the_plugin_step_fails_beside_a_project_copy(t *testing.T) {
	plugins := `[{"id":"quarry@quarry","scope":"user"},{"id":"quarry@quarry","scope":"project","projectPath":"/src/foo"}]`
	fake := newFakeClaude(oursMarketplace, plugins)
	fake.answer(uninstallPlugin, reply{output: "uninstall failed", status: 1})

	res, err := newServer(t, fake).Uninstall(t.Context())

	require.ErrorAs(t, err, new(*claudeplugin.ExitError))
	assert.Equal(t, claudeplugin.UninstallResult{}, res)
	assert.Equal(t, []string{marketplaceList, pluginList, uninstallPlugin}, fake.calls)
}
