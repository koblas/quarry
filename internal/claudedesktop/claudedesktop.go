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

// notObjectError is why a value cannot be merged into: it is valid JSON, but of another kind.
type notObjectError struct{ kind string }

func (e *notObjectError) Error() string { return "a JSON " + e.kind + ", not an object" }

// ForeignEntryError is returned by Install when mcpServers.quarry is not an entry that starts
// `quarry mcp`; the config is untouched.
type ForeignEntryError struct {
	Path string // the config file
}

func (e *ForeignEntryError) Error() string {
	return e.Path + ": mcpServers.quarry is not an entry that starts quarry mcp"
}

// InvalidJSONError is returned by Install when the config is not valid JSON; the config is untouched.
type InvalidJSONError struct {
	Path string // the config file
	Err  error  // the encoding/json error, as decoded
}

func (e *InvalidJSONError) Error() string { return fmt.Sprintf("%s: invalid JSON: %v", e.Path, e.Err) }
func (e *InvalidJSONError) Unwrap() error { return e.Err }

// TopLevelError is returned by Install when the config is valid JSON but not an object; the config
// is untouched.
type TopLevelError struct {
	Path string // the config file
	Kind string // null, array, string, number or boolean
}

func (e *TopLevelError) Error() string {
	return fmt.Sprintf("%s: the config is a JSON %s, not an object", e.Path, e.Kind)
}

// ServersError is returned by Install when mcpServers is neither an object nor null; the config
// is untouched.
type ServersError struct {
	Path string // the config file
	Kind string // array, string, number or boolean
}

func (e *ServersError) Error() string {
	return fmt.Sprintf("%s: mcpServers is a JSON %s, not an object", e.Path, e.Kind)
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

// SymlinkError is returned by Install when the config path is a symbolic link; nothing is written.
type SymlinkError struct {
	Path    string // the config path
	Command string // the quarry path Install would have written
}

func (e *SymlinkError) Error() string { return e.Path + " is a symbolic link" }

// NotAFileError is returned by Install when the config path exists but is neither a regular file
// nor a symbolic link; nothing is written.
type NotAFileError struct {
	Path string // the config path
}

func (e *NotAFileError) Error() string { return e.Path + " is not a regular file" }

// ReadError is returned by Install when the config path cannot be checked or read; nothing is written.
type ReadError struct {
	Path string // the config path
	Err  error
}

func (e *ReadError) Error() string { return fmt.Sprintf("read %s: %v", e.Path, e.Err) }
func (e *ReadError) Unwrap() error { return e.Err }

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
// already starts this binary. It refuses a symlink (*SymlinkError), any other non-regular path
// (*NotAFileError) and a quarry key that is not ours (*ForeignEntryError), and names a config it
// cannot check or read, a failed backup and a failed write as *ReadError, *BackupError and *WriteError.
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
	if symlink, ok := errors.AsType[*SymlinkError](err); ok {
		symlink.Command = command
	}
	if err != nil {
		return res, err
	}
	plan, err := merge(res.Config, current.data, command)
	if err != nil {
		return res, err
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
// symlink is refused with *SymlinkError, any other non-regular path with *NotAFileError, and a
// path it cannot check or read with *ReadError.
func readConfig(path string) (existing, error) {
	// Lstat, not Stat: a dangling symlink looks missing to Stat, and the rename would replace it.
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return existing{mode: configMode}, nil
	}
	if err != nil {
		return existing{}, &ReadError{Path: path, Err: err}
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return existing{}, &SymlinkError{Path: path}
	}
	if !info.Mode().IsRegular() {
		return existing{}, &NotAFileError{Path: path}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return existing{}, &ReadError{Path: path, Err: err}
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
// It refuses a config that is not an object it can merge into and a quarry key that is not ours,
// with the typed errors Install documents (path names the config in them); an entry of ours keeps
// all but its command.
func merge(path string, original []byte, command string) (merged, error) {
	top, err := decodeObject(original)
	if notObject, ok := errors.AsType[*notObjectError](err); ok {
		return merged{}, &TopLevelError{Path: path, Kind: notObject.kind}
	}
	if err != nil {
		return merged{}, &InvalidJSONError{Path: path, Err: err}
	}
	var servers map[string]json.RawMessage
	if raw, ok := top[serversKey]; ok && string(raw) != "null" {
		// top decoded, so raw is valid JSON and a failure here can only be the wrong kind.
		if servers, err = decodeObject(raw); err != nil {
			return merged{}, &ServersError{Path: path, Kind: kindOf(raw)}
		}
	}
	var value any = entry{Command: command, Args: []string{"mcp"}}
	out := merged{outcome: Added}
	if raw, ok := servers[entryKey]; ok {
		fields, started, ours := parseOurs(raw)
		if !ours {
			return merged{}, &ForeignEntryError{Path: path}
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

// decodeObject decodes data as one JSON object, reading empty or whitespace-only data as "{}".
// Valid JSON of another kind returns *notObjectError, a JSON null among them (it decodes to a nil map).
func decodeObject(data []byte) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var obj map[string]json.RawMessage
	err := json.Unmarshal(data, &obj)
	if _, wrongKind := errors.AsType[*json.UnmarshalTypeError](err); wrongKind || (err == nil && obj == nil) {
		return nil, &notObjectError{kind: kindOf(data)}
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // InvalidJSONError carries the encoding/json error as decoded
	}
	return obj, nil
}

// kindOf names the JSON kind of valid, non-object data from its first byte.
func kindOf(data []byte) string {
	switch bytes.TrimSpace(data)[0] {
	case 'n':
		return "null"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	default:
		return "number"
	}
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
