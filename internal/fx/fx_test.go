package fx_test

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_refresh_asks_only_for_the_dates_have_does_not_cover(t *testing.T) {
	cases := []struct {
		name       string
		need, have store.DateSpan
		want       []store.DateSpan
	}{
		{name: "nothing stored asks for all of need", need: span(d(0), d(9)), want: []store.DateSpan{span(d(0), d(9))}},
		{name: "have inside need asks for the head and the tail", need: span(d(0), d(9)), have: span(d(3), d(6)), want: []store.DateSpan{span(d(0), d(2)), span(d(7), d(9))}},
		{name: "have covering the head asks for the tail only", need: span(d(0), d(9)), have: span(d(0), d(6)), want: []store.DateSpan{span(d(7), d(9))}},
		{name: "have covering the tail asks for the head only", need: span(d(0), d(9)), have: span(d(3), d(9)), want: []store.DateSpan{span(d(0), d(2))}},
		{name: "have covering need asks for nothing", need: span(d(2), d(5)), have: span(d(0), d(9))},
		{name: "an empty need asks for nothing", have: span(d(0), d(9))},
		{name: "a tail one day long", need: span(d(0), d(7)), have: span(d(0), d(6)), want: []store.DateSpan{span(d(7), d(7))}},
		{name: "a head one day long", need: span(d(2), d(9)), have: span(d(3), d(9)), want: []store.DateSpan{span(d(2), d(2))}},
		{name: "have wholly before need continues from the day after have", need: span(d(5), d(9)), have: span(d(0), d(2)), want: []store.DateSpan{span(d(3), d(9))}},
		{name: "have ending the day before need asks for need", need: span(d(5), d(9)), have: span(d(0), d(4)), want: []store.DateSpan{span(d(5), d(9))}},
		{name: "a need ending before it starts asks for nothing", need: span(d(9), d(5))},
		{name: "a need ending before it starts asks for nothing beside a have", need: span(d(9), d(5)), have: span(d(0), d(2))},
		{name: "have wholly after need asks for the days up to have", need: span(d(0), d(2)), have: span(d(5), d(9)), want: []store.DateSpan{span(d(0), d(4))}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := &fakeSource{answers: map[string][]fx.Observation{current: {obs(0, 1)}}}

			_, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: c.need, Have: c.have})

			require.NoError(t, err)
			assert.Equal(t, c.want, src.spansAsked(current))
		})
	}
}

func Test_refresh_does_not_ask_for_the_legacy_series_when_the_current_one_covers_the_span(t *testing.T) {
	src := &fakeSource{answers: map[string][]fx.Observation{current: {obs(0, 1_300_000), obs(1, 1_310_000)}}}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(1))})

	require.NoError(t, err)
	assert.Empty(t, src.spansAsked(legacy))
	assert.Equal(t, []store.Rate{
		{Date: d(0), USDCAD: 1_300_000, Series: current},
		{Date: d(1), USDCAD: 1_310_000, Series: current},
	}, got.Rates)
}

func Test_refresh_asks_for_the_legacy_series_over_the_days_before_the_first_current_observation(t *testing.T) {
	src := &fakeSource{answers: map[string][]fx.Observation{
		current: {obs(2, 1_300_000)},
		legacy:  {obs(0, 1_200_000), obs(1, 1_210_000), obs(2, 1_220_000), obs(3, 1_230_000)},
	}}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(5))})

	require.NoError(t, err)
	assert.Equal(t, []store.DateSpan{span(d(0), d(1))}, src.spansAsked(legacy))
	assert.Equal(t, []store.Rate{
		{Date: d(0), USDCAD: 1_200_000, Series: legacy},
		{Date: d(1), USDCAD: 1_210_000, Series: legacy},
		{Date: d(2), USDCAD: 1_300_000, Series: current},
	}, got.Rates)
}

func Test_refresh_asks_for_the_legacy_series_over_the_whole_span_when_the_current_one_has_nothing(t *testing.T) {
	src := &fakeSource{answers: map[string][]fx.Observation{legacy: {obs(0, 1_200_000)}}}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(5))})

	require.NoError(t, err)
	assert.Equal(t, []store.DateSpan{span(d(0), d(5))}, src.spansAsked(legacy))
	assert.Equal(t, []store.Rate{{Date: d(0), USDCAD: 1_200_000, Series: legacy}}, got.Rates)
}

