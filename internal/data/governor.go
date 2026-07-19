package data

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type RESTMetrics struct {
	Requests        uint64 `json:"requests"`
	Retries         uint64 `json:"retries"`
	TooManyRequests uint64 `json:"http_429"`
	InFlight        int64  `json:"in_flight"`
	BackoffMS       uint64 `json:"backoff_ms"`
}
type HTTPError struct {
	Status     int
	RetryAfter string
}

func (e *HTTPError) Error() string { return "HTTP " + strconv.Itoa(e.Status) }

type Governor struct {
	tokens                                      chan struct{}
	sem                                         chan struct{}
	retries                                     int
	stop                                        chan struct{}
	once                                        sync.Once
	requests, retry, tooMany, backoff, inflight atomic.Uint64
}

type GovernedProvider struct {
	provider Provider
	governor *Governor
}

func NewGovernedProvider(p Provider, g *Governor) *GovernedProvider {
	return &GovernedProvider{provider: p, governor: g}
}
func (p *GovernedProvider) FetchBars(ctx context.Context, symbol string, start, end time.Time) (bars []Bar, err error) {
	err = p.governor.Do(ctx, func(callCtx context.Context) error {
		var callErr error
		bars, callErr = p.provider.FetchBars(callCtx, symbol, start, end)
		return callErr
	})
	return
}
func (p *GovernedProvider) Metrics() RESTMetrics { return p.governor.Metrics() }
func (p *GovernedProvider) Close()               { p.governor.Close() }

func NewGovernor(rate, concurrency, retries int) *Governor {
	if rate < 1 {
		rate = 80
	}
	if concurrency < 1 {
		concurrency = 8
	}
	g := &Governor{tokens: make(chan struct{}, rate), sem: make(chan struct{}, concurrency), retries: retries, stop: make(chan struct{})}
	for i := 0; i < rate; i++ {
		g.tokens <- struct{}{}
	}
	go func() {
		t := time.NewTicker(time.Second / time.Duration(rate))
		defer t.Stop()
		for {
			select {
			case <-g.stop:
				return
			case <-t.C:
				select {
				case g.tokens <- struct{}{}:
				default:
				}
			}
		}
	}()
	return g
}
func (g *Governor) Close() { g.once.Do(func() { close(g.stop) }) }
func (g *Governor) Do(ctx context.Context, fn func(context.Context) error) error {
	var err error
	for attempt := 0; attempt <= g.retries; attempt++ {
		if attempt > 0 {
			g.retry.Add(1)
			d := time.Duration(1<<min(attempt, 6))*100*time.Millisecond + time.Duration(rand.Intn(100))*time.Millisecond
			he := new(HTTPError)
			if errors.As(err, &he) && he.RetryAfter != "" {
				if secs, e := strconv.Atoi(he.RetryAfter); e == nil {
					d = time.Duration(secs) * time.Second
				}
			}
			g.backoff.Add(uint64(d.Milliseconds()))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-g.tokens:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case g.sem <- struct{}{}:
		}
		g.requests.Add(1)
		g.inflight.Add(1)
		err = fn(ctx)
		g.inflight.Add(^uint64(0))
		<-g.sem
		if err == nil {
			return nil
		}
		he := new(HTTPError)
		if !errors.As(err, &he) || he.Status != 429 && he.Status < 500 {
			return err
		}
		if he.Status == 429 {
			g.tooMany.Add(1)
		}
	}
	return err
}
func (g *Governor) Metrics() RESTMetrics {
	return RESTMetrics{Requests: g.requests.Load(), Retries: g.retry.Load(), TooManyRequests: g.tooMany.Load(), InFlight: int64(g.inflight.Load()), BackoffMS: g.backoff.Load()}
}
