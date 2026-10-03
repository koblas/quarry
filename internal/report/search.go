package report

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// searchCommand names search in its refusals; it equals the cli command word.
const searchCommand = "search"

// SearchRequest is what a search needs from its caller: the dates to list, the accounts to list
// (each an id or a name; none means every account) and the most transactions to return (0 returns every one).
type SearchRequest struct {
	Window   store.SearchWindow
	Accounts []string
	Limit    int
}

// Search is a search read: the request it answered and the transactions it matched.
type Search struct {
	store.Search

	Window store.SearchWindow
	// Accounts is the accounts the request named, in the order given and without repeats; none when it named none.
	Accounts []store.Account
	Limit    int
}

// Truncated reports whether the limit cut matches off the end of Rows.
func (s Search) Truncated() bool { return s.Matched > len(s.Rows) }

// Search lists the newest req.Limit transactions dated in req.Window, in the accounts req.Accounts names
// (every account when none). An account it cannot pick, or a store it cannot read, is a RefusalError.
func (s *Server) Search(ctx context.Context, req SearchRequest) (Search, error) {
	accounts, accountIDs, err := s.namedAccounts(ctx, searchCommand, req.Accounts)
	if err != nil {
		return Search{}, err
	}
	found, err := s.store.Search(ctx, store.SearchParams{Window: req.Window, AccountIDs: accountIDs, Limit: req.Limit})
	if err != nil {
		return Search{}, s.readRefusal(ctx, searchCommand, err)
	}
	return Search{Search: found, Window: req.Window, Accounts: accounts, Limit: req.Limit}, nil
}