func Test_refresh_asks_for_the_legacy_series_over_a_span_ending_before_what_is_stored(t *testing.T) {
	src := &fakeSource{answers: map[string][]fx.Observation{legacy: {obs(0, 1_200_000)}}}

	_, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(9)), Have: span(d(4), d(9))})

	require.NoError(t, err)
	assert.Equal(t, []store.DateSpan{span(d(0), d(3))}, src.spansAsked(legacy))
}

func Test_refresh_does_not_ask_for_the_legacy_series_over_a_span_after_what_is_stored(t *testing.T) {
	src := &fakeSource{}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(9)), Have: span(d(0), d(6))})

	require.NoError(t, err)
	assert.Empty(t, src.spansAsked(legacy))
	assert.Empty(t, got.Rates)
}

func Test_refresh_asks_for_the_legacy_series_over_a_weekend_before_the_first_current_observation(t *testing.T) {
	saturday, monday := 3, 5
	src := &fakeSource{answers: map[string][]fx.Observation{current: {obs(monday, 1_300_000)}}}

	_, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(saturday), d(monday+2))})

	require.NoError(t, err)
	assert.Equal(t, []store.DateSpan{span(d(saturday), d(saturday+1))}, src.spansAsked(legacy))
}

func Test_refresh_takes_the_current_series_when_both_answer_for_a_date(t *testing.T) {
	src := &fakeSource{answers: map[string][]fx.Observation{
		current: {obs(2, 1_300_000)},
		legacy:  {obs(1, 1_210_000), obs(2, 1_999_999), obs(4, 1_240_000)},
	}}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(5))})

	require.NoError(t, err)
	assert.Equal(t, []store.Rate{
		{Date: d(1), USDCAD: 1_210_000, Series: legacy},
		{Date: d(2), USDCAD: 1_300_000, Series: current},
	}, got.Rates)
}

func Test_refresh_drops_an_observation_dated_outside_the_asked_span(t *testing.T) {
	src := &fakeSource{answers: map[string][]fx.Observation{current: {obs(0, 1_100_000), obs(2, 1_300_000), obs(9, 1_900_000)}}}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(1), d(5))})

	require.NoError(t, err)
	assert.Equal(t, []store.Rate{{Date: d(2), USDCAD: 1_300_000, Series: current}}, got.Rates)
}

func Test_refresh_keeps_the_first_observation_when_a_series_repeats_a_date(t *testing.T) {
	src := &fakeSource{answers: map[string][]fx.Observation{current: {obs(1, 1_300_000), obs(1, 1_400_000)}}}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(1), d(2))})

	require.NoError(t, err)
	assert.Equal(t, []store.Rate{{Date: d(1), USDCAD: 1_300_000, Series: current}}, got.Rates)
}

func Test_refresh_returns_the_head_and_the_tail_oldest_first_and_counts_them(t *testing.T) {
	src := &fakeSource{answers: map[string][]fx.Observation{current: {obs(8, 1_380_000), obs(1, 1_310_000)}}}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(9)), Have: span(d(3), d(6))})

	require.NoError(t, err)
	assert.Equal(t, []store.Rate{
		{Date: d(1), USDCAD: 1_310_000, Series: current},
		{Date: d(8), USDCAD: 1_380_000, Series: current},
	}, got.Rates)
	assert.Equal(t, 2, got.Added)
}

func Test_refresh_reports_no_failure_and_no_rates_when_the_source_answers_with_nothing(t *testing.T) {
	src := &fakeSource{}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(5))})

	require.NoError(t, err)
	assert.Equal(t, store.RatesRefresh{}, got)
}

func Test_refresh_reports_a_rate_the_store_cannot_hold_as_a_fetch_error(t *testing.T) {
	cases := []struct {
		name string
		rate money.Rate
	}{
		{name: "zero", rate: 0},
		{name: "negative", rate: -1_300_000},
		{name: "past DECIMAL(10,6)", rate: 10_000_000_000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := &fakeSource{answers: map[string][]fx.Observation{current: {obs(0, 1_300_000), obs(1, c.rate)}}}

			got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(1))})

			require.NoError(t, err)
			assert.Equal(t, store.RatesRefresh{FetchError: notAList}, got)
		})
	}
}

func Test_refresh_accepts_the_largest_rate_the_store_can_hold(t *testing.T) {
	src := &fakeSource{answers: map[string][]fx.Observation{current: {obs(0, 9_999_999_999)}}}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(0))})

	require.NoError(t, err)
	assert.Equal(t, []store.Rate{{Date: d(0), USDCAD: 9_999_999_999, Series: current}}, got.Rates)
}

