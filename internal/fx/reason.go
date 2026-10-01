package fx

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// The reason a fetch fell short, as the sync warning prints it.
const (
	reasonUnreachable = "cannot reach www.bankofcanada.ca"
	reasonTimeout     = "no answer from www.bankofcanada.ca within 30 seconds"
	reasonNotRates    = "www.bankofcanada.ca sent an answer that is not a list of exchange rates"
)

var (
	// errUnreachable marks a request that never produced an answer: a dial fault or a connection lost mid-body.
	errUnreachable = errors.New("bank unreachable")
	// errNotRates marks an answer that is not a list of exchange rates the store can hold.
	errNotRates = errors.New("not a list of exchange rates")
	// errTimeout marks a request that outlived requestTimeout.
	errTimeout = errors.New("request timed out")
)

// statusError is an answer whose status is not 200.
type statusError struct{ Code int }

func (e statusError) Error() string { return "unexpected status " + statusLine(e.Code) }

// fetchReason returns the ruled reason for a failed Source call, and whether to stop asking: an unreachable or
// timed-out bank would make every later request wait as long. An error no reason names counts as unreachable.
func fetchReason(err error) (string, bool) {
	var status statusError
	switch {
	case errors.Is(err, errTimeout):
		return reasonTimeout, true
	case errors.As(err, &status):
		return "www.bankofcanada.ca answered " + statusLine(status.Code), false
	case errors.Is(err, errNotRates):
		return reasonNotRates, false
	default:
		return reasonUnreachable, true
	}
}

// statusLine is code with its standard text; a code with none shows alone.
func statusLine(code int) string {
	if text := http.StatusText(code); text != "" {
		return fmt.Sprintf("%d %s", code, text)
	}
	return strconv.Itoa(code)
}
