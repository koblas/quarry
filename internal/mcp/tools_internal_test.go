// White-box: anomaliesDescription and maxRows are unexported.
package mcp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
)

func Test_anomaliesDescription_states_the_thresholds_the_report_applies(t *testing.T) {
	description := strings.Join(strings.Fields(anomaliesDescription), " ")
	floor := report.AnomalyMinAmount

	cases := []struct {
		name string
		want string
	}{
		{
			name: "payee rule",
			want: fmt.Sprintf("more than %d times the median of the payee's earlier charges (when it has at least %d)",
				report.AnomalyPayeeMultiplier, report.AnomalyPayeeMinHistory),
		},
		{
			name: "category rule",
			want: fmt.Sprintf("more than %d times the median of the category's earlier charges (at least %d)",
				report.AnomalyCategoryMultiplier, report.AnomalyCategoryMinHistory),
		},
		{name: "smallest charge listed", want: fmt.Sprintf("Charges under %d.%02d in their account's own currency", floor/100, floor%100)},
		{name: "list cap", want: fmt.Sprintf("Returns at most %d charges", maxRows)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Contains(t, description, c.want)
		})
	}
}
