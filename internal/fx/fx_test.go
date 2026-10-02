package fx_test

import (
	"context"
	"errors"
	"io"
	"net/http"
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

type cancellingSource struct {
	cancel context.CancelFunc
	answer []fx.Observation
}

func (c cancellingSource) Observations(context.Context, string, store.DateSpan) ([]fx.Observation, error) {
	c.cancel()
	return c.answer, nil
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
