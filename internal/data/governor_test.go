package data

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestGovernorRetries429WithLimit(t *testing.T) {
	g := NewGovernor(100, 2, 2)
	defer g.Close()
	var calls atomic.Int32
	err := g.Do(context.Background(), func(context.Context) error {
		if calls.Add(1) < 3 {
			return &HTTPError{Status: 429}
		}
		return nil
	})
	if err != nil || calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
	m := g.Metrics()
	if m.TooManyRequests != 2 || m.Retries != 2 {
		t.Fatalf("metrics %+v", m)
	}
}
func TestGovernorHonorsCancellation(t *testing.T) {
	g := NewGovernor(1, 1, 0)
	defer g.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	<-g.tokens
	err := g.Do(ctx, func(context.Context) error { return nil })
	if err == nil {
		t.Fatal("expected cancellation")
	}
}
