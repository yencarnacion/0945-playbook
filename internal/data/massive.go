package data

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	polygon "github.com/polygon-io/client-go/rest"
	"github.com/polygon-io/client-go/rest/models"
)

type MassiveProvider struct {
	client     *polygon.Client
	multiplier int
	timespan   models.Timespan
	adjusted   bool
	loc        *time.Location
}
type responseMeta struct {
	status                int
	retryAfter, requestID string
}
type responseMetaKey struct{}
type metaTransport struct{ base http.RoundTripper }

func (t metaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(r)
	if m, ok := r.Context().Value(responseMetaKey{}).(*responseMeta); ok && resp != nil {
		m.status = resp.StatusCode
		m.retryAfter = resp.Header.Get("Retry-After")
		m.requestID = resp.Header.Get("X-Request-ID")
	}
	return resp, err
}

func NewMassiveProvider(apiKey string, multiplier int, timespan string, adjusted bool, timeout time.Duration, loc *time.Location) (*MassiveProvider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("missing Massive API key")
	}
	if multiplier <= 0 {
		return nil, fmt.Errorf("source multiplier must be positive")
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	hc := &http.Client{Timeout: timeout, Transport: metaTransport{base: http.DefaultTransport}}
	return newMassiveProviderWithHTTP(apiKey, multiplier, timespan, adjusted, loc, hc), nil
}

func newMassiveProviderWithHTTP(apiKey string, multiplier int, timespan string, adjusted bool, loc *time.Location, hc *http.Client) *MassiveProvider {
	return &MassiveProvider{
		client:     polygon.NewWithClient(apiKey, hc),
		multiplier: multiplier,
		timespan:   models.Timespan(strings.ToLower(timespan)),
		adjusted:   adjusted,
		loc:        loc,
	}
}

func (p *MassiveProvider) FetchBars(ctx context.Context, symbol string, start, end time.Time) ([]Bar, error) {
	meta := &responseMeta{}
	ctx = context.WithValue(ctx, responseMetaKey{}, meta)
	order := models.Asc
	limit := 50000
	adjusted := p.adjusted
	params := &models.ListAggsParams{
		Ticker:     symbol,
		Multiplier: p.multiplier,
		Timespan:   p.timespan,
		From:       models.Millis(start.UTC()),
		To:         models.Millis(end.UTC()),
		Adjusted:   &adjusted,
		Order:      &order,
		Limit:      &limit,
	}

	iter := p.client.ListAggs(ctx, params)
	bars := make([]Bar, 0, 512)
	for iter.Next() {
		agg := iter.Item()
		bars = append(bars, Bar{
			Time:   time.Time(agg.Timestamp).UTC(),
			Open:   agg.Open,
			High:   agg.High,
			Low:    agg.Low,
			Close:  agg.Close,
			Volume: agg.Volume,
			VWAP:   agg.VWAP,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, normalizeMassiveError(err, meta, "GET", "/v2/aggs/ticker")
	}

	if p.timespan == models.Minute && p.multiplier == 1 {
		return bars, nil
	}
	return NormalizeToMinutes(bars, p.loc), nil
}
func normalizeMassiveError(err error, m *responseMeta, method, endpoint string) error {
	status, requestID, message := m.status, m.requestID, err.Error()
	var pe *models.ErrorResponse
	if errors.As(err, &pe) {
		if status == 0 {
			status = pe.StatusCode
		}
		if requestID == "" {
			requestID = pe.RequestID
		}
		if pe.ErrorMessage != "" {
			message = pe.ErrorMessage
		}
	}
	retry := retryableStatus(status)
	if status == 0 {
		var temporary interface{ Temporary() bool }
		retry = errors.As(err, &temporary) && temporary.Temporary()
	}
	return &HTTPError{Status: status, RetryAfter: m.retryAfter, Retryable: retry, Endpoint: endpoint, Method: method, RequestID: requestID, ProviderMessage: message, Err: err}
}
