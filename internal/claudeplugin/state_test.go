package claudeplugin_test

import (
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// marketplaceVerdict reports how Install saw the marketplace list: "foreign",
// "absent" (add ran) or "ours" (add skipped). The plugin list is empty.
func marketplaceVerdict(t *testing.T, marketplaces string) string {
	t.Helper()
	res, err := newServer(t, newFakeClaude(marketplaces, "[]")).Install(t.Context())
	if errors.Is(err, claudeplugin.ErrForeignMarketplace) {
		return "foreign"
	}
	require.NoError(t, err)
	if res.MarketplaceAdded {
		return "absent"
	}
	return "ours"
}

// pluginVerdict reports how Install saw the plugin list with our marketplace
// present: "absent" (install ran), "off" (installed, turned off) or "present".
func pluginVerdict(t *testing.T, plugins string) string {
	t.Helper()
	res, err := newServer(t, newFakeClaude(oursMarketplace, plugins)).Install(t.Context())
	require.NoError(t, err)
	if res.PluginInstalled {
		return "absent"
	}
	if res.UserCopyOff {
		return "off"
	}
	return "present"
}

func Test_read_state_classifies_quarry_marketplaces(t *testing.T) {
	cases := []struct {
		name         string
		marketplaces string
		want         string
	}{
		{"an empty list has no marketplace", `[]`, "absent"},
		{"other marketplaces are not quarry's", `[{"name":"caveman","source":"github","repo":"JuliusBrussee/caveman"}]`, "absent"},
		{"quarry on GitHub at koblas/quarry is ours", `[{"name":"quarry","source":"github","repo":"koblas/quarry"}]`, "ours"},
		{"quarry from a directory is foreign", `[{"name":"quarry","source":"directory","path":"/src/quarry"}]`, "foreign"},
		{"quarry without a source is foreign", `[{"name":"quarry","repo":"koblas/quarry"}]`, "foreign"},
		{"quarry on GitHub at another repo is foreign", `[{"name":"quarry","source":"github","repo":"someone/quarry"}]`, "foreign"},
		{"quarry on GitHub without a repo is foreign", `[{"name":"quarry","source":"github"}]`, "foreign"},
		{"the repo is compared case-sensitively", `[{"name":"quarry","source":"github","repo":"Koblas/quarry"}]`, "foreign"},
		{"the name is compared case-sensitively", `[{"name":"Quarry","source":"directory","path":"/src"}]`, "absent"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, marketplaceVerdict(t, c.marketplaces))
		})
	}
}

func Test_read_state_finds_the_user_copy(t *testing.T) {
	cases := []struct {
		name    string
		plugins string
		want    string
	}{
		{"an enabled user copy is present", `[{"id":"quarry@quarry","scope":"user","enabled":true}]`, "present"},
		{"a project-only copy is not the user copy", `[{"id":"quarry@quarry","scope":"project","enabled":true,"projectPath":"/src/foo"}]`, "absent"},
		{"a local-scope copy is not the user copy", `[{"id":"quarry@quarry","scope":"local","enabled":true}]`, "absent"},
		{"another plugin at user scope is not quarry", `[{"id":"caveman@caveman","scope":"user","enabled":true}]`, "absent"},
		{"enabled false turns the user copy off", `[{"id":"quarry@quarry","scope":"user","enabled":false}]`, "off"},
		{"a missing enabled counts as on", `[{"id":"quarry@quarry","scope":"user"}]`, "present"},
		{"a null enabled counts as on", `[{"id":"quarry@quarry","scope":"user","enabled":null}]`, "present"},
		{"a string enabled counts as on", `[{"id":"quarry@quarry","scope":"user","enabled":"false"}]`, "present"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, pluginVerdict(t, c.plugins))
		})
	}
}

