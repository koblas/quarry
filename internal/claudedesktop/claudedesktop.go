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

// ErrInterrupted is returned, wrapping the context's error, when the context ended before a write.
var ErrInterrupted = errors.New("interrupted before the config was changed")

// stopped returns ErrInterrupted wrapping ctx's error once ctx has ended, else nil.
func stopped(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrInterrupted, err)
	}
	return nil
}

// notObjectError is why a value cannot be merged into: it is valid JSON, but of another kind.
type notObjectError struct{ kind string }

// unreachable: merge turns it into *TopLevelError or *ServersError and parseOurs drops it, so nothing formats it
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

// FolderError is returned by Install and Uninstall when Claude Desktop's folder cannot be checked
// for any reason other than being absent; nothing is read or written.
type FolderError struct {
	Path string // the Claude Desktop folder
	Err  error
}

func (e *FolderError) Error() string { return fmt.Sprintf("check %s: %v", e.Path, e.Err) }
func (e *FolderError) Unwrap() error { return e.Err }

// Executable reports the absolute path of the running quarry binary, as os.Executable does.
type Executable func() (string, error)

// LookPath reports the absolute path a shell would run for the named command, as exec.LookPath does.
type LookPath func(file string) (string, error)

// TempBuildError is returned by Install when the quarry binary lives under the temporary
// directory, where it will not outlast the session; nothing is written.
type TempBuildError struct {
	Path string // the path under the temporary directory
}

func (e *TempBuildError) Error() string { return e.Path + " is a temporary build" }

// BinaryNameError is returned by Install when the quarry path it would write does not end in an
// element named exactly quarry, which is the only name an entry of ours can have; nothing is read or written.
type BinaryNameError struct {
	Base string // the chosen path's last element
}

func (e *BinaryNameError) Error() string { return fmt.Sprintf("the quarry binary is named %q", e.Base) }

// ExecutableError is returned by Install when the path of the running quarry binary cannot be
// learned; nothing is written.
type ExecutableError struct {
	Config string // the config file Install would have edited
	Err    error
}

func (e *ExecutableError) Error() string { return "find the quarry binary: " + e.Err.Error() }
func (e *ExecutableError) Unwrap() error { return e.Err }

