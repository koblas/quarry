package fx_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	current = "FXUSDCAD"
	legacy  = "IEXE0101"
)

var errBoom = errors.New("connection refused")

type call struct {
	Series string
	Span   store.DateSpan
}

// fakeSource answers each series with its fixed observations whatever the span asked, so a test can
// see what Refresh keeps; failures[series] makes that series fail, callFailures[c] makes only that
// one request fail.
type fakeSource struct {
	answers      map[string][]fx.Observation
	failures     map[string]error
	callFailures map[call]error
	calls        []call
}

func (f *fakeSource) Observations(_ context.Context, series string, sp store.DateSpan) ([]fx.Observation, error) {
	asked := call{Series: series, Span: sp}
	f.calls = append(f.calls, asked)
	if err := f.callFailures[asked]; err != nil {
		return nil, err
	}
	if err := f.failures[series]; err != nil {
		return nil, err
	}
	return f.answers[series], nil
}

func (f *fakeSource) spansAsked(series string) []store.DateSpan {
	var out []store.DateSpan
	for _, c := range f.calls {
		if c.Series == series {
			out = append(out, c.Span)
		}
	}
	return out
}

// d is a date n days after 2020-01-01, a Wednesday.
func d(n int) time.Time { return day(2020, 1, 1).AddDate(0, 0, n) }

func obs(n int, rate money.Rate) fx.Observation { return fx.Observation{Date: d(n), Rate: rate} }

func newRefresher(src fx.Source) *fx.Server { return fx.NewServer(fx.WithSource(src)) }

type cancellingSource struct {
	cancel context.CancelFunc
	answer []fx.Observation
}

func (c cancellingSource) Observations(context.Context, string, store.DateSpan) ([]fx.Observation, error) {
	c.cancel()
	return c.answer, nil
}

const (
	cannotReach    = "cannot reach www.bankofcanada.ca"
	noAnswerInTime = "no answer from www.bankofcanada.ca within 30 seconds"
	answered503    = "www.bankofcanada.ca answered 503 Service Unavailable"
	notAList       = "www.bankofcanada.ca sent an answer that is not a list of exchange rates"

	requestTimeout = 30 * time.Second
	answerCap      = 16 << 20
)

// bank is the Bank of Canada as scripted replies keyed by "<series> <start date>"; any other request gets an
// empty answer. It records the keys it was asked, in order.
type bank struct {
	replies map[string]roundTripFunc
	asked   []string
}

func (b *bank) RoundTrip(req *http.Request) (*http.Response, error) {
	series := strings.Split(req.URL.Path, "/")[3]
	key := series + " " + req.URL.Query().Get("start_date")
	b.asked = append(b.asked, key)
	if reply, ok := b.replies[key]; ok {
		return reply(req)
	}
	return answering(http.StatusOK, `{"observations":[]}`)(req)
}

func (b *bank) refresher() *fx.Server {
	return fx.NewServer(fx.WithHTTPClient(&http.Client{Transport: b}))
}

// startOf is the key of the request for series starting on the day n days after d(0).
func startOf(series string, n int) string { return series + " " + d(n).Format(time.DateOnly) }

// ratesBody is a Valet answer carrying 1.3 for series on each of the days.
func ratesBody(series string, days ...int) string {
	items := make([]string, len(days))
	for i, n := range days {
		items[i] = `{"d":"` + d(n).Format(time.DateOnly) + `","` + series + `":{"v":"1.3"}}`
	}
	return `{"observations":[` + strings.Join(items, ",") + `]}`
}

func rateOn(n int, series string) store.Rate {
	return store.Rate{Date: d(n), USDCAD: 1_300_000, Series: series}
}

// paddedAnswer is a valid Valet answer for FXUSDCAD, followed by blanks to size bytes.
func paddedAnswer(size int) string {
	answer := ratesBody(current, 0)
	return answer + strings.Repeat(" ", size-len(answer))
}

func failWith(err error) roundTripFunc {
	return func(*http.Request) (*http.Response, error) { return nil, err }
}

func dialFault() roundTripFunc {
	return failWith(&net.OpError{Op: "dial", Net: "tcp", Err: errDial})
}

func valetVia(rt http.RoundTripper) fx.Source { return fx.NewValet(&http.Client{Transport: rt}) }

