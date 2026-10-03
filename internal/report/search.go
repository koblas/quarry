package report

import (
	"context"
	"errors"
	"strings"

	"github.com/koblas/quarry/internal/store"
)

// searchCommand names search in its refusals; it equals the cli command word.
const searchCommand = "search"

// ErrBlankSearchText is the refusal of search text that is empty or only whitespace.
var ErrBlankSearchText = errors.New("search text is blank; leave it out to search by date, account, category or amount alone")

// SearchRequest is what a search needs from its caller: the dates to list, the accounts to list
// (each an id or a name; none means every account), the text a payee or memo must contain (nil for none)
// and the most transactions to return (0 returns every one).
type SearchRequest struct {
	Window   store.SearchWindow
	Accounts []string
	Text     *string
	Limit    int
}

// Search is a search read: the request it answered and the transactions it matched.
type Search struct {
	store.Search

	Window store.SearchWindow
	// Accounts is the accounts the request named, in the order given and without repeats; none when it named none.
	Accounts []store.Account
	// Text is the text exactly as the request gave it, untrimmed; nil when the request gave none.
	Text  *string
	Limit int
}

// Truncated reports whether the limit cut matches off the end of Rows.
func (s Search) Truncated() bool { return s.Matched > len(s.Rows) }

// CheckSearchText returns ErrBlankSearchText for text that is empty or only whitespace; nil text is fine.
func CheckSearchText(text *string) error {
	if text != nil && strings.TrimSpace(*text) == "" {
		return ErrBlankSearchText
	}
	return nil
}

// Search lists the newest req.Limit transactions dated in req.Window, in the accounts req.Accounts names
// (every account when none), containing req.Text when given. Blank text is ErrBlankSearchText; an account it
// cannot pick, or a store it cannot read, is a RefusalError.
func (s *Server) Search(ctx context.Context, req SearchRequest) (Search, error) {
	if err := CheckSearchText(req.Text); err != nil {
		return Search{}, err
	}
	accounts, accountIDs, err := s.namedAccounts(ctx, searchCommand, req.Accounts)
	if err != nil {
		return Search{}, err
	}
	params := store.SearchParams{Window: req.Window, AccountIDs: accountIDs, Limit: req.Limit}
	if req.Text != nil {
		params.Text = *req.Text
	}
	found, err := s.store.Search(ctx, params)
	if err != nil {
		return Search{}, s.readRefusal(ctx, searchCommand, err)
	}
	return Search{Search: found, Window: req.Window, Accounts: accounts, Text: req.Text, Limit: req.Limit}, nil
}
