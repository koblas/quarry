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
	t.Parallel()
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
		{"real float residue snaps to its cent", "real", "-55396.139999999992", -5539614, moneyOK},
		{"real short residue snaps to its cent", "real", "-15.67000000001", -1567, moneyOK},
		{"real classic sum residue snaps to its cent", "real", "0.30000000000000004", 30, moneyOK},
		{"real on the snap tolerance snaps", "real", "12.340001", 1234, moneyOK},
		{"real negative on the snap tolerance snaps", "real", "-12.340001", -1234, moneyOK},
		{"real beyond the snap tolerance", "real", "12.3400011", 0, moneyPrecision},
		{"real negative beyond the snap tolerance", "real", "-12.3400011", 0, moneyPrecision},
		{"real just below a cent snaps up", "real", "12.339999", 1234, moneyOK},
		{"real sub-tolerance negative snaps to zero not negative", "real", "-0.0000001", 0, moneyOK},
		{"real above the bound with a long fraction", "real", "1000000000.123", 0, moneyTooLarge},
		{"real positive exponent", "real", "1.0e+20", 0, moneyTooLarge},
		{"real negative exponent residue snaps to zero", "real", "5.5511151231257827e-17", 0, moneyOK},
		{"real negative exponent within the snap tolerance snaps to zero", "real", "4.0e-07", 0, moneyOK},
		{"real negative exponent beyond the snap tolerance", "real", "2.0e-06", 0, moneyPrecision},
		{"real small negative exponent", "real", "1.0e-05", 0, moneyPrecision},
		{"real negative exponent beyond exact arithmetic is below the snap tolerance", "real", "1.0e-99999999999", 0, moneyOK},
		{"real exponent form above the bound", "real", "1000000000.0e-0", 0, moneyTooLarge},
		{"real exponent form above int64", "real", "18446744073709551616.0e-0", 0, moneyTooLarge},
		{"real exponent form with a huge mantissa above the bound", "real", "99999999999999999999.0e-5", 0, moneyTooLarge},
		{"real exponent form with a large mantissa below the bound", "real", "10000000000.0e-5", 10000000, moneyOK},
		{"real negative exponent with a malformed mantissa", "real", "x.1e-05", 0, moneyNotANumber},
		{"real negative exponent with no exponent digits", "real", "1.0e-", 0, moneyNotANumber},
		{"real just under the bound snaps up past it", "real", "999999999.9999999", 0, moneyTooLarge},
		{"real negative just under the bound snaps down past it", "real", "-999999999.9999999", 0, moneyTooLarge},
		{"real too close to the bound to snap", "real", "999999999.9999", 0, moneyPrecision},
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
		{"real dot with no fraction digits", "real", "1.", 0, moneyNotANumber},
		{"text storage", "text", "12.34", 0, moneyNotANumber},
		{"text empty", "text", "", 0, moneyNotANumber},
		{"text whitespace", "text", "   ", 0, moneyNotANumber},
		{"blob storage", "blob", "\xde\xad\xbe\xef", 0, moneyNotANumber},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
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
