package report_test

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_search_passes_the_window_the_accounts_ids_and_the_limit_to_the_store(t *testing.T) {
	var got store.SearchParams
	since, until := day(2026, time.January, 1), day(2026, time.March, 31)
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chqAccount, visaAccount), gotSearch: &got}))

	_, err := srv.Search(t.Context(), report.SearchRequest{
		Window: store.SearchWindow{Since: &since, Until: &until}, Accounts: []string{"Visa", "acct-chq"}, Limit: 20,
	})

	require.NoError(t, err)
	assert.Equal(t, store.SearchParams{
		Window: store.SearchWindow{Since: &since, Until: &until}, AccountIDs: []string{"acct-visa", "acct-chq"}, Limit: 20,
	}, got)
}

func Test_search_passes_the_amounts_to_the_store_and_echoes_them(t *testing.T) {
	var got store.SearchParams
	srv := report.NewServer(report.WithStore(fakeStore{gotSearch: &got}))
	amounts := report.SearchAmounts{Min: new(int64(1250)), Max: new(int64(99900))}

	result, err := srv.Search(t.Context(), report.SearchRequest{Amounts: amounts})

	require.NoError(t, err)
	assert.Equal(t, store.SearchParams{Min: new(int64(1250)), Max: new(int64(99900))}, got)
	assert.Equal(t, amounts, result.Amounts)
}

func Test_search_leaves_both_amount_bounds_open_when_the_request_gives_none(t *testing.T) {
	var got store.SearchParams
	srv := report.NewServer(report.WithStore(fakeStore{gotSearch: &got}))

	result, err := srv.Search(t.Context(), report.SearchRequest{})

	require.NoError(t, err)
	assert.Equal(t, store.SearchParams{}, got)
	assert.Equal(t, report.SearchAmounts{}, result.Amounts)
}

func Test_search_names_every_account_when_none_is_given(t *testing.T) {
	var got store.SearchParams
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{gotSearch: &got, accountsReads: &reads}))

	result, err := srv.Search(t.Context(), report.SearchRequest{})

	require.NoError(t, err)
	assert.Empty(t, got.AccountIDs)
	assert.Empty(t, result.Accounts)
	assert.Zero(t, reads)
}

func Test_search_echoes_the_accounts_the_request_named_in_the_order_given(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chqAccount, visaAccount)}))

	result, err := srv.Search(t.Context(), report.SearchRequest{Accounts: []string{"Visa", "acct-chq", "visa"}})

	require.NoError(t, err)
	assert.Equal(t, []store.Account{visaAccount, chqAccount}, result.Accounts)
}

func Test_search_carries_what_the_store_matched_and_the_request_it_answered(t *testing.T) {
	since := day(2026, time.January, 1)
	found := store.Search{
		Rows:         []store.SearchRow{{TransactionID: "txn-1"}},
		Matched:      4,
		Transactions: store.TransactionRange{First: day(2003, time.January, 2), Last: day(2026, time.September, 30)},
	}
	srv := report.NewServer(report.WithStore(fakeStore{search: found}))

	result, err := srv.Search(t.Context(), report.SearchRequest{Window: store.SearchWindow{Since: &since}, Limit: 1})

	require.NoError(t, err)
	assert.Equal(t, report.Search{
		Search: found, Window: store.SearchWindow{Since: &since}, Limit: 1,
	}, result)
}

func Test_search_truncated_is_true_only_when_matches_exceed_the_rows(t *testing.T) {
	cases := []struct {
		name   string
		search report.Search
		want   bool
	}{
		{name: "more matches than rows", search: report.Search{Search: store.Search{Rows: make([]store.SearchRow, 2), Matched: 3}}, want: true},
		{name: "as many matches as rows", search: report.Search{Search: store.Search{Rows: make([]store.SearchRow, 2), Matched: 2}}},
		{name: "no matches", search: report.Search{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.search.Truncated())
		})
	}
}

func Test_search_refuses_an_account_it_cannot_pick_without_reading_matches(t *testing.T) {
	got := store.SearchParams{Limit: 99}
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chqAccount), gotSearch: &got}))

	_, err := srv.Search(t.Context(), report.SearchRequest{Accounts: []string{"Nowhere"}})

	var refusal report.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, report.RefusalUnknownAccount, refusal.Kind)
	assert.Equal(t, store.SearchParams{Limit: 99}, got)
}

func Test_search_refuses_a_store_fault_as_a_store_refusal(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Search(t.Context(), report.SearchRequest{})

	var refusal report.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, report.RefusalStore, refusal.Kind)
	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_search_reports_an_interrupt_during_the_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Search(ctx, report.SearchRequest{})

	assert.EqualError(t, err, "search interrupted")
}

func Test_search_reports_an_interrupt_during_the_accounts_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Search(ctx, report.SearchRequest{Accounts: []string{"Visa"}})

	assert.EqualError(t, err, "search interrupted")
}

func Test_CheckSearchText_refuses_blank_text(t *testing.T) {
	cases := []struct {
		name string
		text *string
	}{
		{name: "empty", text: new("")},
		{name: "spaces", text: new("  ")},
		{name: "tab and newline", text: new("\t\n")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.ErrorIs(t, report.CheckSearchText(c.text), report.ErrBlankSearchText)
		})
	}
}

