package mcp

import (
	"context"
	"slices"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// configRefusalLog is the stderr line of a config that cannot be read, naming twin, the quarry command that shows why;
// the reason can quote the file or a value in it.
func configRefusalLog(twin string) string {
	return "cannot read quarry's config file; run quarry " + twin + " to see why"
}

// dataQuality lists the findings in.Status and in.Type select, at most in.Limit with maxItems items each,
// and a warning for each cut.
func (s *Server) dataQuality(ctx context.Context, in dataQualityInput) (any, error) {
	cfg, err := s.newConfig(commandName)
	if err != nil {
		return nil, withLog(err, configRefusalLog("findings"))
	}
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	status, typ := finding.Status(in.Status), finding.Type(in.Type)
	listing, err := srv.Findings(ctx, report.FindingsRequest{
		Ignore:         cfg.Ignore,
		Classification: report.Classification{Registered: cfg.Registered, NonRegistered: cfg.NonRegistered},
		Status:         status,
		Type:           typ,
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // a RefusalError is the tool's answer, sent verbatim
	}
	warnings := append(slices.Clone(cfg.WarningsAbsolute), document.UnmatchedIgnoreWarnings(cfg.Path, listing.Unmatched)...)
	kept, total, cut := capFindings(listing, in.Limit)
	if cut {
		warnings = append(warnings, findingsCapWarning(in.Limit, total, status, typ))
	}
	kept, itemWarnings := capItems(kept)
	return document.NewFindingsList(kept, status, typ, append(warnings, itemWarnings...)), nil
}

// capFindings keeps the first limit findings of listing and reports how many it held and whether any were cut.
func capFindings(listing report.FindingsListing, limit int) (report.FindingsListing, int, bool) {
	kept := report.FindingsListing{Counts: listing.Counts, Unmatched: listing.Unmatched}
	total, left := 0, limit
	for _, group := range listing.Groups {
		total += len(group.Findings)
		take := min(left, len(group.Findings))
		left -= take
		if take > 0 {
			kept.Groups = append(kept.Groups, report.FindingsGroup{Type: group.Type, Findings: slices.Clone(group.Findings[:take])})
		}
	}
	return kept, total, total > limit
}

// capItems keeps the first maxItems items of each finding in a copy of listing, with a warning per cut finding.
func capItems(listing report.FindingsListing) (report.FindingsListing, []string) {
	var warnings []string
	capped := report.FindingsListing{Counts: listing.Counts, Unmatched: listing.Unmatched}
	for _, group := range listing.Groups {
		findings := slices.Clone(group.Findings)
		for i, f := range findings {
			if len(f.Items) > maxItems {
				warnings = append(warnings, itemsCapWarning(f.ID, len(f.Items)))
				findings[i].Items = slices.Clone(f.Items[:maxItems])
			}
		}
		capped.Groups = append(capped.Groups, report.FindingsGroup{Type: group.Type, Findings: findings})
	}
	return capped, warnings
}

// findingsCapWarning is the warning that data_quality listed only the first limit of total
// findings of status, naming the ways to see more.
func findingsCapWarning(limit, total int, status finding.Status, typ finding.Type) string {
	word := ""
	if status != report.FindingsAll {
		word = string(status) + " "
	}
	line := "listed the first " + humanize.Thousands(limit) + " of " + humanize.Thousands(total) + " " + word + "findings"
	switch {
	case limit < maxRows && typ == "":
		return line + "; pass type to narrow the list, or a larger limit (at most " + humanize.Thousands(maxRows) + ")"
	case limit < maxRows:
		return line + "; pass a larger limit (at most " + humanize.Thousands(maxRows) + ")"
	case typ == "":
		return line + "; pass type to narrow the list"
	}
	return line
}

// itemsCapWarning is the warning that data_quality listed only the first maxItems of the total items of finding id.
func itemsCapWarning(id string, total int) string {
	return "finding " + id + " lists the first " + humanize.Thousands(maxItems) + " of " + humanize.Count(total, "item", "items") +
		"; query finding_items WHERE finding_id = '" + id + "' for the rest"
}
