// White-box: parseMoney is unexported, called only from row-mapping code
// that already knows a column is non-NULL. Its integer/real x
// precision/range decision matrix is combinatorial and not economical to
// drive one snapshot fixture per case.
package importer

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_parseMoney(t *testing.T) {
	cases := []struct {
		name      string
		typ, text string
		wantCents int64
		wantFault moneyFault
	}{
		{"integer whole dollars", "integer", "12", 1200, moneyOK},
		{"integer negative dollars", "integer", "-12", -1200, moneyOK},
		{"integer bound in", "integer", "9999999999999999", 9999999999999999 * 100, moneyOK},
		{"integer bound out", "integer", "10000000000000000", 0, moneyTooLarge},
		{"integer huge digit string", "integer", "184467440737095516", 0, moneyTooLarge},
		{"real one decimal", "real", "12.5", 1250, moneyOK},
		{"real two decimals", "real", "12.34", 1234, moneyOK},
		{"real negative", "real", "-12.34", -1234, moneyOK},
		{"real more than two decimals", "real", "12.345", 0, moneyPrecision},
		{"real exponent form", "real", "1e+20", 0, moneyTooLarge},
		{"real bound in", "real", "999999999.99", 99999999999, moneyOK},
		{"real bound out", "real", "1000000000.0", 0, moneyTooLarge},
		{"real negative bound in", "real", "-999999999.99", -99999999999, moneyOK},
		{"real negative bound out", "real", "-1000000000.0", 0, moneyTooLarge},
		{"real infinity", "real", "Inf", 0, moneyTooLarge},
		{"real negative infinity", "real", "-Inf", 0, moneyTooLarge},
		{"real fraction with a sign", "real", "1.-5", 0, moneyNotANumber},
		{"real fraction not a digit", "real", "1.x", 0, moneyNotANumber},
		{"real integer part with a plus sign", "real", "+1.5", 0, moneyNotANumber},
		{"real with no integer digits", "real", ".5", 0, moneyNotANumber},
		{"text storage", "text", "12.34", 0, moneyNotANumber},
		{"text empty", "text", "", 0, moneyNotANumber},
		{"text whitespace", "text", "   ", 0, moneyNotANumber},
		{"blob storage", "blob", "\xde\xad\xbe\xef", 0, moneyNotANumber},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotCents, gotFault := parseMoney(c.typ, c.text)

			assert.Equal(t, c.wantFault, gotFault)
			assert.Equal(t, c.wantCents, gotCents)
		})
	}
}

// scanBelowRealBound is how many cent values below realIntBound the scan
// renders through SQLite, the range where a REAL's text is longest.
const scanBelowRealBound = 1_000_000

// SQLite renders each REAL, so the test pins the linked SQLite's own text.
func Test_parseMoney_reads_every_real_just_below_the_bound_exactly(t *testing.T) {
	db, err := sqlite.OpenMemory(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	const highestCents = realIntBound*100 - 1
	var wrong []string

	err = db.QueryRows(t.Context(), `
WITH RECURSIVE n(cents) AS (SELECT ? UNION ALL SELECT cents - 1 FROM n WHERE cents > ?)
SELECT cents, typeof(cents / 100.0), CAST(cents / 100.0 AS TEXT) FROM n`,
		[]any{highestCents, highestCents - scanBelowRealBound + 1},
		func(scan func(dest ...any) error) error {
			var cents int64
			var typ, text string
			if err := scan(&cents, &typ, &text); err != nil {
				return err
			}
			if got, fault := parseMoney(typ, text); got != cents || fault != moneyOK {
				wrong = append(wrong, text)
			}
			return nil
		})

	require.NoError(t, err)
	assert.Empty(t, wrong)
}
