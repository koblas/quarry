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
	output string // stdout and combined, unless stdout is set
	stdout string // when set, stdout alone; combined stays output
	status int
	err    error
	cancel bool // the call cancels the context set by cancellable before answering
	ctxErr bool // the call fails with the Err of the context it was given
}

// fakeClaude is a Runner that records each call's name and argv and answers
// from replies keyed by argv; an unscripted argv succeeds with empty output.
type fakeClaude struct {
	replies map[string]reply
	calls   []string
	names   []string
	cancel  context.CancelFunc
}

// cancellable returns a context that a reply with cancel set ends.
func (f *fakeClaude) cancellable(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	f.cancel = cancel
	return ctx
}

// newFakeClaude scripts the two lists with the given JSON bodies.
func newFakeClaude(marketplaces, plugins string) *fakeClaude {
	return &fakeClaude{replies: map[string]reply{
		marketplaceList: {output: marketplaces},
		pluginList:      {output: plugins},
	}}
}

func (f *fakeClaude) answer(argv string, r reply) { f.replies[argv] = r }

func (f *fakeClaude) run(ctx context.Context, name string, args ...string) ([]byte, []byte, int, error) {
	argv := strings.Join(args, " ")
	f.names = append(f.names, name)
	f.calls = append(f.calls, argv)
	r := f.replies[argv]
	if r.cancel {
		f.cancel()
	}
	err := r.err
	if r.ctxErr {
		err = ctx.Err()
	}
	stdout := r.output
	if r.stdout != "" {
		stdout = r.stdout
	}
	return []byte(stdout), []byte(r.output), r.status, err
}

func newServer(t *testing.T, f *fakeClaude) *claudeplugin.Server {
	t.Helper()
	return claudeplugin.NewServer(claudeplugin.WithRunner(f.run))
}

// found is a lookPath result: the path and error exec.LookPath would return.
type found struct {
	path string
	err  error
}

// fakePath is a LookPath that records each file asked for. A file with no
// entry in results resolves to binDir/<file>.
type fakePath struct {
	results map[string]found
	asked   []string
}

const binDir = "/opt/bin/"

func newFakePath() *fakePath { return &fakePath{results: map[string]found{}} }

func (f *fakePath) look(file string) (string, error) {
	f.asked = append(f.asked, file)
	if r, ok := f.results[file]; ok {
		return r.path, r.err
	}
	return binDir + file, nil
}

// newServerFinding is newServer with the commands looked up through path.
func newServerFinding(t *testing.T, f *fakeClaude, path *fakePath) *claudeplugin.Server {
	t.Helper()
	return claudeplugin.NewServer(claudeplugin.WithRunner(f.run), claudeplugin.WithLookPath(path.look))
}
