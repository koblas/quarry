package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// syncStatus returns the status document from one read of the store and one load of the config,
// both made for this call. A config that cannot be read never refuses: the ignore list is
// dropped and a warning says so, as in quarry status --json.
func (s *Server) syncStatus(ctx context.Context, _ noInput) (any, error) {
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	st, err := srv.Status(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // a RefusalError is the tool's answer, sent verbatim
	}
	// The config is read after the store, so a refusing store never depends on it.
	ignore, warnings := s.statusIgnore()
	findings := document.FindingsTally{Counts: report.CountFindings(st, ignore), IgnoreKnown: len(warnings) == 0}
	return document.NewStatus(st, findings, warnings), nil
}

// statusIgnore is findings.ignore and the warnings to send with it, with absolute paths.
func (s *Server) statusIgnore() ([]string, []string) {
	cfg, err := s.newConfig(commandName)
	if err != nil {
		return document.StatusIgnore(nil, config.ProblemAbsolute(err))
	}
	return document.StatusIgnore(cfg.Ignore, "")
}
