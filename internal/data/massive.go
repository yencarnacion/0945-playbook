package data

import (
	"context"
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
	hc := &http.Client{Timeout: timeout}
	return &MassiveProvider{
		client:     polygon.NewWithClient(apiKey, hc),
		multiplier: multiplier,
		timespan:   models.Timespan(strings.ToLower(timespan)),
		adjusted:   adjusted,
		loc:        loc,
	}, nil
}

func (p *MassiveProvider) FetchBars(ctx context.Context, symbol string, start, end time.Time) ([]Bar, error) {
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
		return nil, err
	}

	if p.timespan == models.Minute && p.multiplier == 1 {
		return bars, nil
	}
	return NormalizeToMinutes(bars, p.loc), nil
}
