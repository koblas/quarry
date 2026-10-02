package fx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// valetObservations is the Bank of Canada Valet endpoint; a series name and "/json" follow it.
const valetObservations = "https://www.bankofcanada.ca/valet/observations/"

// A stored rate is DECIMAL(10,6): ratePlaces decimal places, millionths per unit, and at most store.MaxRate.
const (
	ratePlaces = 6
	millionth  = 1_000_000
)

// maxAnswerBytes bounds the answer Valet reads; the largest real one, a series over some sixty years, is about 2 MB.
const maxAnswerBytes = 16 << 20

var errRate = errors.New("unreadable rate")

// Valet is the Source that reads the Bank of Canada Valet web service.
type Valet struct {
	client *http.Client
}

// NewValet returns a Valet that makes its requests with client; nil means http.DefaultClient.
func NewValet(client *http.Client) *Valet {
	if client == nil {
		client = http.DefaultClient
	}
	return &Valet{client: client}
}

// Observations returns the series' published rates dated within span, in the order Valet lists them.
// A day with no published rate is skipped. The whole answer fails on a transport or status fault, on
// a body past maxAnswerBytes or that is not a Valet observation list, or on any date or rate it cannot read.
func (v *Valet) Observations(ctx context.Context, series string, span store.DateSpan) ([]Observation, error) {
	body, err := v.get(ctx, series, span)
	if err != nil {
		return nil, err
	}
	return parseObservations(series, body)
}

func (v *Valet) get(ctx context.Context, series string, span store.DateSpan) ([]byte, error) {
	query := url.Values{
		"start_date": {span.First.Format(time.DateOnly)},
		"end_date":   {span.Last.Format(time.DateOnly)},
	}
	target := valetObservations + url.PathEscape(series) + "/json?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", series, err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w: %w", series, errUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: %w", series, statusError{Code: resp.StatusCode})
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w: %w", series, errUnreachable, err)
	}
	if len(body) > maxAnswerBytes {
		return nil, fmt.Errorf("read %s: %w: more than %d bytes", series, errNotRates, maxAnswerBytes)
	}
	return body, nil
}

// parseObservations reads a Valet answer: {"observations":[{"d":"2017-01-03","<series>":{"v":"1.3435"}}]}.
func parseObservations(series string, body []byte) ([]Observation, error) {
	var answer struct {
		Observations []map[string]json.RawMessage `json:"observations"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, fmt.Errorf("decode %s: %w: %w", series, errNotRates, err)
	}
	var out []Observation
	for _, raw := range answer.Observations {
		obs, ok, err := parseObservation(series, raw)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w: %w", series, errNotRates, err)
		}
		if ok {
			out = append(out, obs)
		}
	}
	return out, nil
}

// parseObservation reports false for a day the series has no value for.
func parseObservation(series string, raw map[string]json.RawMessage) (Observation, bool, error) {
	var day string
	if err := json.Unmarshal(raw["d"], &day); err != nil {
		return Observation{}, false, fmt.Errorf("observation date: %w", err)
	}
	date, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return Observation{}, false, fmt.Errorf("observation date: %w", err)
	}
	entry, ok := raw[series]
	if !ok {
		return Observation{}, false, nil
	}
	var value struct {
		V string `json:"v"`
	}
	if err := json.Unmarshal(entry, &value); err != nil {
		return Observation{}, false, fmt.Errorf("rate on %s: %w", day, err)
	}
	rate, err := parseRate(value.V)
	if err != nil {
		return Observation{}, false, fmt.Errorf("rate on %s: %w", day, err)
	}
	return Observation{Date: date, Rate: rate}, true, nil
}

// parseRate reads a plain decimal of at most ratePlaces places, such as "1.3456", into millionths.
func parseRate(s string) (money.Rate, error) {
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) > ratePlaces {
		return 0, fmt.Errorf("%w %q: more than %d decimal places", errRate, s, ratePlaces)
	}
	units, err := strconv.ParseUint(whole, 10, 64)
	if err != nil || units > uint64(store.MaxRate)/millionth {
		return 0, fmt.Errorf("%w %q: not a number in range", errRate, s)
	}
	fraction := uint64(0)
	if frac != "" {
		fraction, err = strconv.ParseUint(frac+strings.Repeat("0", ratePlaces-len(frac)), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%w %q: not a number", errRate, s)
		}
	}
	rate := money.Rate(units*millionth + fraction)
	return rate, checkRate(rate)
}

// checkRate refuses a rate the store cannot hold: zero, negative, or past DECIMAL(10,6).
func checkRate(r money.Rate) error {
	if r <= 0 || r > store.MaxRate {
		return fmt.Errorf("%w %d millionths: out of range", errRate, r)
	}
	return nil
}
