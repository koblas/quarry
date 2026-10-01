package fx_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func Test_refresh_gives_each_failure_its_ruled_reason(t *testing.T) {
	cases := []struct {
		name string
		src  fx.Source
		want string
	}{
		{name: "the bank cannot be reached", src: valetVia(dialFault()), want: cannotReach},
		{name: "the bank answers 503", src: valetVia(answering(http.StatusServiceUnavailable, "")), want: answered503},
		{name: "a status with no standard text shows the code alone", src: valetVia(answering(599, "")), want: "www.bankofcanada.ca answered 599"},
		{name: "the answer is not JSON", src: valetVia(answering(http.StatusOK, "<html>maintenance</html>")), want: notAList},
		{name: "a source offers a rate the store cannot hold", src: &fakeSource{answers: map[string][]fx.Observation{current: {obs(0, 0)}}}, want: notAList},
		{name: "the answer is one byte over the size cap", src: valetVia(answering(http.StatusOK, paddedAnswer(answerCap+1))), want: notAList},
		{
			name: "the connection drops while the answer is read", want: cannotReach,
			src: valetVia(roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return reply(req, http.StatusOK, failingBody{err: errReset}), nil
			})),
		},
		{name: "a source fails in a way no reason names", src: &fakeSource{failures: map[string]error{current: errBoom}}, want: cannotReach},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := newRefresher(c.src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(5))})

			require.NoError(t, err)
			assert.Equal(t, store.RatesRefresh{FetchError: c.want}, got)
		})
	}
}

func Test_refresh_gives_the_timeout_reason_only_after_30_seconds(t *testing.T) {
	cases := []struct {
		name string
		rt   roundTripFunc
		want store.RatesRefresh
	}{
		{
			name: "an answer one nanosecond before the limit is taken",
			rt:   answeringAfter(requestTimeout-time.Nanosecond, ratesBody(current, 0)),
			want: store.RatesRefresh{Rates: []store.Rate{rateOn(0, current)}, Added: 1},
		},
		{
			name: "an answer one nanosecond after the limit is a timeout",
			rt:   answeringAfter(requestTimeout+time.Nanosecond, ratesBody(current, 0)),
			want: store.RatesRefresh{FetchError: noAnswerInTime},
		},
		{
			name: "a body that stalls after the headers is a timeout",
			rt: func(req *http.Request) (*http.Response, error) {
				return reply(req, http.StatusOK, stallingBody{done: req.Context().Done()}), nil
			},
			want: store.RatesRefresh{FetchError: noAnswerInTime},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := &bank{replies: map[string]roundTripFunc{startOf(current, 0): c.rt}}

				got, err := b.refresher().Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(0))})

				require.NoError(t, err)
				assert.Equal(t, c.want, got)
			})
		})
	}
}

func Test_refresh_times_each_request_alone_so_slow_answers_that_each_beat_30_seconds_all_count(t *testing.T) {
	const slow = 20 * time.Second
	cases := []struct {
		name    string
		need    store.RatesRequest
		replies map[string]roundTripFunc
		want    store.RatesRefresh
	}{
		{
			name: "a head span and a tail span",
			need: headAndTail,
			replies: map[string]roundTripFunc{
				startOf(current, 0): answeringAfter(slow, ratesBody(current, 0, 1)),
				startOf(current, 7): answeringAfter(slow, ratesBody(current, 8)),
			},
			want: store.RatesRefresh{Rates: []store.Rate{rateOn(0, current), rateOn(1, current), rateOn(8, current)}, Added: 3},
		},
		{
			name: "a span and its legacy request",
			need: store.RatesRequest{Need: span(d(0), d(2))},
			replies: map[string]roundTripFunc{
				startOf(current, 0): answeringAfter(slow, ratesBody(current, 2)),
				startOf(legacy, 0):  answeringAfter(slow, ratesBody(legacy, 0)),
			},
			want: store.RatesRefresh{Rates: []store.Rate{rateOn(0, legacy), rateOn(2, current)}, Added: 2},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := &bank{replies: c.replies}

				got, err := b.refresher().Refresh(t.Context(), c.need)

				require.NoError(t, err)
				assert.Equal(t, c.want, got)
			})
		})
	}
}

func Test_refresh_returns_an_error_when_the_parent_ends_during_a_timed_request(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		time.AfterFunc(10*time.Second, cancel)
		b := &bank{replies: map[string]roundTripFunc{startOf(current, 0): answeringAfter(time.Hour, ratesBody(current, 0))}}

		got, err := b.refresher().Refresh(ctx, store.RatesRequest{Need: span(d(0), d(0))})

		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, store.RatesRefresh{}, got)
	})
}

