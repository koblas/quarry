package fx_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func newValet(t *testing.T, rt http.RoundTripper) *fx.Valet {
	t.Helper()
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

func Test_valet_asks_for_the_series_over_the_span_with_a_plain_get(t *testing.T) {
	var got *http.Request
	valet := newValet(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req
		return reply(req, http.StatusOK, io.NopCloser(strings.NewReader(`{"observations":[]}`))), nil
	}))

	_, err := valet.Observations(t.Context(), "FXUSDCAD", january2017)

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, "https://www.bankofcanada.ca/valet/observations/FXUSDCAD/json?end_date=2017-01-06&start_date=2017-01-03", got.URL.String())
	assert.Empty(t, got.Header)
	assert.Nil(t, got.Body)
}

func Test_valet_reads_a_series_answer_and_skips_the_days_without_a_value(t *testing.T) {
	body, err := os.ReadFile("testdata/valet_fxusdcad.json")
	require.NoError(t, err)
	valet := newValet(t, answering(http.StatusOK, string(body)))

	got, err := valet.Observations(t.Context(), "FXUSDCAD", january2017)

	require.NoError(t, err)
	assert.Equal(t, []fx.Observation{
		{Date: day(2017, 1, 3), Rate: 1343500},
		{Date: day(2017, 1, 4), Rate: 1331500},
		{Date: day(2017, 1, 6), Rate: 1324400},
	}, got)
}

func Test_valet_reads_the_legacy_series_answer(t *testing.T) {
	body, err := os.ReadFile("testdata/valet_iexe0101.json")
	require.NoError(t, err)
	valet := newValet(t, answering(http.StatusOK, string(body)))

	got, err := valet.Observations(t.Context(), "IEXE0101", span(day(2005, 3, 1), day(2005, 3, 3)))

	require.NoError(t, err)
	assert.Equal(t, []fx.Observation{
		{Date: day(2005, 3, 1), Rate: 1234500},
		{Date: day(2005, 3, 2), Rate: 1240000},
	}, got)
}

func Test_valet_reads_a_rate_into_millionths(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  money.Rate
	}{
		{name: "four places pad to millionths", value: "1.3456", want: 1345600},
		{name: "a whole number has no places", value: "2", want: 2000000},
		{name: "six places is the most kept", value: "1.234567", want: 1234567},
		{name: "the largest rate DECIMAL(10,6) holds", value: "9999.999999", want: 9999999999},
		{name: "the smallest positive rate", value: "0.000001", want: 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			valet := newValet(t, answering(http.StatusOK, observationBody("FXUSDCAD", c.value)))

			got, err := valet.Observations(t.Context(), "FXUSDCAD", january2017)

			require.NoError(t, err)
			assert.Equal(t, []fx.Observation{{Date: day(2017, 1, 3), Rate: c.want}}, got)
		})
	}
}

func Test_valet_refuses_the_whole_answer_when_a_rate_cannot_be_stored(t *testing.T) {
	cases := []struct{ name, value string }{
		{name: "seven decimal places", value: "1.2345678"},
		{name: "one past DECIMAL(10,6)", value: "10000"},
		{name: "one millionth past DECIMAL(10,6)", value: "9999.9999991"},
		{name: "a whole part that wraps to a valid rate when scaled", value: "18446744073710"},
		{name: "zero", value: "0"},
		{name: "zero with places", value: "0.000000"},
		{name: "negative", value: "-1.5"},
		{name: "not a number", value: "n/a"},
		{name: "two points", value: "1.2.3"},
		{name: "a signed fraction", value: "1.-5"},
		{name: "empty", value: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			valet := newValet(t, answering(http.StatusOK, observationBody("FXUSDCAD", c.value)))

			got, err := valet.Observations(t.Context(), "FXUSDCAD", january2017)

			require.Error(t, err)
			assert.Empty(t, got)
		})
	}
}

func Test_valet_fails_the_answer_when_the_body_is_not_an_observation_list(t *testing.T) {
	cases := []struct{ name, body string }{
		{name: "not JSON", body: "<html>"},
		{name: "observations is not a list", body: `{"observations":{}}`},
		{name: "an observation without a date", body: `{"observations":[{"FXUSDCAD":{"v":"1.3"}}]}`},
		{name: "a date that is not a string", body: `{"observations":[{"d":20170103,"FXUSDCAD":{"v":"1.3"}}]}`},
		{name: "a date that is not a calendar date", body: `{"observations":[{"d":"2017-13-40","FXUSDCAD":{"v":"1.3"}}]}`},
		{name: "a series entry that is not an object", body: `{"observations":[{"d":"2017-01-03","FXUSDCAD":"1.3"}]}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			valet := newValet(t, answering(http.StatusOK, c.body))

			_, err := valet.Observations(t.Context(), "FXUSDCAD", january2017)

			require.Error(t, err)
		})
	}
}

func Test_valet_fails_when_the_request_cannot_be_made(t *testing.T) {
	valet := newValet(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errDial }))

	_, err := valet.Observations(t.Context(), "FXUSDCAD", january2017)

	require.ErrorIs(t, err, errDial)
}

func Test_valet_fails_on_a_status_other_than_200(t *testing.T) {
	valet := newValet(t, answering(http.StatusServiceUnavailable, `{"observations":[]}`))

	_, err := valet.Observations(t.Context(), "FXUSDCAD", january2017)

	require.ErrorContains(t, err, "Service Unavailable")
}

var (
	errDial  = errors.New("dial tcp: connection refused")
	errReset = errors.New("connection reset")
)

type failingBody struct{ err error }

func (b failingBody) Read([]byte) (int, error) { return 0, b.err }
func (failingBody) Close() error               { return nil }

func Test_valet_fails_when_the_body_cannot_be_read(t *testing.T) {
	valet := newValet(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return reply(req, http.StatusOK, failingBody{err: errReset}), nil
	}))

	_, err := valet.Observations(t.Context(), "FXUSDCAD", january2017)

	require.ErrorIs(t, err, errReset)
}

func Test_valet_fails_when_the_request_cannot_be_built(t *testing.T) {
	valet := newValet(t, answering(http.StatusOK, `{}`))

	_, err := valet.Observations(context.Context(nil), "FXUSDCAD", january2017)

	require.Error(t, err)
}

func Test_NewValet_without_a_client_uses_the_default_transport(t *testing.T) {
	var reached bool
	prev := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		reached = true
		return reply(req, http.StatusOK, io.NopCloser(strings.NewReader(`{"observations":[]}`))), nil
	})
	t.Cleanup(func() { http.DefaultTransport = prev })

	got, err := fx.NewValet(nil).Observations(t.Context(), "FXUSDCAD", store.DateSpan{First: day(2026, 1, 2), Last: day(2026, 1, 2)})

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.True(t, reached, "the default client's transport carried the request")
}
