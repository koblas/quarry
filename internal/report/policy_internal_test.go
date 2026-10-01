// White-box: the quiet-period constants are unexported policy, so only an in-package test
// can hold them to the numbers the recurring help states.
package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_recurring_policy_constants_equal_the_numbers_its_help_states(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{name: "a weekly series ends after 14 days", got: weeklyEndedAfterDays, want: 14},
		{name: "a monthly series ends after 45 days", got: monthlyEndedAfterDays, want: 45},
		{name: "a quarterly series ends after 120 days", got: quarterlyEndedAfterDays, want: 120},
		{name: "a yearly series ends after 400 days", got: annualEndedAfterDays, want: 400},
		{name: "a price change is a step of more than 5%", got: PriceChangeMinPct, want: 5},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.got)
		})
	}
}

func Test_anomalies_policy_constants_equal_the_numbers_its_help_states(t *testing.T) {
	cases := []struct {
		name string
		got  int64
		want int64
	}{
		{name: "the payee's median is multiplied by 2", got: AnomalyPayeeMultiplier, want: 2},
		{name: "a payee needs at least 3 earlier charges", got: AnomalyPayeeMinHistory, want: 3},
		{name: "the category's median is multiplied by 5", got: AnomalyCategoryMultiplier, want: 5},
		{name: "a category needs at least 10 earlier charges", got: AnomalyCategoryMinHistory, want: 10},
		{name: "charges under 100.00 are never listed", got: AnomalyMinAmount, want: 100_00},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.got)
		})
	}
}
