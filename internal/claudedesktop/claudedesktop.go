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

	"github.com/koblas/quarry/internal/platform/replacefile"
)

// configName is Claude Desktop's config file inside its folder.
const configName = "claude_desktop_config.json"

// configMode is the mode of a config file quarry creates.
const configMode fs.FileMode = 0o600

// ErrNoHome is returned by Install when the Server has no home directory to look in.
var ErrNoHome = errors.New("home directory is not set")

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

// Result reports what an Install did.
type Result struct {
	Folder  string // the Claude Desktop folder
	Config  string // the config file inside it
	Skipped bool   // the folder does not exist, so nothing was done
	Command string // the quarry path the entry starts
}

// Install adds the quarry entry to Claude Desktop's config file, creating the file.
// It returns a Skipped Result when Desktop's folder does not exist, ErrNoHome when the
// Server has no home, and an error wrapping fs.ErrExist when anything is already at the
// config path. Any other failure leaves the folder as it was.
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
	// Lstat, not Stat: a dangling symlink looks missing to Stat, and the rename would replace it.
	if _, err := os.Lstat(res.Config); err == nil {
		return res, fmt.Errorf("%s already exists: %w", res.Config, fs.ErrExist)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return res, fmt.Errorf("check %s: %w", res.Config, err)
	}
	doc, err := encode(command)
	if err != nil {
		return res, err // unreachable: encode fails only when json cannot encode a value, and every value is a string or string slice built here
	}
	if err := replacefile.Write(res.Config, doc, configMode); err != nil {
		return res, fmt.Errorf("write %s: %w", res.Config, err)
	}
	res.Command = command
	return res, nil
}

// entry is the value of mcpServers.quarry: how Desktop starts the quarry MCP server.
type entry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// encode returns a config document holding only the quarry entry, indented by two spaces
// with a trailing newline and no HTML escaping, so "<>&" in the path stays literal.
func encode(command string) ([]byte, error) {
	doc := map[string]map[string]entry{
		"mcpServers": {"quarry": {Command: command, Args: []string{"mcp"}}},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode the config: %w", err) // unreachable: json fails only on unsupported types or marshalers, and doc holds only strings
	}
	return buf.Bytes(), nil
}
