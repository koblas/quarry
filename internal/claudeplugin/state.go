package claudeplugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/koblas/quarry/internal/platform/toolrun"
)

// The commands Install looks up.
const (
	claudeCommand = "claude"
	quarryCommand = "quarry"
)

// The list commands and quarry's own identity; arguments are space-separated.
const (
	marketplaceListArgs = "plugin marketplace list --json"
	pluginListArgs      = "plugin list --json"

	marketplaceName = "quarry"
	marketplaceRepo = "koblas/quarry"
	pluginID        = "quarry@quarry"
	userScope       = "user"
)

// ErrClaudeNotFound reports that the claude command cannot be found; no step runs.
var ErrClaudeNotFound = errors.New("claude is not on your PATH")

// ErrForeignMarketplace reports a Claude Code marketplace named "quarry" that
// is not quarry's own; no step runs when it is found.
var ErrForeignMarketplace = errors.New("a foreign marketplace named quarry exists")

// ExitError reports a claude child that did not exit zero: a non-zero status, or a
// signal quarry did not send.
type ExitError struct {
	Argv   string    // the command as run, e.g. "claude plugin list --json"
	Status int       // the exit status; unset when Signal is
	Signal os.Signal // the signal that ended the child; nil when it exited on its own
	Output []byte    // stdout and stderr combined, in arrival order
}

func (e *ExitError) Error() string {
	if e.Signal != nil {
		return e.Argv + " was stopped by signal " + e.Signal.String()
	}
	return e.Argv + " exited with status " + strconv.Itoa(e.Status)
}

// InterruptedError reports that the context ended before Argv, the running or next-due
// claude command, finished; it unwraps to the context's error.
type InterruptedError struct {
	Argv  string // the command that was running or due next
	cause error
}

func (e *InterruptedError) Error() string {
	return "stopped before " + e.Argv + " finished"
}

func (e *InterruptedError) Unwrap() error { return e.cause }

// ListUnreadableError reports a list command that exited zero but printed
// something quarry cannot classify: not a JSON array of objects, or an entry
// missing a field quarry needs.
type ListUnreadableError struct {
	Argv string // the list command, e.g. "claude plugin list --json"
}

func (e *ListUnreadableError) Error() string {
	return "cannot read what " + e.Argv + " printed"
}

// state is what claude's two lists say about quarry.
type state struct {
	marketplaceOurs bool   // our marketplace is present
	userCopy        bool   // quarry@quarry is installed at user scope
	userCopyOff     bool   // that copy is explicitly turned off
	others          []Copy // quarry@quarry installs at any other scope, in list order
}

// readState runs the marketplace list, then the plugin list, and classifies them.
// A marketplace-list failure skips the plugin list; a foreign marketplace is reported last.
func (s *Server) readState(ctx context.Context, claude string) (state, error) {
	marketplaces, err := s.list(ctx, claude, marketplaceListArgs, "name")
	if err != nil {
		return state{}, err
	}
	plugins, err := s.list(ctx, claude, pluginListArgs, "id", "scope")
	if err != nil {
		return state{}, err
	}

	var st state
	foreign := false
	for _, m := range marketplaces {
		if m["name"] != marketplaceName {
			continue
		}
		if m["source"] == "github" && m["repo"] == marketplaceRepo {
			st.marketplaceOurs = true
		} else {
			foreign = true
		}
	}
	if foreign {
		return state{}, ErrForeignMarketplace
	}
	for _, p := range plugins {
		if p["id"] != pluginID {
			continue
		}
		if p["scope"] != userScope {
			// scope is a string: list requires it.
			scope, _ := p["scope"].(string)
			path, _ := p["projectPath"].(string)
			st.others = append(st.others, Copy{Scope: scope, ProjectPath: path})
			continue
		}
		st.userCopy = true
		// Only an explicit false turns the copy off; a missing or odd value counts as on.
		st.userCopyOff = p["enabled"] == false
	}
	return st, nil
}

// list runs one list command and decodes its JSON array of objects, requiring
// each entry to carry every key in required as a string.
func (s *Server) list(ctx context.Context, claude, args string, required ...string) ([]map[string]any, error) {
	out, err := s.run(ctx, claude, args)
	if err != nil {
		return nil, err
	}
	unreadable := &ListUnreadableError{Argv: claudeCommand + " " + args}

	var raw any
	if json.Unmarshal(out, &raw) != nil {
		return nil, unreadable
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, unreadable
	}
	entries := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, unreadable
		}
		for _, key := range required {
			if _, ok := entry[key].(string); !ok {
				return nil, unreadable
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// run runs one child of claude and returns its combined output: a done ctx is
// *InterruptedError, a non-zero exit or signal is *ExitError, any other error is unchanged.
func (s *Server) run(ctx context.Context, claude, args string) ([]byte, error) {
	argv := claudeCommand + " " + args
	if ctx.Err() != nil {
		return nil, &InterruptedError{Argv: argv, cause: ctx.Err()}
	}
	out, status, err := s.runner(ctx, claude, strings.Fields(args)...)
	if err != nil {
		// A cancelled child dies by SIGKILL, so ctx is decided before the signal.
		if ctx.Err() != nil {
			return nil, &InterruptedError{Argv: argv, cause: ctx.Err()}
		}
		if sig, ok := errors.AsType[*toolrun.SignalError](err); ok {
			return nil, &ExitError{Argv: argv, Signal: sig.Signal, Output: out}
		}
		return nil, err
	}
	if status != 0 {
		return nil, &ExitError{Argv: argv, Status: status, Output: out}
	}
	return out, nil
}
