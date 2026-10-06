package mcp_test

import (
	"testing"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/stretchr/testify/assert"
)

const (
	acbLogPrefix       = "quarry: mcp: acb: "
	acbConfigLog       = "cannot read quarry's config file; run quarry acb to see why"
	acbSecurityLog     = "refused the call's security; details went to the client only"
	acbYearLog         = "refused the call's year; details went to the client only"
	acbArgumentsLog    = "refused the call's arguments; details went to the client only"
	acbUnclassifiedOne = "acb needs every brokerage and retirement account classified; 1 account is " +
		"in neither accounts.registered nor accounts.non-registered in ~/Library/Application Support/quarry/config.toml; " +
		"data_quality with type unclassified-account and status all lists it"
	acbUnclassifiedTwo = "acb needs every brokerage and retirement account classified; 2 accounts are " +
		"in neither accounts.registered nor accounts.non-registered in ~/Library/Application Support/quarry/config.toml; " +
		"data_quality with type unclassified-account and status all lists them"
)

func Test_acb_words_the_unclassified_refusal_for_a_tool_with_its_count(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{name: "one account lists it", cfg: config.Config{Registered: []string{"acct-r"}}, want: acbUnclassifiedOne},
		{name: "two accounts list them", cfg: config.Config{}, want: acbUnclassifiedTwo},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: c.cfg}).load), atACBToday())

			result := h.acb(t, map[string]any{})

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Equal(t, acbLogPrefix+c.want+"\n", h.stderr.String())
		})
	}
}

func Test_acb_refuses_unclassified_accounts_whose_finding_the_config_ignores(t *testing.T) {
	cfg := config.Config{Registered: []string{"acct-r"}, Ignore: []string{"unclassified-account:acct-n"}}
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: cfg}).load), atACBToday())

	result := h.acb(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, acbUnclassifiedOne, textOf(t, result))
}

func Test_acb_names_the_first_security_that_names_none_without_it_reaching_stderr(t *testing.T) {
	cases := []struct {
		name       string
		securities []string
		want       string
	}{
		{
			name: "the first in argument order", securities: []string{"sec-acme", "XYZ", "ABC"},
			want: `acb covers no security named "XYZ"; acb with no arguments lists every security it covers`,
		},
		{
			name: "an empty name", securities: []string{""},
			want: `acb covers no security named ""; acb with no arguments lists every security it covers`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

			result := h.acb(t, map[string]any{"security": c.securities})

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Equal(t, acbLogPrefix+acbSecurityLog+"\n", h.stderr.String())
		})
	}
}

func Test_acb_refuses_a_year_after_this_one_without_it_reaching_stderr(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

	result := h.acb(t, map[string]any{"year": 2027})

	assert.True(t, result.IsError)
	assert.Equal(t, "year 2027 is after this year; pass this year or an earlier one", textOf(t, result))
	assert.Equal(t, acbLogPrefix+acbYearLog+"\n", h.stderr.String())
}

func Test_acb_refuses_a_year_before_it_reads_the_config_or_the_store(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	fake := &fakeStore{history: acbHistory()}
	h := newHarness(t, fake, nil, mcp.WithConfig(stub.load), atACBToday())

	result := h.acb(t, map[string]any{"year": 2027})

	assert.True(t, result.IsError)
	assert.Equal(t, acbLogPrefix+acbYearLog+"\n", h.stderr.String())
	assert.Empty(t, stub.commands)
	assert.Zero(t, h.built)
}

func Test_acb_refuses_an_unreadable_config_before_building_the_report(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig(stub.load), atACBToday())

	result := h.acb(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errBadConfig.Error(), textOf(t, result))
	assert.Zero(t, h.built)
	assert.Equal(t, acbLogPrefix+acbConfigLog+"\n", h.stderr.String())
}

func Test_acb_refuses_arguments_its_schema_does_not_allow(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "a year given as text", arguments: map[string]any{"year": "2024"}},
		{name: "year 0", arguments: map[string]any{"year": 0}},
		{name: "year 10000", arguments: map[string]any{"year": 10000}},
		{name: "a currency", arguments: map[string]any{"currency": "USD"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &configStub{cfg: acbConfig()}
			h := newHarness(t, &fakeStore{history: acbHistory()}, nil, mcp.WithConfig(stub.load), atACBToday())

			result := h.acb(t, c.arguments)

			assert.True(t, result.IsError)
			assert.Equal(t, acbLogPrefix+acbArgumentsLog+"\n", h.stderr.String())
			assert.Empty(t, stub.commands)
		})
	}
}