// answeringAfter answers 200 with body after delay on the request's clock, or fails when the request's context ends first.
func answeringAfter(delay time.Duration, body string) roundTripFunc {
	return func(req *http.Request) (*http.Response, error) {
		select {
		case <-time.After(delay):
			return reply(req, http.StatusOK, io.NopCloser(strings.NewReader(body))), nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
}

// stallingBody blocks every read until done closes, then fails the way a cut connection does, without the cause.
type stallingBody struct{ done <-chan struct{} }

func (b stallingBody) Read([]byte) (int, error) {
	<-b.done
	return 0, io.ErrUnexpectedEOF
}
func (stallingBody) Close() error { return nil }

var (
	headAndTail  = store.RatesRequest{Need: span(d(0), d(9)), Have: span(d(3), d(6))}
	bothSpansAsk = []string{startOf(current, 0), startOf(current, 7)}
)

// observationOf is a Valet answer holding one FXUSDCAD observation, written as the bank would, date and rate as given.
func observationOf(date, rate string) string {
	return `{"observations":[{"d":"` + date + `","FXUSDCAD":{"v":"` + rate + `"}}]}`
}

// gridDays is how many consecutive days, d(0)..d(gridDays-1), the Need and Have spans are drawn from.
const gridDays = 7

// everyDaySource answers FXUSDCAD with one observation for each day of every span it is asked, and records the spans.
type everyDaySource struct{ asked []store.DateSpan }

func (s *everyDaySource) Observations(_ context.Context, series string, sp store.DateSpan) ([]fx.Observation, error) {
	s.asked = append(s.asked, sp)
	if series != current {
		return nil, nil
	}
	var out []fx.Observation
	for day := sp.First; !day.After(sp.Last); day = day.AddDate(0, 0, 1) {
		out = append(out, fx.Observation{Date: day, Rate: 1_300_000})
	}
	return out, nil
}

// dayIndex is the number of days from d(0) to t.
func dayIndex(t time.Time) int { return int(t.Sub(d(0)) / (24 * time.Hour)) }

// indexesOf lists the day indexes of each span in turn, in the order given; a zero or inverted span has none.
func indexesOf(spans ...store.DateSpan) []int {
	var out []int
	for _, sp := range spans {
		if sp.First.IsZero() {
			continue
		}
		for i := dayIndex(sp.First); i <= dayIndex(sp.Last); i++ {
			out = append(out, i)
		}
	}
	return out
}

// hullIndexes lists every day from the earliest first to the latest last of the non-zero spans.
func hullIndexes(spans ...store.DateSpan) []int {
	var firsts, lasts []int
	for _, sp := range spans {
		if sp.First.IsZero() {
			continue
		}
		firsts = append(firsts, dayIndex(sp.First))
		lasts = append(lasts, dayIndex(sp.Last))
	}
	var out []int
	for i := slices.Min(firsts); i <= slices.Max(lasts); i++ {
		out = append(out, i)
	}
	return out
}

func sorted(a, b []int) []int {
	all := slices.Concat(a, b)
	slices.Sort(all)
	return all
}

// gridNeeds is the zero span, then every pair of grid days, in either order.
func gridNeeds() []store.DateSpan {
	needs := make([]store.DateSpan, 1, 1+gridDays*gridDays) // the zero span first
	for first := range gridDays {
		for last := range gridDays {
			needs = append(needs, span(d(first), d(last)))
		}
	}
	return needs
}

// gridHaves is the zero span, then every pair of grid days that does not end before it starts.
func gridHaves() []store.DateSpan {
	haves := []store.DateSpan{{}}
	for first := range gridDays {
		for last := first; last < gridDays; last++ {
			haves = append(haves, span(d(first), d(last)))
		}
	}
	return haves
}

func spanLabel(sp store.DateSpan) string {
	if sp.First.IsZero() {
		return "none"
	}
	return fmt.Sprintf("d%d..d%d", dayIndex(sp.First), dayIndex(sp.Last))
}

// gridRun is one Refresh over a Need and Have, with the day indexes it asked and the rates it returned.
type gridRun struct {
	need, have store.DateSpan
	asked      []int
	got        store.RatesRefresh
}

// overGrid runs Refresh for each of needs against every Have, and checks each run in its own subtest.
func overGrid(t *testing.T, needs []store.DateSpan, check func(a *assert.Assertions, run gridRun)) {
	t.Helper()
	for _, need := range needs {
		for _, have := range gridHaves() {
			t.Run("need "+spanLabel(need)+" have "+spanLabel(have), func(t *testing.T) {
				src := &everyDaySource{}

				got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: need, Have: have})

				require.NoError(t, err)
				check(assert.New(t), gridRun{need: need, have: have, asked: indexesOf(src.asked...), got: got})
			})
		}
	}
}

func nonEmpty(needs []store.DateSpan) []store.DateSpan {
	return slices.DeleteFunc(slices.Clone(needs), func(n store.DateSpan) bool { return n.First.IsZero() || n.Last.Before(n.First) })
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func reply(req *http.Request, status int, body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: body, Request: req}
}

func answering(status int, body string) roundTripFunc {
	return func(req *http.Request) (*http.Response, error) {
		return reply(req, status, io.NopCloser(strings.NewReader(body))), nil
	}
}

func newValet(tb testing.TB, rt http.RoundTripper) *fx.Valet {
	tb.Helper()
	return fx.NewValet(&http.Client{Transport: rt})
}

func day(year int, month time.Month, d int) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}

func span(first, last time.Time) store.DateSpan { return store.DateSpan{First: first, Last: last} }

func observationBody(series, value string) string {
	return `{"observations":[{"d":"2017-01-03","` + series + `":{"v":"` + value + `"}}]}`
}

var january2017 = span(day(2017, 1, 3), day(2017, 1, 6))

// blanks is a reader of size spaces that counts what was taken from it.
type blanks struct{ size, taken int }

func (b *blanks) Read(p []byte) (int, error) {
	n := min(len(p), b.size-b.taken)
	for i := range p[:n] {
		p[i] = ' '
	}
	b.taken += n
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

var (
	errDial  = errors.New("dial tcp: connection refused")
	errReset = errors.New("connection reset")
)

type failingBody struct{ err error }

func (b failingBody) Read([]byte) (int, error) { return 0, b.err }
func (failingBody) Close() error               { return nil }
