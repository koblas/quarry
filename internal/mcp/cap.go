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
	return list[:maxRows], append(warnings, capLine(tool, noun, len(list), advice))
}

// capLine is the warning that tool listed the first maxRows of total noun, with advice for seeing the rest.
func capLine(tool, noun string, total int, advice string) string {
	return fmt.Sprintf("%s lists the first %s %s of %s; %s", tool, humanize.Thousands(maxRows), noun, humanize.Thousands(total), advice)
}
