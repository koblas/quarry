package claudeplugin_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/claudeplugin"
)

const (
	marketplaceList = "plugin marketplace list --json"
	pluginList      = "plugin list --json"
	addMarketplace  = "plugin marketplace add --scope user koblas/quarry"
	installPlugin   = "plugin install --scope user quarry@quarry"

	oursMarketplace = `[{"name":"quarry","source":"github","repo":"koblas/quarry"}]`
	userPlugin      = `[{"id":"quarry@quarry","scope":"user","enabled":true}]`
)

// errBoom stands in for a child that cannot start.
var errBoom = errors.New("cannot start")

// reply is one scripted answer of the fake claude.
type reply struct {
	output string
	status int
	err    error
}

// fakeClaude is a Runner that records each call's argv and answers from
// replies keyed by argv; an unscripted argv succeeds with empty output.
type fakeClaude struct {
	replies map[string]reply
	calls   []string
}

// newFakeClaude scripts the two lists with the given JSON bodies.
func newFakeClaude(marketplaces, plugins string) *fakeClaude {
	return &fakeClaude{replies: map[string]reply{
		marketplaceList: {output: marketplaces},
		pluginList:      {output: plugins},
	}}
}

func (f *fakeClaude) answer(argv string, r reply) { f.replies[argv] = r }

func (f *fakeClaude) run(_ context.Context, _ string, args ...string) ([]byte, int, error) {
	argv := strings.Join(args, " ")
	f.calls = append(f.calls, argv)
	r := f.replies[argv]
	return []byte(r.output), r.status, r.err
}

func newServer(t *testing.T, f *fakeClaude) *claudeplugin.Server {
	t.Helper()
	return claudeplugin.NewServer(claudeplugin.WithRunner(f.run))
}
