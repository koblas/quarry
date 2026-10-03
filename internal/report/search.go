package report

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/store"
)

// searchCommand names search in its refusals; it equals the cli command word.
const searchCommand = "search"

// ErrBlankSearchText is the refusal of search text that is empty or only whitespace.
var ErrBlankSearchText = errors.New("search text is blank; leave it out to search by date, account, category or amount alone")

// SearchRequest is what a search needs from its caller: dates, accounts (ids or names; none is every account),
// text and category (nil for none), the amount range from ParseSearchAmounts and the most transactions (0 is all).
type SearchRequest struct {
	Window   store.SearchWindow
	Accounts []string
	Text     *string
	Category *string
	Amounts  SearchAmounts
	Limit    int
}

// Search is a search read: the request it answered and the transactions it matched.
type Search struct {
	store.Search

	Window store.SearchWindow
	// Accounts is the accounts the request named, in the order given and without repeats; none when it named none.
	Accounts []store.Account
	// Text is the text exactly as the request gave it, untrimmed; nil when the request gave none.
	Text *string
	// Category is the category exactly as the request gave it; nil when it gave none.
	Category *string
	Amounts  SearchAmounts
	Limit    int
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

// SearchInput names the search input an InvalidUTF8Error refused.
type SearchInput int

// The inputs CheckUTF8 can refuse.
const (
	SearchInputText SearchInput = iota
	SearchInputCategory
)

// InvalidUTF8Error is the refusal of search text or a category that is not valid UTF-8; Error() is the CLI line.
type InvalidUTF8Error struct {
	Field SearchInput
	Value string
}

// Error words the refusal as the command line does, quoting the value with %q.
func (e InvalidUTF8Error) Error() string {
	label := "search text"
	if e.Field == SearchInputCategory {
		label = "--category"
	}
	return fmt.Sprintf("%s %q is not valid UTF-8; set your terminal or script to UTF-8", label, e.Value)
}

// CheckUTF8 returns an InvalidUTF8Error for a value that is not valid UTF-8; nil is fine. The MCP transport
// decodes arguments to valid UTF-8, so only the command line calls it.
func CheckUTF8(field SearchInput, value *string) error {
	if value != nil && !utf8.ValidString(*value) {
		return InvalidUTF8Error{Field: field, Value: *value}
	}
	return nil
}

// Search lists the newest req.Limit transactions dated in req.Window, in the accounts req.Accounts names
// (every account when none), containing req.Text and in req.Category when given. Blank text is ErrBlankSearchText;
// an account or category it cannot pick, or a store it cannot read, is a RefusalError.
func (s *Server) Search(ctx context.Context, req SearchRequest) (Search, error) {
	if err := CheckSearchText(req.Text); err != nil {
		return Search{}, err
	}
	accounts, accountIDs, err := s.namedAccounts(ctx, searchCommand, req.Accounts)
	if err != nil {
		return Search{}, err
	}
	params := store.SearchParams{
		Window: req.Window, AccountIDs: accountIDs, Category: req.Category, Min: req.Amounts.Min, Max: req.Amounts.Max, Limit: req.Limit,
	}
	if req.Text != nil {
		params.Text = *req.Text
	}
	found, err := s.store.Search(ctx, params)
	if err != nil {
		return Search{}, s.readRefusal(ctx, searchCommand, err)
	}
	if found.UnknownCategory {
		return Search{}, unknownCategoryRefusal(*req.Category)
	}
	return Search{
		Search: found, Window: req.Window, Accounts: accounts, Text: req.Text, Category: req.Category, Amounts: req.Amounts, Limit: req.Limit,
	}, nil
}
