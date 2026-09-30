package cli

import "strings"

// errCSVAndJSON refuses --csv together with --json; the two pick one output format.
var errCSVAndJSON = UsageError{msg: "--csv and --json cannot be used together; choose one output format"}

// csvCell is one CSV field: Text, or a NULL (Null set) that prints as an unquoted empty field.
type csvCell struct {
	Text string
	Null bool
}

// csvField is c as one CSV field: quoted, with `"` doubled, when it holds a
// comma, a quote, CR or LF or is empty, so `""` differs from a NULL (nothing).
func csvField(c csvCell) string {
	if c.Null {
		return ""
	}
	if c.Text != "" && !strings.ContainsAny(c.Text, ",\"\r\n") {
		return c.Text
	}
	return `"` + strings.ReplaceAll(c.Text, `"`, `""`) + `"`
}

// csvRecord is cells as one CSV line: the fields joined by commas, ending in a newline.
func csvRecord(cells []csvCell) string {
	fields := make([]string, len(cells))
	for i, c := range cells {
		fields[i] = csvField(c)
	}
	return strings.Join(fields, ",") + "\n"
}
