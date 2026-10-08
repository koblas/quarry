package claudedesktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/koblas/quarry/internal/platform/replacefile"
)

// configName is Claude Desktop's config file inside its folder.
const configName = "claude_desktop_config.json"

// configMode is the mode of a config file quarry creates.
const configMode fs.FileMode = 0o600

// backupSuffix names the backup of a config next to it, and backupMode is the backup's mode.
const (
	backupSuffix = ".before-quarry"
	backupMode   = fs.FileMode(0o600)
)

// ErrNoHome is returned by Install when the Server has no home directory to look in.
var ErrNoHome = errors.New("home directory is not set")

// errNotObject is why a value cannot be merged into: it decoded, but not to a JSON object.
var errNotObject = errors.New("not a JSON object")

// errForeignEntry is why merge refused mcpServers.quarry: it is not an entry that starts `quarry mcp`.
var errForeignEntry = errors.New("mcpServers.quarry does not start quarry mcp")

// ForeignEntryError is returned by Install when mcpServers.quarry is not an entry that starts
// `quarry mcp`; the config is untouched.
type ForeignEntryError struct {
	Path string // the config file
}

func (e *ForeignEntryError) Error() string {
	return e.Path + ": mcpServers.quarry is not an entry that starts quarry mcp"
}

// BackupError is returned by Install when the backup of the existing config cannot be saved;
// the config is untouched.
type BackupError struct {
	Path   string // the backup file
	Config string // the config it backs up
	Err    error
}

func (e *BackupError) Error() string { return fmt.Sprintf("save %s: %v", e.Path, e.Err) }
func (e *BackupError) Unwrap() error { return e.Err }

// WriteError is returned by Install when the config cannot be written; the backup written just
// before stays.
type WriteError struct {
	Path string // the config file
	Err  error
}

func (e *WriteError) Error() string { return fmt.Sprintf("write %s: %v", e.Path, e.Err) }
func (e *WriteError) Unwrap() error { return e.Err }

// Executable reports the absolute path of the running quarry binary, as os.Executable does.
type Executable func() (string, error)

// Server installs quarry's MCP server into Claude Desktop's config file.
type Server struct {
	home       string
	executable Executable
}

// Option configures a Server.
type Option func(*Server)

// WithHome sets the user's home directory, under which Claude Desktop keeps its config.
func WithHome(home string) Option {
	return func(s *Server) { s.home = home }
}

// WithExecutable sets how Install learns the path of the running quarry binary.
// A Server needs one before Install finds a Desktop folder.
func WithExecutable(e Executable) Option {
	return func(s *Server) { s.executable = e }
}

// NewServer returns a Server configured by opts.
func NewServer(opts ...Option) *Server {
	s := &Server{}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Outcome is what an Install did to a Desktop config that exists.
type Outcome int

// The outcomes of an Install; Added is the zero value.
const (
	Added     Outcome = iota // no quarry entry was there; one was added
	Updated                  // an entry of ours started another path; its command was replaced
	Unchanged                // an entry of ours already started this path; nothing was written
)

// Result reports what an Install did.
type Result struct {
	Folder   string  // the Claude Desktop folder
	Config   string  // the config file inside it
	Skipped  bool    // the folder does not exist, so nothing was done
	Outcome  Outcome // what happened to the entry
	Command  string  // the quarry path the entry starts
	Previous string  // the command an Updated entry started before
}

// Install adds the quarry entry to Claude Desktop's config, creating the file or merging into
// it after saving a backup beside it; an entry of ours is repointed, or left alone when it
// already starts this binary. It refuses a non-regular path (fs.ErrExist) and a quarry key that
// is not ours (*ForeignEntryError), and names a failed backup or write as *BackupError or *WriteError.
func (s *Server) Install(_ context.Context) (Result, error) {
	if s.home == "" {
		return Result{}, ErrNoHome
	}
	folder := filepath.Join(s.home, "Library", "Application Support", "Claude")
	res := Result{Folder: folder, Config: filepath.Join(folder, configName)}
	if _, err := os.Stat(folder); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			res.Skipped = true
			return res, nil
		}
		return res, fmt.Errorf("check %s: %w", folder, err)
	}
	command, err := s.executable()
	if err != nil {
		return res, fmt.Errorf("find the quarry binary: %w", err)
	}
	current, err := readConfig(res.Config)
	if err != nil {
		return res, err
	}
	plan, err := merge(current.data, command)
	if errors.Is(err, errForeignEntry) {
		return res, &ForeignEntryError{Path: res.Config}
	}
	if err != nil {
		return res, fmt.Errorf("merge into %s: %w", res.Config, err)
	}
	if plan.outcome == Unchanged {
		res.Outcome, res.Command = Unchanged, command
		return res, nil
	}
	// The backup goes first: a config that could not be saved must not be replaced.
	if current.present {
		backup := res.Config + backupSuffix
		if err := replacefile.Write(backup, current.data, backupMode); err != nil {
			return res, &BackupError{Path: backup, Config: res.Config, Err: err}
		}
	}
	if err := replacefile.Write(res.Config, plan.doc, current.mode); err != nil {
		return res, &WriteError{Path: res.Config, Err: err}
	}
	res.Outcome, res.Command, res.Previous = plan.outcome, command, plan.previous
	return res, nil
}

