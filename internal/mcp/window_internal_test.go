// White-box: windowRefusal and logLine are unexported, and the charge kinds reach no tool yet.
package mcp

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
)

type windowRefusalCase struct {
	name    string
	refusal report.WindowError
	want    string
}

func windowRefusalCases() []windowRefusalCase {
	return []windowRefusalCase{
		{
			name:    "since that is not a date",
			refusal: report.WindowError{Kind: report.WindowNotADate, Bound: "since", Value: "2024-13"},
			want:    `since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			name:    "until that is not a date",
			refusal: report.WindowError{Kind: report.WindowNotADate, Bound: "until", Value: "last spring"},
			want:    `until "last spring" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			name:    "an empty value is quoted so it shows",
			refusal: report.WindowError{Kind: report.WindowNotADate, Bound: "since", Value: ""},
			want:    `since "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			name:    "since after today",
			refusal: report.WindowError{Kind: report.WindowSinceAfterToday, Bound: "since", Value: "2099"},
			want:    "since 2099 is after today; pass until to include future-dated transactions",
		},
		{
			name:    "charge since after today names recurring_charges",
			refusal: report.WindowError{Kind: report.WindowChargeSinceAfterToday, Bound: "since", Value: "2099", Command: "recurring_charges"},
			want:    "since 2099 is after today; recurring_charges lists charges up to today only, so pass an earlier since",
		},
		{
			name:    "charge since after today names anomalies",
			refusal: report.WindowError{Kind: report.WindowChargeSinceAfterToday, Bound: "since", Value: "2030-05", Command: "anomalies"},
			want:    "since 2030-05 is after today; anomalies lists charges up to today only, so pass an earlier since",
		},
		{
			name:    "since after until",
			refusal: report.WindowError{Kind: report.WindowSinceAfterUntil, Bound: "since", Value: "2025", Other: "2024"},
			want:    "since 2025 is after until 2024",
		},
		{
			name:    "until before the default since",
			refusal: report.WindowError{Kind: report.WindowUntilBeforeDefault, Bound: "until", Value: "2025-03", DefaultSince: "2026-01-01"},
			want:    "until 2025-03 is before the default since 2026-01-01; pass since too",
		},
	}
}

func Test_windowRefusal_words_every_kind_with_bare_bound_names(t *testing.T) {
	for _, c := range windowRefusalCases() {
		t.Run(c.name, func(t *testing.T) {
			got := windowRefusal(c.refusal)

			assert.EqualError(t, got, c.want)
		})
	}
}

func Test_windowRefusal_logs_the_class_line_without_the_callers_values(t *testing.T) {
	for _, c := range windowRefusalCases() {
		t.Run(c.name, func(t *testing.T) {
			got := windowRefusal(c.refusal)

			assert.Equal(t, windowRefusedLog, logLine(got))
		})
	}
}

func Test_windowRefusal_keeps_every_value_of_the_caller_off_the_log_line(t *testing.T) {
	refusal := report.WindowError{Kind: report.WindowSinceAfterUntil, Bound: "since", Value: "zorblax-since", Other: "zorblax-until"}

	got := logLine(windowRefusal(refusal))

	assert.NotContains(t, got, "zorblax")
}
