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
