package document_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/stretchr/testify/assert"
)

func Test_UnmatchedAccountWarnings_names_the_config_and_the_list_of_each_id_registered_first(t *testing.T) {
	got := document.UnmatchedAccountWarnings(configShown, report.UnmatchedAccounts{
		Registered: []string{"acct-99"}, NonRegistered: []string{"acct-98"},
	})

	assert.Equal(t, []string{
		`~/config.toml: accounts.registered lists "acct-99", which is not an account in quarry's store; quarry skips it`,
		`~/config.toml: accounts.non-registered lists "acct-98", which is not an account in quarry's store; quarry skips it`,
	}, got)
}

func Test_UnmatchedAccountWarnings_masks_an_account_number_before_quoting_it(t *testing.T) {
	got := document.UnmatchedAccountWarnings(configShown, report.UnmatchedAccounts{NonRegistered: []string{"12345678"}})

	assert.Equal(t, []string{`~/config.toml: accounts.non-registered lists "****5678", which is not an account in quarry's store; quarry skips it`}, got)
}

func Test_UnmatchedAccountWarnings_escapes_an_id_holding_a_quote(t *testing.T) {
	got := document.UnmatchedAccountWarnings(configShown, report.UnmatchedAccounts{Registered: []string{`a"b`}})

	assert.Equal(t, []string{`~/config.toml: accounts.registered lists "a\"b", which is not an account in quarry's store; quarry skips it`}, got)
}

func Test_UnmatchedAccountWarnings_repeats_a_repeated_id(t *testing.T) {
	got := document.UnmatchedAccountWarnings(configShown, report.UnmatchedAccounts{Registered: []string{"acct-99", "acct-99"}})

	assert.Len(t, got, 2)
}

func Test_UnmatchedAccountWarnings_is_an_empty_list_not_nil_when_nothing_is_unmatched(t *testing.T) {
	got := document.UnmatchedAccountWarnings(configShown, report.UnmatchedAccounts{})

	assert.NotNil(t, got)
	assert.Empty(t, got)
}
