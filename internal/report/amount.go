package report

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// AmountErrorKind is the reason a min/max pair was refused.
type AmountErrorKind int

// The reasons an amount pair is refused.
const (
	// AmountNotAnAmount: Value, the Bound's argument, is not digits with up to two decimals and no sign.
	AmountNotAnAmount AmountErrorKind = iota
	// AmountMinAboveMax: Value, the min, is more than Other, the max.
	AmountMinAboveMax
)

// The bounds an AmountError names.
const (
	boundMin = "min"
	boundMax = "max"
)

// amountGrammar is up to 16 integer digits (DECIMAL(18,2) holds no more) and an optional one or two decimals.
var amountGrammar = regexp.MustCompile(`^[0-9]{1,16}(\.[0-9]{1,2})?$`)

// AmountError is a refusal of a min/max pair, carried as parts so each surface words it in its own
// vocabulary. Bound is "min" or "max"; Value and Other are the arguments as given. Error words it for
// the command line, with "--" before each bound and without the "quarry: " prefix a caller adds.
type AmountError struct {
	Kind  AmountErrorKind
	Bound string
	Value string
	Other string
}

// Error is the command-line wording of the refusal.
func (e AmountError) Error() string {
	flag := "--" + e.Bound
	switch e.Kind {
	case AmountNotAnAmount:
		return fmt.Sprintf("%s %q is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99", flag, e.Value)
	case AmountMinAboveMax:
		return fmt.Sprintf("%s %s is more than --%s %s", flag, e.Value, boundMax, e.Other)
	}
	// unreachable: ParseSearchAmounts is the only AmountError constructor and it sets one of the kinds above
	return flag + " " + e.Value + " is refused"
}

// SearchAmounts is the amount range a search lists, in cents of the amount without its sign; nil is no bound.
type SearchAmounts struct {
	Min, Max *int64
}

// ParseSearchAmounts reads the min and max arguments, each nil when its flag was not given. It refuses, in
// this order, a min or a max that is not an amount and a min above the max, as an AmountError.
func ParseSearchAmounts(least, most *string) (SearchAmounts, error) {
	lo, err := parseAmountBound(boundMin, least)
	if err != nil {
		return SearchAmounts{}, err
	}
	hi, err := parseAmountBound(boundMax, most)
	if err != nil {
		return SearchAmounts{}, err
	}
	if lo != nil && hi != nil && *lo > *hi {
		return SearchAmounts{}, AmountError{Kind: AmountMinAboveMax, Bound: boundMin, Value: *least, Other: *most}
	}
	return SearchAmounts{Min: lo, Max: hi}, nil
}

// parseAmountBound is value in cents, or nil when the flag was not given.
func parseAmountBound(bound string, value *string) (*int64, error) {
	if value == nil {
		return nil, nil //nolint:nilnil // nil is the bound not given
	}
	if !amountGrammar.MatchString(*value) {
		return nil, AmountError{Kind: AmountNotAnAmount, Bound: bound, Value: *value}
	}
	whole, fraction, _ := strings.Cut(*value, ".")
	fraction = (fraction + "00")[:2]
	cents, err := strconv.ParseInt(whole+fraction, 10, 64)
	if err != nil {
		// unreachable: amountGrammar allows at most 18 digits here, which int64 holds
		return nil, AmountError{Kind: AmountNotAnAmount, Bound: bound, Value: *value}
	}
	return &cents, nil
}
