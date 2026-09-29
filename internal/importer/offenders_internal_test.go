// White-box: lessOffender is unexported comparison logic with several
// branches (dated vs undated, date/account/source-id tiebreaks); driving
// each branch through a full snapshot per case is not economical.
package importer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_lessOffender(t *testing.T) {
	early := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		a, b offender
		want bool
	}{
		{"undated sorts before dated", offender{dated: false, name: "Z"}, offender{dated: true, date: early}, true},
		{"dated does not sort before undated", offender{dated: true, date: early}, offender{dated: false, name: "A"}, false},
		{"earlier date sorts first", offender{dated: true, date: early, account: "B"}, offender{dated: true, date: late, account: "A"}, true},
		{"same date: account breaks the tie", offender{dated: true, date: early, account: "Alpha"}, offender{dated: true, date: early, account: "Zeta"}, true},
		{"same date and account: source id breaks the tie", offender{dated: true, date: early, account: "A", sourceID: 1}, offender{dated: true, date: early, account: "A", sourceID: 2}, true},
		{"undated: name breaks the tie", offender{name: "Alpha"}, offender{name: "Zeta"}, true},
		{"undated: same name, source id breaks the tie", offender{name: "A", sourceID: 1}, offender{name: "A", sourceID: 2}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, lessOffender(c.a, c.b))
		})
	}
}

func Test_firstError_groups_the_more_count_by_thousands(t *testing.T) {
	var off offenders
	for range 1001 {
		off.add(offender{class: 10, reason: "a split (source id 1) has no transaction"})
	}

	err := off.firstError()

	assert.EqualError(t, err, "a split (source id 1) has no transaction (and 1,000 more)")
}
