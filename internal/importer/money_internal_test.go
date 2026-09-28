// White-box: parseMoney is unexported, called only from row-mapping code
// that already knows a column is non-NULL. Its integer/real x
// precision/range decision matrix is combinatorial and not economical to
// drive one snapshot fixture per case.
package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		{"real bound in", "real", "9999999999999.99", 9999999999999*100 + 99, moneyOK},
		{"real bound out", "real", "10000000000000.5", 0, moneyTooLarge},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotCents, gotFault := parseMoney(c.typ, c.text)

			assert.Equal(t, c.wantFault, gotFault)
			assert.Equal(t, c.wantCents, gotCents)
		})
	}
}
