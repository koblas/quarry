package money

// Rate is CAD per one USD, in millionths: 1.25 is Rate(1250000).
type Rate int64

// Currency is a currency a report can show amounts in.
type Currency int

// The currencies Convert understands. Native is an account's own currency,
// which is never a conversion target or source.
const (
	Native Currency = iota
	CAD
	USD
)
