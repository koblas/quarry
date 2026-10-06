package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// syncStatus returns the status document from one read of the store and one load of the config.
// A config that cannot be read never refuses: the ignore list and classification are dropped and a warning says so.
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
	ignore, classification, warnings := s.statusChoices()
	findings := document.FindingsTally{Counts: report.CountFindings(st, ignore, classification), IgnoreKnown: len(warnings) == 0}
	return document.NewStatus(st, findings, warnings), nil
}

// statusChoices is findings.ignore, the account classification and the warnings to send with them, with absolute paths.
func (s *Server) statusChoices() ([]string, report.Classification, []string) {
	cfg, err := s.newConfig(commandName)
	if err != nil {
		return nil, report.Classification{}, []string{document.CannotTellChoices(config.ProblemAbsolute(err))}
	}
	return cfg.Ignore, classificationOf(cfg), nil
}
