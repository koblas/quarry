package report

import (
	"fmt"
	"slices"
	"strconv"
	"time"
)

// Cut is a restricted to the securities it names (Selected, SelectedIDs) and then to the tax year a.Year names. A
// selection keeps those securities and their sales, and re-sums each year's totals over them; a security with a
// trade quarry could not value adds no return of capital above its ACB. It is the one cut: a report that names
// neither is returned as it is.
func (a ACB) Cut() ACB {
	return a.selectedOnly().InYear()
}

// selectedOnly is a cut to the securities SelectedIDs names; a report with no selection is returned as it is.
func (a ACB) selectedOnly() ACB {
	if !a.Selected {
		return a
	}
	named := make(map[string]bool, len(a.SelectedIDs))
	for _, id := range a.SelectedIDs {
		named[id] = true
	}

	cut := a
	cut.Securities = nil
	var excesses []acbExcess
	for _, security := range a.Securities {
		if !named[security.Security.ID] {
			continue
		}
		cut.Securities = append(cut.Securities, security)
		if security.NoRate == nil {
			excesses = append(excesses, security.excesses()...)
		}
	}
	var sales []ACBSale
	for _, year := range a.Years {
		sales = append(sales, slices.DeleteFunc(slices.Clone(year.Sales), func(sale ACBSale) bool { return !named[sale.SecurityID] })...)
	}
	cut.Years = yearsOf(sales, excesses)

	return cut
}

// excesses are the returns of capital above the ACB among the security's events.
func (s ACBSecurity) excesses() []acbExcess {
	var excesses []acbExcess
	for _, e := range s.Events {
		if e.Action == ACBActionReturnOfCapital && e.Realized {
			excesses = append(excesses, acbExcess{date: e.Date, amount: e.Gain})
		}
	}

	return excesses
}

// InYear is a cut to the tax year a.Year names: Years holds that year alone, a zero ACBYear with no sales when
// the report has none, and Securities those with a sale or a return of capital above the ACB counted in it,
// events and order unchanged. A report with no Year is returned as it is.
func (a ACB) InYear() ACB {
	if a.Year == 0 {
		return a
	}

	cut := a
	cut.Years = []ACBYear{a.yearOrZero()}
	cut.Securities = nil
	for _, security := range a.Securities {
		if security.soldOrExceededIn(a.Year, a.Years) {
			cut.Securities = append(cut.Securities, security)
		}
	}

	return cut
}

// yearOrZero is the report's entry for a.Year, or an empty one.
func (a ACB) yearOrZero() ACBYear {
	i := slices.IndexFunc(a.Years, func(y ACBYear) bool { return y.Year == a.Year })
	if i < 0 {
		return ACBYear{Year: a.Year}
	}

	return a.Years[i]
}

// soldOrExceededIn is whether the security has a counted sale in year, or a counted return of capital above its
// ACB; a security with a trade quarry could not value counts none.
func (s ACBSecurity) soldOrExceededIn(year int, years []ACBYear) bool {
	if s.NoRate != nil {
		return false
	}
	for _, y := range years {
		if y.Year == year && slices.ContainsFunc(y.Sales, func(sale ACBSale) bool { return sale.SecurityID == s.Security.ID }) {
			return true
		}
	}

	return slices.ContainsFunc(s.Events, func(e ACBEvent) bool {
		return e.Action == ACBActionReturnOfCapital && e.Realized && e.Date.Year() == year
	})
}

// NoPoolEvents is whether no non-registered account bought or sold a security, so the report has no position.
func (a ACB) NoPoolEvents() bool {
	return len(a.Securities) == 0
}

// SaleYears is the first and last year with a counted sale, and false when no year has one. A year with only a
// return of capital above the ACB, or only sales quarry could not value, is not one.
func (a ACB) SaleYears() (int, int, bool) {
	var first, last int
	found := false
	for _, year := range a.Years {
		if len(year.Sales) == 0 {
			continue
		}
		if !found {
			first, found = year.Year, true
		}
		last = year.Year
	}

	return first, last, found
}

// YearIsEmpty is whether a.Year has no counted sale and no return of capital above the ACB; false without a Year.
func (a ACB) YearIsEmpty() bool {
	if a.Year == 0 {
		return false
	}
	year := a.yearOrZero()

	return len(year.Sales) == 0 && year.ReturnOfCapitalGain == 0
}

// ACBYearErrorKind is the reason a tax year was refused.
type ACBYearErrorKind int

// The reasons a tax year is refused.
const (
	// ACBYearNotAYear: Value is not four digits naming a year from 0001.
	ACBYearNotAYear ACBYearErrorKind = iota
	// ACBYearAfterThisYear: Value is a year after the current one.
	ACBYearAfterThisYear
)

// ACBYearError is a refusal of a --year value, Value as typed. Error words it for the command line, without the
// "quarry: " prefix.
type ACBYearError struct {
	Kind  ACBYearErrorKind
	Value string
}

// Error is the command-line wording of the refusal.
func (e ACBYearError) Error() string {
	if e.Kind == ACBYearAfterThisYear {
		return fmt.Sprintf("--year %s is after this year; pass this year or an earlier one", e.Value)
	}

	return fmt.Sprintf("--year %q is not a year; use YYYY, such as 2024", e.Value)
}

// ParseACBYear is the tax year value names: four digits, 0001 to the year of now in now's own zone. It returns an
// ACBYearError for any other text and for a year after this one.
func ParseACBYear(value string, now time.Time) (int, error) {
	year, ok := fourDigitYear(value)
	if !ok {
		return 0, ACBYearError{Kind: ACBYearNotAYear, Value: value}
	}
	if year > Today(now).Year() {
		return 0, ACBYearError{Kind: ACBYearAfterThisYear, Value: value}
	}

	return year, nil
}

// fourDigitYear is value as a year when it is exactly four ASCII digits, not 0000.
func fourDigitYear(value string) (int, bool) {
	const digits = 4
	if len(value) != digits || value[0] == '+' || value[0] == '-' {
		return 0, false
	}
	year, err := strconv.Atoi(value)
	if err != nil || year < 1 {
		return 0, false
	}

	return year, true
}