func Test_refresh_keeps_the_rates_of_the_spans_that_answered(t *testing.T) {
	cases := []struct {
		name    string
		replies map[string]roundTripFunc
		want    store.RatesRefresh
	}{
		{
			name: "the head answers and the tail fails",
			replies: map[string]roundTripFunc{
				startOf(current, 0): answering(http.StatusOK, ratesBody(current, 0, 1)),
				startOf(current, 7): answering(http.StatusServiceUnavailable, ""),
			},
			want: store.RatesRefresh{Rates: []store.Rate{rateOn(0, current), rateOn(1, current)}, Added: 2, FetchError: answered503, Partial: true},
		},
		{
			name: "the head fails and the tail answers",
			replies: map[string]roundTripFunc{
				startOf(current, 0): answering(http.StatusServiceUnavailable, ""),
				startOf(current, 7): answering(http.StatusOK, ratesBody(current, 8)),
			},
			want: store.RatesRefresh{Rates: []store.Rate{rateOn(8, current)}, Added: 1, FetchError: answered503, Partial: true},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &bank{replies: c.replies}

			got, err := b.refresher().Refresh(t.Context(), headAndTail)

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
			assert.Equal(t, bothSpansAsk, b.asked)
		})
	}
}

func Test_refresh_does_not_ask_the_later_spans_once_the_bank_cannot_be_reached_or_has_timed_out(t *testing.T) {
	cases := []struct {
		name string
		head roundTripFunc
		want string
	}{
		{name: "the bank cannot be reached", head: dialFault(), want: cannotReach},
		{name: "the request timed out", head: failWith(context.DeadlineExceeded), want: noAnswerInTime},
		{
			name: "the connection drops while the answer is read", want: cannotReach,
			head: func(req *http.Request) (*http.Response, error) {
				return reply(req, http.StatusOK, failingBody{err: errReset}), nil
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &bank{replies: map[string]roundTripFunc{startOf(current, 0): c.head}}

			got, err := b.refresher().Refresh(t.Context(), headAndTail)

			require.NoError(t, err)
			assert.Equal(t, store.RatesRefresh{FetchError: c.want}, got)
			assert.Equal(t, []string{startOf(current, 0)}, b.asked)
		})
	}
}

func Test_refresh_keeps_the_current_rates_when_the_legacy_series_fails(t *testing.T) {
	b := &bank{replies: map[string]roundTripFunc{
		startOf(current, 0): answering(http.StatusOK, ratesBody(current, 2)),
		startOf(legacy, 0):  answering(http.StatusServiceUnavailable, ""),
	}}

	got, err := b.refresher().Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(5))})

	require.NoError(t, err)
	assert.Equal(t, store.RatesRefresh{Rates: []store.Rate{rateOn(2, current)}, Added: 1, FetchError: answered503, Partial: true}, got)
}

func Test_refresh_stops_after_the_legacy_series_times_out_and_keeps_the_current_rates(t *testing.T) {
	b := &bank{replies: map[string]roundTripFunc{
		startOf(current, 0): answering(http.StatusOK, ratesBody(current, 1)),
		startOf(legacy, 0):  failWith(context.DeadlineExceeded),
	}}

	got, err := b.refresher().Refresh(t.Context(), headAndTail)

	require.NoError(t, err)
	assert.Equal(t, store.RatesRefresh{Rates: []store.Rate{rateOn(1, current)}, Added: 1, FetchError: noAnswerInTime, Partial: true}, got)
	assert.Equal(t, []string{startOf(current, 0), startOf(legacy, 0)}, b.asked)
}

func Test_refresh_reports_the_first_reason_when_two_spans_fail_differently(t *testing.T) {
	cases := []struct {
		name       string
		head, tail roundTripFunc
		want       string
	}{
		{name: "the head answers 503 and the tail is not a list", head: answering(http.StatusServiceUnavailable, ""), tail: answering(http.StatusOK, "<html>"), want: answered503},
		{name: "the head is not a list and the tail answers 503", head: answering(http.StatusOK, "<html>"), tail: answering(http.StatusServiceUnavailable, ""), want: notAList},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &bank{replies: map[string]roundTripFunc{startOf(current, 0): c.head, startOf(current, 7): c.tail}}

			got, err := b.refresher().Refresh(t.Context(), headAndTail)

			require.NoError(t, err)
			assert.Equal(t, store.RatesRefresh{FetchError: c.want}, got)
			assert.Equal(t, bothSpansAsk, b.asked)
		})
	}
}

func Test_refresh_returns_an_error_when_the_parent_ends_after_a_failed_span(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	b := &bank{replies: map[string]roundTripFunc{
		startOf(current, 0): answering(http.StatusServiceUnavailable, ""),
		startOf(current, 7): func(req *http.Request) (*http.Response, error) {
			cancel()
			return answering(http.StatusOK, ratesBody(current, 8))(req)
		},
	}}

	got, err := b.refresher().Refresh(ctx, headAndTail)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, store.RatesRefresh{}, got)
}

func Test_refresh_is_not_partial_when_the_answered_span_was_empty(t *testing.T) {
	b := &bank{replies: map[string]roundTripFunc{startOf(current, 7): answering(http.StatusServiceUnavailable, "")}}

	got, err := b.refresher().Refresh(t.Context(), headAndTail)

	require.NoError(t, err)
	assert.Equal(t, store.RatesRefresh{FetchError: answered503}, got)
}
