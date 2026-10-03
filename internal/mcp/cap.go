package mcp

import (
	"fmt"

	"github.com/koblas/quarry/internal/platform/humanize"
)

// capList is list cut to maxRows, with warnings extended by a line naming tool, noun and advice;
// a list within the cap and its warnings come back as given. The line goes last.
func capList[T any](list []T, warnings []string, tool, noun, advice string) ([]T, []string) {
	if len(list) <= maxRows {
		return list, warnings
	}
	line := fmt.Sprintf("%s lists the first %s %s of %s; %s", tool, humanize.Thousands(maxRows), noun, humanize.Thousands(len(list)), advice)
	return list[:maxRows], append(warnings, line)
}