func Test_read_state_refuses_an_unreadable_marketplace_list(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty output", ``},
		{"not JSON", `not json`},
		{"a JSON object", `{}`},
		{"null", `null`},
		{"an entry that is not an object", `[1]`},
		{"an entry without a name", `[{"source":"github"}]`},
		{"a name that is not a string", `[{"name":5}]`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude(c.body, "[]")

			_, err := newServer(t, fake).Install(t.Context())

			var unreadable *claudeplugin.ListUnreadableError
			require.ErrorAs(t, err, &unreadable)
			assert.Equal(t, "claude plugin marketplace list --json", unreadable.Argv)
			assert.Equal(t, []string{marketplaceList}, fake.calls)
		})
	}
}

func Test_read_state_refuses_an_unreadable_plugin_list(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty output", ``},
		{"not JSON", `not json`},
		{"a JSON object", `{}`},
		{"null", `null`},
		{"an entry that is not an object", `["quarry@quarry"]`},
		{"an entry without an id", `[{"scope":"user"}]`},
		{"an id that is not a string", `[{"id":5,"scope":"user"}]`},
		{"an entry without a scope", `[{"id":"quarry@quarry"}]`},
		{"a scope that is not a string", `[{"id":"quarry@quarry","scope":true}]`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude("[]", c.body)

			_, err := newServer(t, fake).Install(t.Context())

			var unreadable *claudeplugin.ListUnreadableError
			require.ErrorAs(t, err, &unreadable)
			assert.Equal(t, "claude plugin list --json", unreadable.Argv)
			assert.Equal(t, []string{marketplaceList, pluginList}, fake.calls)
		})
	}
}

func Test_read_state_reports_an_unreadable_plugin_list_before_a_foreign_marketplace(t *testing.T) {
	foreign := `[{"name":"quarry","source":"directory"}]`
	fake := newFakeClaude(foreign, `{}`)

	_, err := newServer(t, fake).Install(t.Context())

	var unreadable *claudeplugin.ListUnreadableError
	require.ErrorAs(t, err, &unreadable)
	assert.NotErrorIs(t, err, claudeplugin.ErrForeignMarketplace)
}

func Test_read_state_reports_a_foreign_marketplace_after_both_lists(t *testing.T) {
	fake := newFakeClaude(`[{"name":"quarry","source":"directory"}]`, "[]")

	_, err := newServer(t, fake).Install(t.Context())

	require.ErrorIs(t, err, claudeplugin.ErrForeignMarketplace)
	assert.Equal(t, []string{marketplaceList, pluginList}, fake.calls)
}

func Test_read_state_reports_a_list_that_exits_non_zero(t *testing.T) {
	cases := []struct {
		name  string
		argv  string
		reply reply
		want  *claudeplugin.ExitError
		calls []string
	}{
		{
			name:  "marketplace list",
			argv:  marketplaceList,
			reply: reply{output: "marketplace boom", status: 3},
			want:  &claudeplugin.ExitError{Argv: "claude plugin marketplace list --json", Status: 3, Output: []byte("marketplace boom")},
			calls: []string{marketplaceList},
		},
		{
			name:  "plugin list",
			argv:  pluginList,
			reply: reply{output: "plugin boom", status: 4},
			want:  &claudeplugin.ExitError{Argv: "claude plugin list --json", Status: 4, Output: []byte("plugin boom")},
			calls: []string{marketplaceList, pluginList},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude("[]", "[]")
			fake.answer(c.argv, c.reply)

			_, err := newServer(t, fake).Install(t.Context())

			var exit *claudeplugin.ExitError
			require.ErrorAs(t, err, &exit)
			assert.Equal(t, c.want, exit)
			assert.Equal(t, c.calls, fake.calls)
		})
	}
}

func Test_read_state_returns_the_runner_error_from_a_list_unchanged(t *testing.T) {
	cases := []struct {
		name  string
		argv  string
		calls []string
	}{
		{"marketplace list", marketplaceList, []string{marketplaceList}},
		{"plugin list", pluginList, []string{marketplaceList, pluginList}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFakeClaude("[]", "[]")
			fake.answer(c.argv, reply{err: errBoom})

			_, err := newServer(t, fake).Install(t.Context())

			assert.Same(t, errBoom, err)
			assert.Equal(t, c.calls, fake.calls)
		})
	}
}
