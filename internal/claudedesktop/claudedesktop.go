package claudedesktop

import "context"

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

// Install adds the quarry entry to Claude Desktop's config file.
func (s *Server) Install(ctx context.Context) (Result, error) {
	return Result{}, nil
}