// Server installs quarry's MCP server into Claude Desktop's config file and removes it again.
type Server struct {
	home       string
	executable Executable
	lookPath   LookPath
	tempDir    string
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

// WithLookPath sets how Install finds the quarry a shell would run.
func WithLookPath(l LookPath) Option {
	return func(s *Server) { s.lookPath = l }
}

// WithTempDir sets the temporary directory root a quarry binary must not live under; an empty
// root turns that check off. NewServer defaults it to os.TempDir().
func WithTempDir(root string) Option {
	return func(s *Server) { s.tempDir = root }
}

// NewServer returns a Server configured by opts.
func NewServer(opts ...Option) *Server {
	s := &Server{tempDir: os.TempDir()}
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
	Skipped  bool    // there is no Claude Desktop folder, so nothing was done
	Outcome  Outcome // what happened to the entry
	Command  string  // the quarry path the entry starts
	Previous string  // the command an Updated entry started before

	NotAFolder bool // Skipped because the path exists but is not a folder; false when it is missing

	PathQuarry string // the quarry a shell runs, when it is not the file Command names
}

// Install adds the quarry entry to Claude Desktop's config, backing up an existing file first, or
// repoints or leaves alone an entry of ours. A Desktop folder that is missing or not a folder is
// skipped (Result.Skipped); anything it cannot safely read or write is refused with a typed error.
// Result.PathQuarry names a different quarry on PATH. A context that ended before the write is
// ErrInterrupted, and nothing is written.
func (s *Server) Install(ctx context.Context) (Result, error) {
	loc, err := s.locate()
	res := Result{Folder: loc.folder, Config: loc.config}
	if err != nil {
		return res, err
	}
	if !loc.found {
		res.Skipped, res.NotAFolder = true, loc.notAFolder
		return res, nil
	}
	exe, err := s.executable()
	if err != nil {
		return res, &ExecutableError{Config: res.Config, Err: err}
	}
	command, pathQuarry := s.choosePath(exe)
	for _, p := range []string{exe, command} {
		if s.isTemporary(p) {
			return res, &TempBuildError{Path: p}
		}
	}
	if base := lastElement(command); base != quarryName {
		return res, &BinaryNameError{Base: base}
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
		res.Outcome, res.Command, res.PathQuarry = Unchanged, command, pathQuarry
		return res, nil
	}
	if err := stopped(ctx); err != nil {
		return res, err
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
	res.Outcome, res.Command, res.Previous, res.PathQuarry = plan.outcome, command, plan.previous, pathQuarry
	return res, nil
}

// location is where Claude Desktop keeps its config and what is at its folder.
type location struct {
	folder, config string
	found          bool // the folder exists and is a folder
	notAFolder     bool // something other than a folder is there, as opposed to nothing
}

// locate finds Claude Desktop's folder: found, missing, or not a folder (notAFolder); any other
// stat failure is *FolderError.
func (s *Server) locate() (location, error) {
	if s.home == "" {
		return location{}, ErrNoHome
	}
	folder := filepath.Join(s.home, "Library", "Application Support", "Claude")
	loc := location{folder: folder, config: filepath.Join(folder, configName)}
	info, err := os.Stat(folder)
	if errors.Is(err, fs.ErrNotExist) {
		return loc, nil
	}
	if err != nil {
		return loc, &FolderError{Path: folder, Err: err}
	}
	loc.found, loc.notAFolder = info.IsDir(), !info.IsDir()
	return loc, nil
}

// UninstallResult reports what an Uninstall did.
type UninstallResult struct {
	Folder  string // the Claude Desktop folder
	Config  string // the config file inside it
	Skipped bool   // there is no Claude Desktop folder, so nothing was done
	Removed bool   // the quarry entry was removed

	NotAFolder bool // Skipped because the path exists but is not a folder; false when it is missing
}

// Uninstall removes the quarry entry from Claude Desktop's config, backing up the file first, but
// only an entry that starts `quarry mcp` (*ForeignEntryError otherwise). A config that cannot hold
// our entry is left alone, and a Desktop folder that is missing or not a folder is skipped
// (UninstallResult.Skipped). A context that ended before the removal is ErrInterrupted, and nothing
// is written.
func (s *Server) Uninstall(ctx context.Context) (UninstallResult, error) {
	loc, err := s.locate()
	config := loc.config
	res := UninstallResult{Folder: loc.folder, Config: config}
	if err != nil {
		return res, err
	}
	if !loc.found {
		res.Skipped, res.NotAFolder = true, loc.notAFolder
		return res, nil
	}
	current, err := readConfig(config)
	if _, notAFile := errors.AsType[*NotAFileError](err); notAFile {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	top, err := decodeObject(current.data)
	if _, wrongKind := errors.AsType[*notObjectError](err); wrongKind {
		return res, nil
	}
	if err != nil {
		return res, &InvalidJSONError{Path: config, Err: err}
	}
	// A failure here is only the wrong kind or null (top decoded): nothing of ours in it.
	servers, _ := decodeObject(top[serversKey])
	raw, ok := servers[entryKey]
	if !ok {
		return res, nil
	}
	if _, _, ours := parseOurs(raw); !ours {
		return res, &ForeignEntryError{Path: config}
	}
	doc, err := encodeConfig(top, serverValues(servers))
	if err != nil {
		return res, err // unreachable: encodeConfig only fails on a value json cannot encode, and every value is a RawMessage from a successful decode
	}
	if err := stopped(ctx); err != nil {
		return res, err
	}
	// The backup goes first: a config that could not be saved must not be replaced.
	backup := config + backupSuffix
	if err := replacefile.Write(backup, current.data, backupMode); err != nil {
		return res, &BackupError{Path: backup, Config: config, Err: err}
	}
	if err := replacefile.Write(config, doc, current.mode); err != nil {
		return res, &WriteError{Path: config, Err: err}
	}
	res.Removed = true
	return res, nil
}

// choosePath returns the path to write — the PATH quarry when it is the running binary, else the
// running binary — and the PATH quarry when it is a different, checkable file.
func (s *Server) choosePath(exe string) (string, string) {
	if s.lookPath == nil {
		return exe, ""
	}
	onPath, err := s.lookPath(quarryName)
	if err != nil || !filepath.IsAbs(onPath) {
		return exe, ""
	}
	pathInfo, err := os.Stat(onPath)
	if err != nil {
		return exe, ""
	}
	// Stat, not Lstat: a link and its target must compare equal.
	if exeInfo, err := os.Stat(exe); err == nil && os.SameFile(pathInfo, exeInfo) {
		return onPath, ""
	}
	return exe, onPath
}

// goBuildPrefix starts the name of the directory `go run` builds into.
const goBuildPrefix = "go-build"

// isTemporary reports whether path lies under the temporary directory root or inside a go-build
// directory. The test is lexical: links are not resolved.
func (s *Server) isTemporary(path string) bool {
	for element := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(element, goBuildPrefix) {
			return true
		}
	}
	if s.tempDir == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(s.tempDir), path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// existing is what Install found at the config path: whether a config was there, the bytes to
// merge into and the mode the written config takes.
type existing struct {
	present bool
	data    []byte
	mode    fs.FileMode
}

// readConfig reads the config at path; a missing one yields no data and configMode. A path it
// cannot take as a regular file is refused with *SymlinkError, *NotAFileError or *ReadError.
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

// merge returns original (empty meaning "{}") with the quarry entry set under mcpServers, or the
// typed refusal Install documents; an entry of ours keeps all but its command. path names the config.
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
	all := serverValues(servers)
	all[entryKey] = value
	out.doc, err = encodeConfig(top, all)
	return out, err
}

// serverValues copies servers without its quarry entry, as values encodeConfig can render.
func serverValues(servers map[string]json.RawMessage) map[string]any {
	all := make(map[string]any, len(servers)+1)
	for k, v := range servers {
		if k != entryKey {
			all[k] = v
		}
	}
	return all
}

// encodeConfig renders top with servers as its mcpServers object.
func encodeConfig(top map[string]json.RawMessage, servers map[string]any) ([]byte, error) {
	doc := make(map[string]any, len(top)+1)
	for k, v := range top {
		doc[k] = v
	}
	doc[serversKey] = servers

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

// lastElement is the text after the last slash of path; unlike filepath.Base it keeps a trailing
// slash as an empty element, so a directory is never taken for a binary.
func lastElement(path string) string { return path[strings.LastIndex(path, "/")+1:] }

// parseOurs reports whether raw is an entry that starts `quarry mcp`, returning its fields and
// command. Names are compared as text, never resolved on disk.
func parseOurs(raw json.RawMessage) (map[string]json.RawMessage, string, bool) {
	fields, err := decodeObject(raw)
	if err != nil {
		return nil, "", false
	}
	var command string
	if json.Unmarshal(fields["command"], &command) != nil || lastElement(command) != quarryName {
		return nil, "", false
	}
	var args []string
	if json.Unmarshal(fields["args"], &args) != nil || len(args) != 1 || args[0] != "mcp" {
		return nil, "", false
	}
	return fields, command, true
}