func Test_CheckSearchText_accepts_absent_or_non_blank_text(t *testing.T) {
	cases := []struct {
		name string
		text *string
	}{
		{name: "no text", text: nil},
		{name: "text with surrounding spaces", text: new(" a ")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.NoError(t, report.CheckSearchText(c.text))
		})
	}
}

func Test_CheckUTF8_accepts_nil_and_valid_values(t *testing.T) {
	cases := []struct {
		name  string
		value *string
	}{
		{name: "nil", value: nil},
		{name: "ascii", value: new("Food")},
		{name: "multibyte", value: new("café ☕")},
		{name: "empty", value: new("")},
		{name: "a NUL byte", value: new("a\x00b")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.NoError(t, report.CheckUTF8(report.SearchInputText, c.value))
		})
	}
}

func Test_CheckUTF8_refuses_invalid_bytes_with_the_field_and_the_value(t *testing.T) {
	cases := []struct {
		name  string
		field report.SearchInput
		value string
		want  string
	}{
		{
			name: "a lone continuation byte in text", field: report.SearchInputText, value: "\xff",
			want: `search text "\xff" is not valid UTF-8; set your terminal or script to UTF-8`,
		},
		{
			name: "a truncated rune in text", field: report.SearchInputText, value: "caf\xc3",
			want: `search text "caf\xc3" is not valid UTF-8; set your terminal or script to UTF-8`,
		},
		{
			name: "a lone continuation byte in the category", field: report.SearchInputCategory, value: "\xff",
			want: `--category "\xff" is not valid UTF-8; set your terminal or script to UTF-8`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := report.CheckUTF8(c.field, &c.value)

			var bad report.InvalidUTF8Error
			require.ErrorAs(t, err, &bad)
			assert.Equal(t, report.InvalidUTF8Error{Field: c.field, Value: c.value}, bad)
			assert.EqualError(t, err, c.want)
		})
	}
}

func Test_search_refuses_blank_text_without_reading(t *testing.T) {
	got := store.SearchParams{Limit: 99}
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chqAccount), gotSearch: &got, accountsReads: &reads}))

	_, err := srv.Search(t.Context(), report.SearchRequest{Text: new(" "), Accounts: []string{"Visa"}})

	require.ErrorIs(t, err, report.ErrBlankSearchText)
	assert.Equal(t, store.SearchParams{Limit: 99}, got)
	assert.Zero(t, reads)
}

func Test_search_passes_the_text_untrimmed_and_echoes_it(t *testing.T) {
	var got store.SearchParams
	srv := report.NewServer(report.WithStore(fakeStore{gotSearch: &got}))

	result, err := srv.Search(t.Context(), report.SearchRequest{Text: new(" Costco ")})

	require.NoError(t, err)
	assert.Equal(t, " Costco ", got.Text)
	assert.Equal(t, new(" Costco "), result.Text)
}

func Test_search_without_text_passes_none_and_echoes_none(t *testing.T) {
	var got store.SearchParams
	srv := report.NewServer(report.WithStore(fakeStore{gotSearch: &got}))

	result, err := srv.Search(t.Context(), report.SearchRequest{})

	require.NoError(t, err)
	assert.Empty(t, got.Text)
	assert.Nil(t, result.Text)
}

func Test_search_passes_the_category_to_the_store_and_echoes_it(t *testing.T) {
	var got store.SearchParams
	srv := report.NewServer(report.WithStore(fakeStore{gotSearch: &got}))

	result, err := srv.Search(t.Context(), report.SearchRequest{Category: new("food:Groceries")})

	require.NoError(t, err)
	assert.Equal(t, new("food:Groceries"), got.Category)
	assert.Equal(t, new("food:Groceries"), result.Category)
}

func Test_search_without_a_category_passes_none_and_echoes_none(t *testing.T) {
	var got store.SearchParams
	srv := report.NewServer(report.WithStore(fakeStore{gotSearch: &got}))

	result, err := srv.Search(t.Context(), report.SearchRequest{})

	require.NoError(t, err)
	assert.Nil(t, got.Category)
	assert.Nil(t, result.Category)
}

func Test_search_refuses_a_category_the_store_says_names_none(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{search: store.Search{UnknownCategory: true}}))

	_, err := srv.Search(t.Context(), report.SearchRequest{Category: new("Fod")})

	var refusal report.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, report.RefusalUnknownCategory, refusal.Kind)
	assert.Equal(t, "Fod", refusal.Arg)
	assert.EqualError(t, err, `no category named "Fod"; list them with quarry sql "SELECT full_path FROM categories ORDER BY full_path"`)
}

func Test_search_refuses_an_account_it_cannot_pick_before_a_category_it_cannot_pick(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chqAccount), search: store.Search{UnknownCategory: true}}))

	_, err := srv.Search(t.Context(), report.SearchRequest{Accounts: []string{"Nowhere"}, Category: new("Fod")})

	var refusal report.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, report.RefusalUnknownAccount, refusal.Kind)
}

func Test_search_refuses_a_store_fault_as_a_store_refusal_whatever_the_category(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr, search: store.Search{UnknownCategory: true}}), report.WithHome(refusalHome))

	_, err := srv.Search(t.Context(), report.SearchRequest{Category: new("Fod")})

	var refusal report.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, report.RefusalStore, refusal.Kind)
}
