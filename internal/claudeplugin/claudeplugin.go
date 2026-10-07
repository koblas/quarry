package claudeplugin

import (
	"context"
	"fmt"
)

// Runner runs the command name with args and returns its combined output and
// exit status. err is non-nil only when the command could not start, a signal
// ended it, or ctx ended; a non-zero exit status comes back with a nil err.
type Runner func(ctx context.Context, name string, args ...string) (output []byte, exitCode int, err error)

// LookPath resolves the command file to the path that would run, as exec.LookPath does.
type LookPath func(file string) (string, error)

const (
	addMarketplaceArgs = "plugin marketplace add --scope user " + marketplaceRepo
	installPluginArgs  = "plugin install --scope user " + pluginID
)

// Server installs quarry's Claude Code plugin through a Runner.
type Server struct {
	runner   Runner
	lookPath LookPath
}

// Option configures a Server.
type Option func(*Server)

// WithRunner sets how claude children run. A Server needs one before Install.
func WithRunner(r Runner) Option {
	return func(s *Server) { s.runner = r }
}

// WithLookPath sets how Install finds the claude and quarry commands. Without
// it, Install runs "claude" as named and does not look for quarry.
func WithLookPath(l LookPath) Option {
	return func(s *Server) { s.lookPath = l }
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
	QuarryNotOnPath  bool // the plugin starts quarry from PATH and PATH has none
}

// Ran reports whether any claude step changed something.
func (r Result) Ran() bool {
	return r.MarketplaceAdded || r.PluginInstalled
}

// Install adds quarry's marketplace and installs its plugin at user scope, skipping
// each step the lists show done, and returns the steps completed even on failure.
// Errors: ErrClaudeNotFound, ErrForeignMarketplace, *ListUnreadableError, *ExitError,
// or the Runner's own.
func (s *Server) Install(ctx context.Context) (Result, error) {
	claude, err := s.findClaude()
	if err != nil {
		return Result{}, err
	}
	st, err := s.readState(ctx, claude)
	if err != nil {
		return Result{}, err
	}

	var res Result
	if !st.marketplaceOurs {
		if _, err := s.run(ctx, claude, addMarketplaceArgs); err != nil {
			return res, err
		}
		res.MarketplaceAdded = true
	}
	if !st.userCopy {
		if _, err := s.run(ctx, claude, installPluginArgs); err != nil {
			return res, err
		}
		res.PluginInstalled = true
	}
	res.UserCopyOff = st.userCopyOff
	res.QuarryNotOnPath = s.quarryNotOnPath()
	return res, nil
}

// findClaude returns the command every child runs: the path lookPath resolved, else "claude".
// Any lookup error, even one that came with a path, is ErrClaudeNotFound.
func (s *Server) findClaude() (string, error) {
	if s.lookPath == nil {
		return claudeCommand, nil
	}
	path, err := s.lookPath(claudeCommand)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrClaudeNotFound, err)
	}
	return path, nil
}

// quarryNotOnPath reports whether a LookPath is set and finds no quarry.
func (s *Server) quarryNotOnPath() bool {
	if s.lookPath == nil {
		return false
	}
	_, err := s.lookPath(quarryCommand)
	return err != nil
}
