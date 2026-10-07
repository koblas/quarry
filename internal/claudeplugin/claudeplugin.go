package claudeplugin

import "context"

// Runner runs the command name with args and returns its combined output and
// exit status. err is non-nil only when the command could not start or ctx
// ended; a non-zero exit status comes back with a nil err.
type Runner func(ctx context.Context, name string, args ...string) (output []byte, exitCode int, err error)

const (
	addMarketplaceArgs = "plugin marketplace add --scope user " + marketplaceRepo
	installPluginArgs  = "plugin install --scope user " + pluginID
)

// Server installs quarry's Claude Code plugin through a Runner.
type Server struct {
	runner Runner
}

// Option configures a Server.
type Option func(*Server)

// WithRunner sets how claude children run. A Server needs one before Install.
func WithRunner(r Runner) Option {
	return func(s *Server) { s.runner = r }
}

// NewServer returns a Server configured by opts.
func NewServer(opts ...Option) *Server {
	s := &Server{}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Result reports what an Install did.
type Result struct {
	MarketplaceAdded bool // the add step ran
	PluginInstalled  bool // the install step ran
	UserCopyOff      bool // the user copy is installed but turned off
}

// Ran reports whether any claude step changed something.
func (r Result) Ran() bool {
	return r.MarketplaceAdded || r.PluginInstalled
}

// Install adds quarry's marketplace and installs its plugin at user scope, skipping
// each step the lists show done, and returns the steps completed even on failure.
// Errors: ErrForeignMarketplace, *ListUnreadableError, *ExitError, or the Runner's own.
func (s *Server) Install(ctx context.Context) (Result, error) {
	st, err := s.readState(ctx)
	if err != nil {
		return Result{}, err
	}

	var res Result
	if !st.marketplaceOurs {
		if _, err := s.run(ctx, addMarketplaceArgs); err != nil {
			return res, err
		}
		res.MarketplaceAdded = true
	}
	if !st.userCopy {
		if _, err := s.run(ctx, installPluginArgs); err != nil {
			return res, err
		}
		res.PluginInstalled = true
	}
	res.UserCopyOff = st.userCopyOff
	return res, nil
}
