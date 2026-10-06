package mcp

import (
	"context"
	"fmt"
	"slices"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// acbTwin is the quarry command whose output the acb tool matches.
const acbTwin = "acb"

// acb is the acb tool: acb --json's document for the call's year and securities. Its warnings read the whole
// report, so a cut that leaves a security out does not drop what is said about it.
func (s *Server) acb(ctx context.Context, in acbInput) (any, error) {
	now := s.now()
	var year int
	if in.Year != nil {
		parsed, err := report.ParseACBYear(fmt.Sprintf("%04d", *in.Year), now)
		if err != nil {
			return nil, acbYearRefusal(err)
		}
		year = parsed
	}
	cfg, err := s.newConfig(commandName)
	if err != nil {
		return nil, withLog(err, configRefusalLog(acbTwin))
	}
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	acb, err := srv.ACB(ctx, report.ACBRequest{
		Classification: classificationOf(cfg), Today: report.Today(now), Year: year, Securities: in.Security, Adjustments: acbAdjustmentsOf(cfg),
	})
	if err != nil {
		return nil, acbRefusal(err)
	}

	doc := document.NewACB(acb.Cut(), slices.Concat(cfg.WarningsAbsolute, document.ACBWarnings(acb, cfg.Path, document.ACBAdviceTool)))
	doc.Securities, doc.Warnings = capEvents(doc.Securities, doc.Warnings)

	return doc, nil
}

// capEvents is securities with their events cut to the first maxRows in all, in document order, every security
// kept with its header; a cut adds a last warning. A list within the cap and its warnings come back as given.
func capEvents(securities []document.ACBSecurity, warnings []string) ([]document.ACBSecurity, []string) {
	total := 0
	for _, security := range securities {
		total += len(security.Events)
	}
	if total <= maxRows {
		return securities, warnings
	}
	left := maxRows
	capped := slices.Clone(securities)
	for i := range capped {
		keep := min(left, len(capped[i].Events))
		capped[i].Events = capped[i].Events[:keep]
		left -= keep
	}

	return capped, append(warnings, capLine(toolACB, "events", total, "pass security to narrow"))
}

// acbAdjustmentsOf is the adjustments cfg lists, in file order.
func acbAdjustmentsOf(cfg config.Config) []report.ACBAdjustment {
	adjustments := make([]report.ACBAdjustment, 0, len(cfg.Adjustments))
	for _, a := range cfg.Adjustments {
		adjustments = append(adjustments, report.ACBAdjustment{
			SecurityID: a.Security, Date: a.Date, ReturnOfCapital: a.ReturnOfCapital, ReinvestedDistribution: a.ReinvestedDistribution,
		})
	}

	return adjustments
}
