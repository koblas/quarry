package claudeplugin

import (
	"context"
	"fmt"
)

// Runner runs the command name with args and returns its stdout, its stdout and stderr
// combined, and its exit status. err is non-nil only when the command could not start,
// a signal ended it, or ctx ended; a non-zero exit status comes back with a nil err.
type Runner func(ctx context.Context, name string, args ...string) (stdout, combined []byte, exitCode int, err error)

// LookPath resolves the command file to the path that would run, as exec.LookPath does.
type LookPath func(file string) (string, error)

const (
	addMarketplaceArgs = "plugin marketplace add --scope user " + marketplaceRepo
	installPluginArgs  = "plugin install --scope user " + pluginID

	uninstallPluginArgs   = "plugin uninstall --scope user " + pluginID
	removeMarketplaceArgs = "plugin marketplace remove --scope user " + marketplaceName
)

// Server installs and uninstalls quarry's Claude Code plugin through a Runner. Install and
// Uninstall fail with ErrClaudeNotFound, ErrForeignMarketplace, *ListUnreadableError,
// *ExitError, *InterruptedError, or the Runner's own.
type Server struct {
	runner   Runner
	lookPath LookPath
}

// Option configures a Server.
type Option func(*Server)

// WithRunner sets how claude children run. A Server needs one before Install or Uninstall.
func WithRunner(r Runner) Option {
	return func(s *Server) { s.runner = r }
}

// WithLookPath sets how Install and Uninstall find the claude command, and Install the
// quarry command. Without it, both run "claude" as named and Install does not look for quarry.
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

// Copy is a quarry plugin install at a scope other than user.
type Copy struct {
	Scope       string // the scope claude reported, verbatim
	ProjectPath string // the project it belongs to; empty when claude gave none
}

// UninstallResult reports what an Uninstall did.
type UninstallResult struct {
	PluginUninstalled  bool   // the plugin uninstall step ran
	MarketplaceRemoved bool   // the marketplace remove step ran
	MarketplaceKept    bool   // our marketplace was left because a non-user copy remains
	Remaining          []Copy // every non-user copy the list showed; nil when none
}

// Ran reports whether any claude step changed something.
func (r UninstallResult) Ran() bool {
	return r.PluginUninstalled || r.MarketplaceRemoved
}

// Uninstall removes quarry's user-scope plugin, then its marketplace unless a copy at another
// scope remains, skipping each step the lists show absent; it returns the steps done even on failure.
func (s *Server) Uninstall(ctx context.Context) (UninstallResult, error) {
	claude, err := s.findClaude()
	if err != nil {
		return UninstallResult{}, err
	}
	st, err := s.readState(ctx, claude)
	if err != nil {
		return UninstallResult{}, err
	}

	var res UninstallResult
	if st.userCopy {
		if _, err := s.run(ctx, claude, uninstallPluginArgs); err != nil {
			return res, err
		}
		res.PluginUninstalled = true
	}
	res.Remaining = st.others
	if !st.marketplaceOurs {
		return res, nil
	}
	if len(st.others) > 0 {
		res.MarketplaceKept = true
		return res, nil
	}
	if _, err := s.run(ctx, claude, removeMarketplaceArgs); err != nil {
		return res, err
	}
	res.MarketplaceRemoved = true
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
