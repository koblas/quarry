package fx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// Server fetches the exchange rates a store build needs.
type Server struct {
	client *http.Client
	source Source
}

// Option configures a Server.
type Option func(*Server)

// WithHTTPClient sets the client the default Source uses to reach the publisher.
func WithHTTPClient(c *http.Client) Option {
	return func(s *Server) { s.client = c }
}

// WithSource replaces the default Source.
func WithSource(src Source) Option {
	return func(s *Server) { s.source = src }
}

// NewServer returns a Server with the given options applied; without a Source it reads the Bank of Canada Valet.
func NewServer(opts ...Option) *Server {
	s := &Server{}
	for _, o := range opts {
		o(s)
	}
	if s.source == nil {
		s.source = NewValet(s.client)
	}
	return s
}

// requestTimeout bounds each request to the Source; the sync warning's "within 30 seconds" is this value.
const requestTimeout = 30 * time.Second

// Refresh fetches the rates for req.Need that req.Have does not cover, oldest first, none dated
// inside Have and none out of range. A source that answers with nothing is not a failure.
// A failed Source call does not drop what answered: the span's FXUSDCAD rates survive a failed IEXE0101 call,
// and spans that answered survive a failed one. Partial is then set (FetchError set and at least one rate kept)
// and FetchError carries the first failure's reason. After a timeout or an unreachable bank the later spans are
// not asked; after any other failure they are. It returns an error only when ctx ended.
func (s *Server) Refresh(ctx context.Context, req store.RatesRequest) (store.RatesRefresh, error) {
	var out store.RatesRefresh
	for _, ask := range planSpans(req) {
		got, err := s.fetch(ctx, ask)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return store.RatesRefresh{}, fmt.Errorf("fetch rates: %w", ctxErr)
		}
		out.Rates = append(out.Rates, got...)
		if err == nil {
			continue
		}
		reason, stop := fetchReason(err)
		if out.FetchError == "" {
			out.FetchError = reason
		}
		if stop {
			break
		}
	}
	out.Added = len(out.Rates)
	out.Partial = out.FetchError != "" && len(out.Rates) > 0
	return out, nil
}

// fetch returns one run's rates: FXUSDCAD's, plus the legacy series' for the days before FXUSDCAD's first
// observation when the run may use it. The cutover is whatever FXUSDCAD answered with, never a constant.
// When only the legacy series fails it returns FXUSDCAD's rates beside the error.
func (s *Server) fetch(ctx context.Context, ask askSpan) ([]store.Rate, error) {
	current, err := s.observe(ctx, seriesCurrent, ask.span)
	if err != nil {
		return nil, err
	}
	legacySpan := ask.span
	if len(current) > 0 {
		legacySpan.Last = dayBefore(current[0].Date)
	}
	if !ask.legacy || legacySpan.Last.Before(legacySpan.First) {
		return toRates(current, seriesCurrent), nil
	}
	legacy, err := s.observe(ctx, seriesLegacy, legacySpan)
	if err != nil {
		return toRates(current, seriesCurrent), err
	}
	return append(toRates(legacy, seriesLegacy), toRates(current, seriesCurrent)...), nil
}

// observe asks the source for a series over span, within requestTimeout, and returns its valid observations
// dated inside span, oldest first, the first of any repeated date kept. A rate the store could not hold fails
// the answer as errNotRates; a request that outlived requestTimeout fails as errTimeout.
func (s *Server) observe(ctx context.Context, series string, span store.DateSpan) ([]Observation, error) {
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	got, err := s.source.Observations(reqCtx, series, span)
	if err != nil {
		// A read cut by the deadline may drop the cause, so the request's own clock decides too.
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %w", errTimeout, err)
		}
		return nil, err //nolint:wrapcheck // Source errors already name their series
	}
	var kept []Observation
	for _, o := range got {
		if o.Date.Before(span.First) || o.Date.After(span.Last) {
			continue
		}
		if err := checkRate(o.Rate); err != nil {
			return nil, fmt.Errorf("%s on %s: %w: %w", series, o.Date.Format(time.DateOnly), errNotRates, err)
		}
		if slices.ContainsFunc(kept, func(k Observation) bool { return k.Date.Equal(o.Date) }) {
			continue
		}
		kept = append(kept, o)
	}
	slices.SortFunc(kept, func(a, b Observation) int { return a.Date.Compare(b.Date) })
	return kept, nil
}

func toRates(obs []Observation, series string) []store.Rate {
	rates := make([]store.Rate, len(obs))
	for i, o := range obs {
		rates[i] = store.Rate{Date: o.Date, USDCAD: o.Rate, Series: series}
	}
	return rates
}
