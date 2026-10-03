package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
)

// spendTwin is the quarry command whose output the spending tool matches.
const spendTwin = "spend"

// spending totals the spending the call's window, accounts and currency select, grouped by in.By, as spend --json does.
func (s *Server) spending(ctx context.Context, in spendingInput) (any, error) {
	// Today is read once per call, here: a second read could straddle midnight.
	window, err := report.ParseWindow(in.Since, in.Until, s.now())
	if err != nil {
		return nil, windowRefusal(err)
	}
	currency, configWarnings, err := s.resolveCurrency(in.Currency, spendTwin)
	if err != nil {
		return nil, err
	}
	by, err := parseSpendingGroup(in.By)
	if err != nil {
		// unreachable: see parseSpendingGroup
		return nil, err
	}
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	spent, err := srv.Spend(ctx, report.SpendRequest{Window: window, By: by, Accounts: in.Accounts, Currency: currency})
	if err != nil {
		return nil, accountRefusal(err)
	}
	doc := document.NewSpending(spent, append(configWarnings, document.SpendingWarnings(spent, toolSpending)...))
	doc.Rows, doc.Warnings = capList(doc.Rows, doc.Warnings, toolSpending, "rows",
		"totals count every row; pass a shorter period or fewer accounts, or query v_spending for the rest")
	return doc, nil
}

// resolveCurrency is name when given, else reporting.currency from the config and its warnings;
// an unreadable config is refused with a stderr line naming twin.
func (s *Server) resolveCurrency(name, twin string) (money.Currency, []string, error) {
	if name != "" {
		currency, _ := money.ParseCurrency(name) // the schema's enum admits only spellings it reads
		return currency, nil, nil
	}
	cfg, err := s.newConfig(commandName)
	if err != nil {
		return money.Native, nil, withLog(err, configRefusalLog(twin))
	}
	return cfg.Currency, slices.Clone(cfg.WarningsAbsolute), nil
}

// errUnknownBy is a by value outside the schema enum, which no schema-validated call carries.
var errUnknownBy = errors.New("by names no value of the tool's enum")

// parseSpendingGroup is the grouping whose String is name; a name no grouping has is an error, never a default.
func parseSpendingGroup(name string) (store.SpendingGroup, error) {
	for _, group := range store.SpendingGroups() {
		if group.String() == name {
			return group, nil
		}
	}
	// unreachable: the schema's enum admits only the String of a SpendingGroup, and its default supplies category
	return 0, fmt.Errorf("%w: %q", errUnknownBy, name)
}