func Test_refresh_reports_a_source_timeout_as_a_fetch_error_while_the_context_is_live(t *testing.T) {
	src := &fakeSource{failures: map[string]error{current: context.DeadlineExceeded}}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(5))})

	require.NoError(t, err)
	assert.Equal(t, noAnswerInTime, got.FetchError)
}

func Test_refresh_returns_an_error_when_the_context_ends_during_a_fetch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		cancel()
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}

	got, err := fx.NewServer(fx.WithHTTPClient(client)).Refresh(ctx, store.RatesRequest{Need: span(d(0), d(5))})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, store.RatesRefresh{}, got)
}

func Test_refresh_returns_an_error_when_the_context_deadline_has_passed(t *testing.T) {
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	src := &fakeSource{failures: map[string]error{current: context.DeadlineExceeded}}

	got, err := newRefresher(src).Refresh(ctx, store.RatesRequest{Need: span(d(0), d(5))})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, store.RatesRefresh{}, got)
}

func Test_refresh_returns_an_error_when_the_context_ends_though_the_source_answered(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	src := cancellingSource{cancel: cancel, answer: []fx.Observation{obs(0, 1_300_000)}}

	got, err := newRefresher(src).Refresh(ctx, store.RatesRequest{Need: span(d(0), d(0))})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, store.RatesRefresh{}, got)
}

func Test_refresh_sends_its_requests_through_the_given_http_client(t *testing.T) {
	var paths []string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		body := `{"observations":[{"d":"2020-01-01","FXUSDCAD":{"v":"1.3"}}]}`
		return reply(req, http.StatusOK, io.NopCloser(strings.NewReader(body))), nil
	})}

	got, err := fx.NewServer(fx.WithHTTPClient(client)).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(0))})

	require.NoError(t, err)
	assert.Equal(t, []string{"/valet/observations/FXUSDCAD/json"}, paths)
	assert.Equal(t, []store.Rate{{Date: d(0), USDCAD: 1_300_000, Series: current}}, got.Rates)
}

func Test_refresh_takes_the_earliest_current_observation_as_the_cutover_whatever_order_the_source_lists_them(t *testing.T) {
	src := &fakeSource{answers: map[string][]fx.Observation{current: {obs(4, 1_340_000), obs(2, 1_320_000)}}}

	got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: span(d(0), d(5))})

	require.NoError(t, err)
	assert.Equal(t, []store.DateSpan{span(d(0), d(1))}, src.spansAsked(legacy))
	assert.Equal(t, []store.Rate{
		{Date: d(2), USDCAD: 1_320_000, Series: current},
		{Date: d(4), USDCAD: 1_340_000, Series: current},
	}, got.Rates)
}

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
		{name: "the answer holds a rate with more decimals than the store keeps", src: valetVia(answering(http.StatusOK, observationOf("2020-01-01", "1.3456789"))), want: notAList},
		{name: "the answer holds a date that is not a day", src: valetVia(answering(http.StatusOK, observationOf("2020-13-45", "1.3456"))), want: notAList},
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

func Test_refresh_asks_for_nothing_when_need_is_empty_or_ends_before_it_starts(t *testing.T) {
	empty := slices.DeleteFunc(gridNeeds(), func(n store.DateSpan) bool { return !n.First.IsZero() && !n.Last.Before(n.First) })

	overGrid(t, empty, func(a *assert.Assertions, run gridRun) {
		a.Empty(run.asked)
		a.Empty(run.got.Rates)
	})
}

func Test_refresh_asks_for_the_days_that_join_have_and_need_into_one_interval(t *testing.T) {
	overGrid(t, nonEmpty(gridNeeds()), func(a *assert.Assertions, run gridRun) {
		a.IsIncreasing(run.asked, "oldest first, no day twice, none inside have")
		a.Equal(hullIndexes(run.need, run.have), sorted(run.asked, indexesOf(run.have)))
	})
}

func Test_refresh_returns_a_rate_for_each_day_it_asked_for(t *testing.T) {
	overGrid(t, nonEmpty(gridNeeds()), func(a *assert.Assertions, run gridRun) {
		returned := make([]int, 0, len(run.got.Rates))
		for _, r := range run.got.Rates {
			returned = append(returned, dayIndex(r.Date))
		}
		a.Equal(hullIndexes(run.need, run.have), sorted(returned, indexesOf(run.have)))
		a.Equal(len(run.got.Rates), run.got.Added)
	})
}
