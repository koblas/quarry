package config_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_load_reads_acb_adjustments_as_exact_cents(t *testing.T) {
	_, cfg, err := load(t, `[[acb.adjustment]]
security = "sec-41"
date = 2024-12-31
return-of-capital = 12.34

[[acb.adjustment]]
security = "sec-41"
date = 2024-12-31
reinvested-distribution = 56.78

[[acb.adjustment]]
security = 'sec-7'
date = 2023-01-05
return-of-capital = 1.15
reinvested-distribution = 0.29
`)

	require.NoError(t, err)
	assert.Equal(t, []config.Adjustment{
		{Security: "sec-41", Date: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC), ReturnOfCapital: 1234},
		{Security: "sec-41", Date: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC), ReinvestedDistribution: 5678},
		{Security: "sec-7", Date: time.Date(2023, 1, 5, 0, 0, 0, 0, time.UTC), ReturnOfCapital: 115, ReinvestedDistribution: 29},
	}, cfg.Adjustments)
	assert.Empty(t, cfg.Warnings)
}

func Test_load_warns_of_an_unknown_key_beside_acb_adjustments_in_both_warning_lists(t *testing.T) {
	cases := []struct {
		name    string
		content string
		key     string
	}{
		{name: "a subkey inside an item", content: goodItem + "memo = \"year-end\"\n", key: "acb.adjustment.memo"},
		{name: "a sibling of adjustment under acb", content: goodItem + "\n[acb]\nother = 1\n", key: "acb.other"},
		{name: "a known subkey in another letter case", content: goodItem + "Date = 2024-12-30\n", key: "acb.adjustment.Date"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, []string{shownPath + ": unknown key " + c.key + "; quarry ignores it"}, cfg.Warnings)
			assert.Equal(t, []string{cfg.Path + ": unknown key " + c.key + "; quarry ignores it"}, cfg.WarningsAbsolute)
		})
	}
}
