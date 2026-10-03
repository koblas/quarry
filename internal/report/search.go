package report

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

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

// Search lists the newest req.Limit transactions dated in req.Window, in the accounts req.Accounts names.
func (*Server) Search(context.Context, SearchRequest) (Search, error) {
	return Search{}, nil
}