// existing is what Install found at the config path: whether a config was there, the bytes to
// merge into and the mode the written config takes.
type existing struct {
	present bool
	data    []byte
	mode    fs.FileMode
}

// readConfig reads the config at path. A missing config yields no data and configMode; a
// path that is not a regular file is refused with an error wrapping fs.ErrExist.
func readConfig(path string) (existing, error) {
	// Lstat, not Stat: a dangling symlink looks missing to Stat, and the rename would replace it.
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return existing{mode: configMode}, nil
	}
	if err != nil {
		return existing{}, fmt.Errorf("check %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return existing{}, fmt.Errorf("%s already exists: %w", path, fs.ErrExist)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return existing{}, fmt.Errorf("read %s: %w", path, err)
	}
	return existing{present: true, data: data, mode: info.Mode().Perm()}, nil
}

// serversKey and entryKey are the config keys the quarry entry lives under.
const (
	serversKey = "mcpServers"
	entryKey   = "quarry"
)

// quarryName is the final path element of a command that runs quarry.
const quarryName = "quarry"

// entry is the value of mcpServers.quarry: how Desktop starts the quarry MCP server.
type entry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// merged is what merge decided: the config to write (nil when Unchanged), the outcome, and for
// Updated the command the entry started before.
type merged struct {
	doc      []byte
	outcome  Outcome
	previous string
}

// merge returns original (empty meaning "{}") with the quarry entry set under mcpServers.
// It refuses a quarry key that is not ours (errForeignEntry); an entry of ours keeps all but its command.
func merge(original []byte, command string) (merged, error) {
	top, err := decodeObject(original)
	if err != nil {
		return merged{}, fmt.Errorf("config: %w", err)
	}
	var servers map[string]json.RawMessage
	if raw, ok := top[serversKey]; ok && string(raw) != "null" {
		if servers, err = decodeObject(raw); err != nil {
			return merged{}, fmt.Errorf("%s: %w", serversKey, err)
		}
	}
	var value any = entry{Command: command, Args: []string{"mcp"}}
	out := merged{outcome: Added}
	if raw, ok := servers[entryKey]; ok {
		fields, started, ours := parseOurs(raw)
		if !ours {
			return merged{}, errForeignEntry
		}
		if started == command {
			return merged{outcome: Unchanged}, nil
		}
		repointed := make(map[string]any, len(fields))
		for k, v := range fields {
			repointed[k] = v
		}
		repointed["command"] = command
		value, out = repointed, merged{outcome: Updated, previous: started}
	}
	out.doc, err = encodeConfig(top, servers, value)
	return out, err
}

// encodeConfig renders top with value as mcpServers.quarry beside the other servers.
func encodeConfig(top, servers map[string]json.RawMessage, value any) ([]byte, error) {
	doc := make(map[string]any, len(top)+1)
	for k, v := range top {
		doc[k] = v
	}
	all := make(map[string]any, len(servers)+1)
	for k, v := range servers {
		all[k] = v
	}
	all[entryKey] = value
	doc[serversKey] = all

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode the config: %w", err) // unreachable: every value is a RawMessage from a successful decode, or a string or string slice built here
	}
	return buf.Bytes(), nil
}

// decodeObject decodes data as one JSON object, reading empty or whitespace-only data as
// "{}". A JSON null decodes without error but is not an object, so it returns errNotObject.
func decodeObject(data []byte) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if obj == nil {
		return nil, errNotObject
	}
	return obj, nil
}

// parseOurs reports whether raw is an entry that starts `quarry mcp`, returning its fields and
// command. Names are compared as text, never resolved on disk.
func parseOurs(raw json.RawMessage) (map[string]json.RawMessage, string, bool) {
	fields, err := decodeObject(raw)
	if err != nil {
		return nil, "", false
	}
	var command string
	if json.Unmarshal(fields["command"], &command) != nil || command[strings.LastIndex(command, "/")+1:] != quarryName {
		return nil, "", false
	}
	var args []string
	if json.Unmarshal(fields["args"], &args) != nil || len(args) != 1 || args[0] != "mcp" {
		return nil, "", false
	}
	return fields, command, true
}
