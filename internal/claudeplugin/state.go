package claudeplugin

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
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

// ExitError reports a claude child that ran and exited non-zero.
type ExitError struct {
	Argv   string // the command as run, e.g. "claude plugin list --json"
	Status int    // the exit status
	Output []byte // stdout and stderr combined, in arrival order
}

func (e *ExitError) Error() string {
	return e.Argv + " exited with status " + strconv.Itoa(e.Status)
}

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
	marketplaceOurs bool // our marketplace is present
	userCopy        bool // quarry@quarry is installed at user scope
	userCopyOff     bool // that copy is explicitly turned off
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
		if p["id"] == pluginID && p["scope"] == userScope {
			st.userCopy = true
			// Only an explicit false turns the copy off; a missing or odd value counts as on.
			st.userCopyOff = p["enabled"] == false
		}
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

// run runs one child of the claude command and returns its combined output. The
// Runner's own error comes back as is; a non-zero exit comes back as *ExitError.
func (s *Server) run(ctx context.Context, claude, args string) ([]byte, error) {
	out, status, err := s.runner(ctx, claude, strings.Fields(args)...)
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, &ExitError{Argv: claudeCommand + " " + args, Status: status, Output: out}
	}
	return out, nil
}
